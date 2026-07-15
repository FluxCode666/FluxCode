package service

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"sync"
)

var ErrMediaAdapterNotFound = errors.New("media adapter not found")

type MediaExecutionRequest struct {
	Task           *MediaTask
	Account        *Account
	Definition     *MediaModelDefinition
	Spec           MediaSpec
	UpstreamModel  string
	IdempotencyKey string
}

type MediaArtifactInput struct {
	Direction         string
	Position          int
	MediaType         MediaType
	ContentType       string
	Data              []byte
	ExternalURL       string
	UpstreamReference string
	Width             int
	Height            int
	DurationSeconds   float64
	Resolution        string
	FPS               float64
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
	mu       sync.RWMutex
	adapters map[string]MediaAdapter
}

func NewMediaAdapterRegistry() *MediaAdapterRegistry {
	return &MediaAdapterRegistry{adapters: make(map[string]MediaAdapter)}
}

func (r *MediaAdapterRegistry) Register(name string, adapter MediaAdapter) {
	key := normalizeMediaAdapterName(name)
	if key == "" || isNilMediaAdapter(adapter) {
		panic("media adapter name and implementation are required")
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	if r.adapters == nil {
		r.adapters = make(map[string]MediaAdapter)
	}
	if _, exists := r.adapters[key]; exists {
		panic("duplicate media adapter: " + key)
	}
	r.adapters[key] = adapter
}

func (r *MediaAdapterRegistry) Resolve(name string) (MediaAdapter, error) {
	key := normalizeMediaAdapterName(name)
	r.mu.RLock()
	adapter, ok := r.adapters[key]
	r.mu.RUnlock()
	if !ok {
		return nil, ErrMediaAdapterNotFound
	}
	return adapter, nil
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
