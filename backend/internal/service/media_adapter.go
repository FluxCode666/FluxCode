package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"reflect"
	"slices"
	"strings"
	"sync"
)

var ErrMediaAdapterNotFound = errors.New("media adapter not found")

type MediaExecutionRequest struct {
	Task       *MediaTask
	Account    *Account
	Definition *MediaModelDefinition
	Spec       MediaSpec
	// ResolvedRequest is the immutable, account-specific JSON request after
	// declarative mapping. Spec remains the validated canonical domain request;
	// adapters that need provider-specific fields must consume this snapshot.
	ResolvedRequest json.RawMessage
	UpstreamModel   string
	IdempotencyKey  string
	// Inputs contains validated, bounded bytes for input artifacts referenced by
	// Spec. The Worker materializes this slice before calling an Adapter so
	// provider implementations never need repository or object-store access.
	Inputs []MediaArtifactInput
}

type MediaArtifactInput struct {
	Direction       string
	Position        int
	MediaType       MediaType
	ContentType     string
	Data            []byte
	SizeBytes       int64
	ChecksumSHA256  string
	StorageProvider string
	// StorageRevision is an opaque revision of the encrypted storage settings
	// row used for this write. It is internal-only and is never serialized to an
	// upstream or downstream request.
	StorageRevision   string `json:"-"`
	ObjectKey         string
	ExternalURL       string
	UpstreamReference string
	Width             int
	Height            int
	DurationSeconds   float64
	Resolution        string
	FPS               float64
}

type MediaContent struct {
	Body          io.ReadCloser
	StatusCode    int
	ContentType   string
	ContentLength int64
	ContentRange  string
	AcceptRanges  string
}

type MediaHTTPContentRequest struct {
	URL       string
	Headers   http.Header
	Account   *Account
	MediaType MediaType
	ByteRange string
}

type MediaHTTPContentReader interface {
	ValidateURL(raw string) (string, error)
	Open(ctx context.Context, req MediaHTTPContentRequest) (*MediaContent, error)
}

type MediaContentFetcher interface {
	OpenContent(ctx context.Context, account *Account, artifact *MediaArtifact, byteRange string) (*MediaContent, error)
}

type MediaArtifactObjectStore interface {
	Put(ctx context.Context, input MediaArtifactInput) (*MediaArtifact, error)
	Open(ctx context.Context, artifact *MediaArtifact, byteRange string) (*MediaContent, error)
	Discard(ctx context.Context, input MediaArtifactInput) error
}

// MediaArtifactStreamObjectStore persists a bounded media body without first
// materializing the complete object in memory. Enabled production stores must
// implement this interface so large generated videos remain constant-memory.
type MediaArtifactStreamObjectStore interface {
	PutStream(ctx context.Context, input MediaArtifactInput, body io.Reader) (*MediaArtifact, error)
}

type MediaInputStager interface {
	Stage(ctx context.Context, userID int64, input MediaArtifactInput) (MediaArtifactInput, error)
}

type MediaInputDiscarder interface {
	Discard(ctx context.Context, userID int64, input MediaArtifactInput) error
}

type MediaInputLifecycle interface {
	MediaInputStager
	MediaInputDiscarder
}

// MediaArtifactInputReader materializes immutable input artifacts for an
// Adapter after the Worker has selected an account. Implementations must
// enforce task ownership, media type, byte limits and integrity checks.
type MediaArtifactInputReader interface {
	LoadInputs(ctx context.Context, task *MediaTask, spec MediaSpec, account *Account) ([]MediaArtifactInput, error)
}

type MediaUsage struct {
	ImageCount      int
	ImageSize       string
	OutputTokens    int
	VideoSeconds    float64
	VideoResolution string
}

type MediaGenerateResult struct {
	Artifacts []MediaArtifactInput
	Usage     MediaUsage
}

type MediaAsyncSubmission struct {
	UpstreamTaskID string
	PollMetadata   json.RawMessage
}

type MediaPollState string

const (
	MediaPollStateRunning   MediaPollState = "running"
	MediaPollStateCompleted MediaPollState = "completed"
	MediaPollStateFailed    MediaPollState = "failed"
	MediaPollStateCanceled  MediaPollState = "canceled"
)

type MediaPollRequest struct {
	Account        *Account
	UpstreamTaskID string
	PollMetadata   json.RawMessage
}

type MediaPollResult struct {
	State    MediaPollState
	Progress int
	Result   *MediaGenerateResult
	Error    *MediaAdapterError
}

// MediaAdapterError carries stable, caller-safe failure classification while
// retaining an optional internal cause for errors.Is/errors.As and logging.
type MediaAdapterError struct {
	Code              string `json:"code"`
	Message           string `json:"message"`
	Retryable         bool   `json:"retryable"`
	SubmissionUnknown bool   `json:"submission_unknown"`
	SystemFailure     bool   `json:"system_failure"`
	Cause             error  `json:"-"`
}

func (e *MediaAdapterError) Error() string {
	if e == nil || strings.TrimSpace(e.Message) == "" {
		return "media adapter error"
	}
	return e.Message
}

func (e *MediaAdapterError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Cause
}

type MediaAdapter interface {
	Name() string
}

type MediaSyncGenerator interface {
	Generate(ctx context.Context, req MediaExecutionRequest) (*MediaGenerateResult, error)
}

type MediaAsyncSubmitter interface {
	Submit(ctx context.Context, req MediaExecutionRequest) (*MediaAsyncSubmission, error)
}

type MediaIdempotentSubmitter interface {
	MediaAsyncSubmitter
	SupportsIdempotentSubmit() bool
}

type MediaAsyncPoller interface {
	Poll(ctx context.Context, req MediaPollRequest) (*MediaPollResult, error)
}

type MediaAborter interface {
	Abort(ctx context.Context, req MediaPollRequest) error
}

type MediaAdapterRegistry struct {
	mu             sync.RWMutex
	adapters       map[string]MediaAdapter
	aliases        map[string]string
	registrations  []MediaAdapterRegistration
	routingMetrics MediaRoutingMetrics
	logger         *slog.Logger
}

func NewMediaAdapterRegistry() *MediaAdapterRegistry {
	return &MediaAdapterRegistry{
		adapters: make(map[string]MediaAdapter),
		aliases:  make(map[string]string),
		logger:   slog.Default(),
	}
}

func (r *MediaAdapterRegistry) SetRoutingMetrics(metrics MediaRoutingMetrics) {
	r.mu.Lock()
	r.routingMetrics = metrics
	r.mu.Unlock()
}

func (r *MediaAdapterRegistry) SetLogger(logger *slog.Logger) {
	if logger == nil {
		logger = slog.Default()
	}
	r.mu.Lock()
	r.logger = logger
	r.mu.Unlock()
}

func (r *MediaAdapterRegistry) Register(name string, adapter MediaAdapter) error {
	key := normalizeMediaAdapterName(name)
	if key == "" || isNilMediaAdapter(adapter) {
		return errors.New("media adapter name and implementation are required")
	}
	if normalizeMediaAdapterName(adapter.Name()) != key {
		return fmt.Errorf("media adapter key %q does not match implementation name %q", key, adapter.Name())
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.aliases[key]; exists {
		return fmt.Errorf("media adapter key %q conflicts with an alias", key)
	}
	if _, exists := r.adapters[key]; exists {
		return fmt.Errorf("duplicate media adapter: %s", key)
	}
	if r.adapters == nil {
		r.adapters = make(map[string]MediaAdapter)
	}
	r.adapters[key] = adapter
	return nil
}

func (r *MediaAdapterRegistry) RegisterDefinition(registration MediaAdapterRegistration) error {
	normalized, err := normalizeAndValidateMediaAdapterRegistration(registration)
	if err != nil {
		return err
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.aliases[normalized.Key]; exists {
		return fmt.Errorf("media adapter key %q conflicts with an alias", normalized.Key)
	}
	if _, exists := r.adapters[normalized.Key]; exists {
		return fmt.Errorf("duplicate media adapter: %s", normalized.Key)
	}
	exactKeys := make(map[mediaAdapterExactRuleKey]struct{})
	for _, existing := range r.registrations {
		for _, rule := range existing.ExactRules {
			exactKeys[mediaAdapterExactRuleKey{vendor: rule.Vendor, modelID: rule.ModelID}] = struct{}{}
		}
	}
	for _, rule := range normalized.ExactRules {
		key := mediaAdapterExactRuleKey{vendor: rule.Vendor, modelID: rule.ModelID}
		if _, exists := exactKeys[key]; exists {
			return fmt.Errorf("duplicate media adapter exact rule: vendor=%q model_id=%q", rule.Vendor, rule.ModelID)
		}
		exactKeys[key] = struct{}{}
	}

	if r.adapters == nil {
		r.adapters = make(map[string]MediaAdapter)
	}
	r.adapters[normalized.Key] = normalized.Adapter
	r.registrations = append(r.registrations, cloneMediaAdapterRegistration(normalized))
	return nil
}

func (r *MediaAdapterRegistry) RegisterAlias(oldKey, canonicalKey string) error {
	oldKey = normalizeMediaAdapterName(oldKey)
	canonicalKey = normalizeMediaAdapterName(canonicalKey)
	r.mu.Lock()
	defer r.mu.Unlock()
	if oldKey == "" || canonicalKey == "" || oldKey == canonicalKey {
		return errors.New("media adapter alias and canonical key are invalid")
	}
	if _, exists := r.adapters[oldKey]; exists {
		return fmt.Errorf("media adapter alias %q conflicts with a canonical key", oldKey)
	}
	if _, exists := r.aliases[oldKey]; exists {
		return fmt.Errorf("duplicate media adapter alias: %s", oldKey)
	}
	if _, aliasTarget := r.aliases[canonicalKey]; aliasTarget {
		return errors.New("media adapter alias chains are not allowed")
	}
	if _, exists := r.adapters[canonicalKey]; !exists {
		return fmt.Errorf("canonical media adapter %q is not registered", canonicalKey)
	}
	if r.aliases == nil {
		r.aliases = make(map[string]string)
	}
	r.aliases[oldKey] = canonicalKey
	return nil
}

func (r *MediaAdapterRegistry) Resolve(name string) (MediaAdapter, error) {
	requestedKey := normalizeMediaAdapterName(name)
	r.mu.RLock()
	canonicalKey := requestedKey
	if target, ok := r.aliases[requestedKey]; ok {
		canonicalKey = target
	}
	adapter, ok := r.adapters[canonicalKey]
	metrics, logger := r.routingMetrics, r.logger
	r.mu.RUnlock()
	if !ok {
		return nil, ErrMediaAdapterNotFound
	}
	if canonicalKey != requestedKey {
		if metrics != nil {
			metrics.IncrementHistoricalAdapterAliasResolution()
		}
		if logger == nil {
			logger = slog.Default()
		}
		logger.Debug(
			"media_adapter_historical_alias_resolved",
			"legacy_adapter_key", requestedKey,
			"adapter_key", canonicalKey,
		)
	}
	return adapter, nil
}

func (r *MediaAdapterRegistry) CanonicalKey(name string) (canonical string, aliased bool) {
	key := normalizeMediaAdapterName(name)
	r.mu.RLock()
	canonical, aliased = r.aliases[key]
	r.mu.RUnlock()
	if !aliased {
		return key, false
	}
	return canonical, true
}

func (r *MediaAdapterRegistry) Registrations() []MediaAdapterRegistration {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return cloneMediaAdapterRegistrations(r.registrations)
}

func (r *MediaAdapterRegistry) Validate() error {
	r.mu.RLock()
	adapters := make(map[string]MediaAdapter, len(r.adapters))
	for key, adapter := range r.adapters {
		adapters[key] = adapter
	}
	aliases := make(map[string]string, len(r.aliases))
	for alias, canonical := range r.aliases {
		aliases[alias] = canonical
	}
	registrations := cloneMediaAdapterRegistrations(r.registrations)
	r.mu.RUnlock()

	for key, adapter := range adapters {
		if key == "" || normalizeMediaAdapterName(key) != key || isNilMediaAdapter(adapter) {
			return fmt.Errorf("invalid media adapter registry entry %q", key)
		}
		if normalizeMediaAdapterName(adapter.Name()) != key {
			return fmt.Errorf("media adapter key %q does not match implementation name %q", key, adapter.Name())
		}
		if _, exists := aliases[key]; exists {
			return fmt.Errorf("media adapter key %q conflicts with an alias", key)
		}
	}
	for alias, canonical := range aliases {
		if alias == "" || canonical == "" || alias == canonical || normalizeMediaAdapterName(alias) != alias || normalizeMediaAdapterName(canonical) != canonical {
			return fmt.Errorf("invalid media adapter alias %q", alias)
		}
		if _, exists := adapters[alias]; exists {
			return fmt.Errorf("media adapter alias %q conflicts with a canonical key", alias)
		}
		if _, exists := aliases[canonical]; exists {
			return fmt.Errorf("media adapter alias %q points to another alias", alias)
		}
		if _, exists := adapters[canonical]; !exists {
			return fmt.Errorf("media adapter alias %q points to missing key %q", alias, canonical)
		}
	}

	registrationKeys := make(map[string]struct{}, len(registrations))
	exactKeys := make(map[mediaAdapterExactRuleKey]struct{})
	for _, registration := range registrations {
		canonicalKey := normalizeMediaAdapterName(registration.Key)
		if canonicalKey != registration.Key {
			return fmt.Errorf("media adapter registration %q is not canonical: key", registration.Key)
		}
		if _, exists := registrationKeys[canonicalKey]; exists {
			return fmt.Errorf("duplicate media adapter registration key: %s", canonicalKey)
		}
		registrationKeys[canonicalKey] = struct{}{}

		liveAdapter, exists := adapters[canonicalKey]
		if !exists {
			return fmt.Errorf("media adapter registration %q has no implementation", registration.Key)
		}
		if isNilMediaAdapter(registration.Adapter) {
			return fmt.Errorf("media adapter registration %q has no registered implementation snapshot", registration.Key)
		}
		if normalizeMediaAdapterName(registration.Adapter.Name()) != canonicalKey {
			return fmt.Errorf("media adapter registration %q implementation name is not canonical", registration.Key)
		}

		validationRegistration := cloneMediaAdapterRegistration(registration)
		validationRegistration.Adapter = liveAdapter
		normalized, err := normalizeAndValidateMediaAdapterRegistration(validationRegistration)
		if err != nil {
			return fmt.Errorf("validate media adapter registration %q: %w", registration.Key, err)
		}
		if err := validateCanonicalMediaAdapterRegistration(registration, normalized); err != nil {
			return err
		}
		for _, rule := range normalized.ExactRules {
			key := mediaAdapterExactRuleKey{vendor: rule.Vendor, modelID: rule.ModelID}
			if _, exists := exactKeys[key]; exists {
				return fmt.Errorf("duplicate media adapter exact rule: vendor=%q model_id=%q", rule.Vendor, rule.ModelID)
			}
			exactKeys[key] = struct{}{}
		}
	}
	return nil
}

func validateCanonicalMediaAdapterRegistration(stored, normalized MediaAdapterRegistration) error {
	nonCanonical := func(field string) error {
		return fmt.Errorf("media adapter registration %q is not canonical: %s", stored.Key, field)
	}
	if stored.Key != normalized.Key {
		return nonCanonical("key")
	}
	if !slices.Equal(stored.SupportedOperations, normalized.SupportedOperations) {
		return nonCanonical("supported operations")
	}
	if len(stored.ExactRules) != len(normalized.ExactRules) {
		return nonCanonical("exact rules")
	}
	for index := range stored.ExactRules {
		storedRule, normalizedRule := stored.ExactRules[index], normalized.ExactRules[index]
		if storedRule.Vendor != normalizedRule.Vendor {
			return nonCanonical(fmt.Sprintf("exact rule %d vendor", index))
		}
		if storedRule.ModelID != normalizedRule.ModelID {
			return nonCanonical(fmt.Sprintf("exact rule %d model id", index))
		}
		if storedRule.Capabilities.SyncUpstream != normalizedRule.Capabilities.SyncUpstream ||
			storedRule.Capabilities.NativeAsyncUpstream != normalizedRule.Capabilities.NativeAsyncUpstream ||
			!slices.Equal(storedRule.Capabilities.Operations, normalizedRule.Capabilities.Operations) {
			return nonCanonical(fmt.Sprintf("exact rule %d capabilities", index))
		}
	}
	if len(stored.FamilyRules) != len(normalized.FamilyRules) {
		return nonCanonical("family rules")
	}
	for index := range stored.FamilyRules {
		storedRule, normalizedRule := stored.FamilyRules[index], normalized.FamilyRules[index]
		if storedRule.Vendor != normalizedRule.Vendor {
			return nonCanonical(fmt.Sprintf("family rule %d vendor", index))
		}
		if storedRule.FamilyID != normalizedRule.FamilyID {
			return nonCanonical(fmt.Sprintf("family rule %d family id", index))
		}
		if storedRule.Capabilities.SyncUpstream != normalizedRule.Capabilities.SyncUpstream ||
			storedRule.Capabilities.NativeAsyncUpstream != normalizedRule.Capabilities.NativeAsyncUpstream ||
			!slices.Equal(storedRule.Capabilities.Operations, normalizedRule.Capabilities.Operations) {
			return nonCanonical(fmt.Sprintf("family rule %d capabilities", index))
		}
	}
	return nil
}

func normalizeMediaAdapterName(name string) string {
	return strings.ToLower(strings.TrimSpace(name))
}

func isNilMediaAdapter(adapter MediaAdapter) bool {
	if adapter == nil {
		return true
	}
	value := reflect.ValueOf(adapter)
	switch value.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return value.IsNil()
	default:
		return false
	}
}
