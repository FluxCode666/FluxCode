package service

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/util/urlvalidator"
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

const (
	defaultProviderRequestMaxBytes       int64 = 32 << 20
	defaultProviderResponseBodyMaxBytes  int64 = 32 << 20
	defaultProviderStreamEventMaxBytes         = 2 << 20
	defaultProviderRequestTimeout              = 5 * time.Minute
	defaultProviderResponseHeaderTimeout       = 30 * time.Second
)

var (
	ErrProviderProtocolMismatch    = errors.New("provider route protocol mismatch")
	ErrUnsafeProviderDestination   = errors.New("unsafe provider destination")
	ErrProviderResponseTooLarge    = errors.New("provider response body is too large")
	ErrProviderRequestTooLarge     = errors.New("provider request body is too large")
	ErrProviderStreamEventTooLarge = errors.New("provider stream event is too large")
	ErrProviderSecureTransport     = errors.New("secure provider transport is unavailable")
	ErrProviderFeatureUnsupported  = errors.New("provider feature profile does not support request")
)

var lookupProviderHostIPs providerHostLookup = func(ctx context.Context, host string) ([]net.IP, error) {
	return net.DefaultResolver.LookupIP(ctx, "ip", host)
}

type ProviderForwarderOptions struct {
	RequestMaxBytes       int64
	ResponseBodyMaxBytes  int64
	StreamEventMaxBytes   int
	RequestTimeout        time.Duration
	ResponseHeaderTimeout time.Duration
	AllowInsecureHTTP     bool
}

func (o ProviderForwarderOptions) withDefaults() ProviderForwarderOptions {
	if o.RequestMaxBytes <= 0 {
		o.RequestMaxBytes = defaultProviderRequestMaxBytes
	}
	if o.ResponseBodyMaxBytes <= 0 {
		o.ResponseBodyMaxBytes = defaultProviderResponseBodyMaxBytes
	}
	if o.StreamEventMaxBytes <= 0 {
		o.StreamEventMaxBytes = defaultProviderStreamEventMaxBytes
	}
	if o.RequestTimeout <= 0 {
		o.RequestTimeout = defaultProviderRequestTimeout
	}
	if o.ResponseHeaderTimeout <= 0 {
		o.ResponseHeaderTimeout = defaultProviderResponseHeaderTimeout
	}
	return o
}

type ProviderForwarder struct {
	httpUpstream HTTPUpstream
	options      ProviderForwarderOptions
}

func NewProviderForwarder(httpUpstream HTTPUpstream, options ProviderForwarderOptions) *ProviderForwarder {
	return &ProviderForwarder{httpUpstream: httpUpstream, options: options.withDefaults()}
}

type ProviderForwardInput struct {
	Candidate RouteCandidate
	Body      []byte
	Headers   http.Header
}

type ProviderUsage struct {
	InputTokens           int
	OutputTokens          int
	CacheCreationTokens   int
	CacheReadTokens       int
	CacheCreation5mTokens int
	CacheCreation1hTokens int
	Raw                   json.RawMessage
	Complete              bool
}

type ProviderForwardResult struct {
	Route             RouteIdentity
	StatusCode        int
	Headers           http.Header
	Body              []byte
	Usage             ProviderUsage
	Duration          time.Duration
	UpstreamModel     string
	LogicalModel      string
	UpstreamRequestID string
}

type ProviderForwardStreamResult struct {
	Route             RouteIdentity
	StatusCode        int
	Headers           http.Header
	Body              io.ReadCloser
	Duration          time.Duration
	UpstreamModel     string
	LogicalModel      string
	UpstreamRequestID string
	usage             *providerStreamUsage
}

func (r *ProviderForwardStreamResult) Usage() ProviderUsage {
	if r == nil || r.usage == nil {
		return ProviderUsage{}
	}
	return r.usage.snapshot()
}

func (r *ProviderForwardStreamResult) ResponseID() string {
	if r == nil || r.usage == nil {
		return ""
	}
	return r.usage.responseIDSnapshot()
}

type ProviderUpstreamError struct {
	Route      RouteIdentity
	StatusCode int
	Retryable  bool
	Body       []byte
	Headers    http.Header
}

func (e *ProviderUpstreamError) Error() string {
	return "provider upstream returned an error"
}

func (f *ProviderForwarder) ForwardChat(ctx context.Context, input ProviderForwardInput) (*ProviderForwardResult, error) {
	return f.forwardNative(ctx, input, ProtocolChatCompletions)
}

func (f *ProviderForwarder) ForwardResponses(ctx context.Context, input ProviderForwardInput) (*ProviderForwardResult, error) {
	return f.forwardNative(ctx, input, ProtocolResponses)
}

func (f *ProviderForwarder) ForwardMessages(ctx context.Context, input ProviderForwardInput) (*ProviderForwardResult, error) {
	return f.forwardNative(ctx, input, ProtocolAnthropicMessages)
}

func (f *ProviderForwarder) ForwardEmbeddings(ctx context.Context, input ProviderForwardInput) (*ProviderForwardResult, error) {
	return f.forwardNative(ctx, input, ProtocolEmbeddings)
}

func (f *ProviderForwarder) ForwardChatStream(ctx context.Context, input ProviderForwardInput) (*ProviderForwardStreamResult, error) {
	return f.forwardNativeStream(ctx, input, ProtocolChatCompletions)
}

func (f *ProviderForwarder) ForwardResponsesStream(ctx context.Context, input ProviderForwardInput) (*ProviderForwardStreamResult, error) {
	return f.forwardNativeStream(ctx, input, ProtocolResponses)
}

func (f *ProviderForwarder) ForwardMessagesStream(ctx context.Context, input ProviderForwardInput) (*ProviderForwardStreamResult, error) {
	return f.forwardNativeStream(ctx, input, ProtocolAnthropicMessages)
}

func (f *ProviderForwarder) forwardNative(
	ctx context.Context,
	input ProviderForwardInput,
	protocol ProtocolFamily,
) (*ProviderForwardResult, error) {
	startedAt := time.Now()
	if f == nil || f.httpUpstream == nil {
		return nil, ErrProviderSecureTransport
	}
	candidate := input.Candidate
	if candidate.Tier != RouteTierNative ||
		candidate.Identity.IngressProtocol != protocol ||
		candidate.Identity.UpstreamProtocol != protocol ||
		candidate.Capability.Protocol != protocol {
		return nil, ErrProviderProtocolMismatch
	}
	if candidate.Profile == nil || candidate.Account == nil || candidate.Endpoint == nil {
		return nil, ErrNoProviderRoute
	}
	if int64(len(input.Body)) > f.options.RequestMaxBytes {
		return nil, ErrProviderRequestTooLarge
	}
	if err := validateProviderNativeRequest(protocol, candidate.Capability.FeatureProfile, input.Body); err != nil {
		return nil, err
	}

	connection, err := candidate.Endpoint.EffectiveConfig(candidate.Profile.Connection)
	if err != nil {
		return nil, fmt.Errorf("resolve provider endpoint: %w", err)
	}
	validatedURL, destinationIP, err := resolveProviderUpstreamTarget(ctx, connection.URL, f.options.AllowInsecureHTTP)
	if err != nil {
		return nil, err
	}
	if proxyURL := strings.TrimSpace(candidate.Account.EffectiveProxyURL()); proxyURL != "" {
		return nil, fmt.Errorf("%w: provider proxy routes require a destination-pinned transport", ErrProviderSecureTransport)
	}

	outboundBody, err := sjson.SetBytes(input.Body, "model", candidate.Capability.UpstreamModel)
	if err != nil {
		return nil, fmt.Errorf("rewrite provider model: %w", err)
	}
	attemptCtx, cancel := context.WithTimeout(ctx, f.options.RequestTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(attemptCtx, http.MethodPost, validatedURL, bytes.NewReader(outboundBody))
	if err != nil {
		return nil, fmt.Errorf("build provider request: %w", err)
	}
	// Provider requests are never automatically replayable. Route-aware retry is
	// decided above this layer using the attempt's commit state.
	req.GetBody = nil
	applyProviderRequestHeaders(req.Header, input.Headers, connection.Headers, protocol)
	if err := applyProviderAuthentication(req.Header, connection.AuthType, candidate.Account); err != nil {
		return nil, err
	}

	doer, ok := f.httpUpstream.(ProviderHTTPUpstream)
	if !ok || doer == nil {
		return nil, ErrProviderSecureTransport
	}
	resp, err := doer.DoProvider(req, ProviderUpstreamPolicy{
		ValidatedIP: destinationIP, ResponseHeaderTimeout: f.options.ResponseHeaderTimeout,
	})
	if err != nil {
		return nil, err
	}
	if resp == nil || resp.Body == nil {
		return nil, errors.New("provider returned an empty response")
	}
	defer func() { _ = resp.Body.Close() }()
	body, err := readProviderResponseBody(resp.Body, f.options.ResponseBodyMaxBytes)
	if err != nil {
		return nil, err
	}
	headers := providerResponseHeaders(resp.Header)
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return nil, &ProviderUpstreamError{
			Route: candidate.Identity, StatusCode: resp.StatusCode,
			Retryable: providerStatusRetryable(resp.StatusCode), Body: body, Headers: headers,
		}
	}
	if protocol == ProtocolEmbeddings {
		if err := validateProviderEmbeddingResponse(body); err != nil {
			return nil, err
		}
	}

	logicalModel := candidate.LogicalModel.Name
	if gjson.ValidBytes(body) && gjson.GetBytes(body, "model").Exists() {
		if rewritten, rewriteErr := sjson.SetBytes(body, "model", logicalModel); rewriteErr == nil {
			body = rewritten
		}
	}
	return &ProviderForwardResult{
		Route:             candidate.Identity,
		StatusCode:        resp.StatusCode,
		Headers:           headers,
		Body:              body,
		Usage:             parseProviderUsage(protocol, body),
		Duration:          time.Since(startedAt),
		UpstreamModel:     candidate.Capability.UpstreamModel,
		LogicalModel:      logicalModel,
		UpstreamRequestID: providerRequestID(resp.Header),
	}, nil
}

func (f *ProviderForwarder) forwardNativeStream(
	ctx context.Context,
	input ProviderForwardInput,
	protocol ProtocolFamily,
) (*ProviderForwardStreamResult, error) {
	startedAt := time.Now()
	if f == nil || f.httpUpstream == nil {
		return nil, ErrProviderSecureTransport
	}
	candidate := input.Candidate
	if candidate.Tier != RouteTierNative ||
		candidate.Identity.IngressProtocol != protocol ||
		candidate.Identity.UpstreamProtocol != protocol ||
		candidate.Capability.Protocol != protocol || protocol == ProtocolEmbeddings {
		return nil, ErrProviderProtocolMismatch
	}
	if candidate.Profile == nil || candidate.Account == nil || candidate.Endpoint == nil {
		return nil, ErrNoProviderRoute
	}
	if int64(len(input.Body)) > f.options.RequestMaxBytes {
		return nil, ErrProviderRequestTooLarge
	}
	if err := validateProviderNativeRequest(protocol, candidate.Capability.FeatureProfile, input.Body); err != nil {
		return nil, err
	}
	if !gjson.GetBytes(input.Body, "stream").Bool() {
		return nil, fmt.Errorf("%w: stream must be true", ErrProviderFeatureUnsupported)
	}

	connection, err := candidate.Endpoint.EffectiveConfig(candidate.Profile.Connection)
	if err != nil {
		return nil, fmt.Errorf("resolve provider endpoint: %w", err)
	}
	validatedURL, destinationIP, err := resolveProviderUpstreamTarget(ctx, connection.URL, f.options.AllowInsecureHTTP)
	if err != nil {
		return nil, err
	}
	if proxyURL := strings.TrimSpace(candidate.Account.EffectiveProxyURL()); proxyURL != "" {
		return nil, fmt.Errorf("%w: provider proxy routes require a destination-pinned transport", ErrProviderSecureTransport)
	}
	outboundBody, err := sjson.SetBytes(input.Body, "model", candidate.Capability.UpstreamModel)
	if err != nil {
		return nil, fmt.Errorf("rewrite provider model: %w", err)
	}
	suppressInjectedUsage := false
	if protocol == ProtocolChatCompletions {
		clientRequestedUsage := gjson.GetBytes(input.Body, "stream_options.include_usage").Bool()
		outboundBody, err = sjson.SetBytes(outboundBody, "stream_options.include_usage", true)
		if err != nil {
			return nil, fmt.Errorf("request provider stream usage: %w", err)
		}
		suppressInjectedUsage = !clientRequestedUsage
	}

	attemptCtx, cancel := context.WithTimeout(ctx, f.options.RequestTimeout)
	req, err := http.NewRequestWithContext(attemptCtx, http.MethodPost, validatedURL, bytes.NewReader(outboundBody))
	if err != nil {
		cancel()
		return nil, fmt.Errorf("build provider request: %w", err)
	}
	req.GetBody = nil
	applyProviderRequestHeaders(req.Header, input.Headers, connection.Headers, protocol)
	req.Header.Set("Accept", "text/event-stream")
	if err := applyProviderAuthentication(req.Header, connection.AuthType, candidate.Account); err != nil {
		cancel()
		return nil, err
	}
	doer, ok := f.httpUpstream.(ProviderHTTPUpstream)
	if !ok || doer == nil {
		cancel()
		return nil, ErrProviderSecureTransport
	}
	resp, err := doer.DoProvider(req, ProviderUpstreamPolicy{
		ValidatedIP: destinationIP, ResponseHeaderTimeout: f.options.ResponseHeaderTimeout,
	})
	if err != nil {
		cancel()
		return nil, err
	}
	if resp == nil || resp.Body == nil {
		cancel()
		return nil, errors.New("provider returned an empty response")
	}
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		defer func() { _ = resp.Body.Close() }()
		body, readErr := readProviderResponseBody(resp.Body, f.options.ResponseBodyMaxBytes)
		cancel()
		if readErr != nil {
			return nil, readErr
		}
		return nil, &ProviderUpstreamError{
			Route: candidate.Identity, StatusCode: resp.StatusCode,
			Retryable: providerStatusRetryable(resp.StatusCode), Body: body,
			Headers: providerResponseHeaders(resp.Header),
		}
	}

	usage := &providerStreamUsage{}
	body := newProviderSSEBody(
		resp.Body,
		cancel,
		protocol,
		candidate.LogicalModel.Name,
		f.options.ResponseBodyMaxBytes,
		f.options.StreamEventMaxBytes,
		usage,
		suppressInjectedUsage,
	)
	return &ProviderForwardStreamResult{
		Route:             candidate.Identity,
		StatusCode:        resp.StatusCode,
		Headers:           providerResponseHeaders(resp.Header),
		Body:              body,
		Duration:          time.Since(startedAt),
		UpstreamModel:     candidate.Capability.UpstreamModel,
		LogicalModel:      candidate.LogicalModel.Name,
		UpstreamRequestID: providerRequestID(resp.Header),
		usage:             usage,
	}, nil
}

type providerStreamUsage struct {
	mu         sync.RWMutex
	usage      ProviderUsage
	responseID string
}

func (u *providerStreamUsage) observe(protocol ProtocolFamily, payload []byte) {
	if u == nil || len(payload) == 0 || bytes.Equal(bytes.TrimSpace(payload), []byte("[DONE]")) {
		return
	}
	parsed := parseProviderUsage(protocol, payload)
	responseID := ""
	if protocol == ProtocolResponses {
		responseID = strings.TrimSpace(gjson.GetBytes(payload, "response.id").String())
		if responseID == "" {
			responseID = strings.TrimSpace(gjson.GetBytes(payload, "id").String())
		}
	}
	if protocol == ProtocolResponses && !parsed.Complete {
		if response := gjson.GetBytes(payload, "response"); response.Exists() && response.IsObject() {
			parsed = parseProviderUsage(protocol, []byte(response.Raw))
		}
	}
	u.mu.Lock()
	defer u.mu.Unlock()
	if responseID != "" {
		u.responseID = responseID
	}
	if protocol == ProtocolAnthropicMessages {
		if input := firstExistingJSONInt(payload, "usage.input_tokens", "message.usage.input_tokens"); input >= 0 {
			u.usage.InputTokens = input
		}
		if output := firstExistingJSONInt(payload, "usage.output_tokens", "message.usage.output_tokens"); output >= 0 {
			u.usage.OutputTokens = output
		}
		if u.usage.InputTokens > 0 || u.usage.OutputTokens > 0 {
			u.usage.Complete = true
		}
	}
	if parsed.CacheCreationTokens > 0 {
		u.usage.CacheCreationTokens = parsed.CacheCreationTokens
	}
	if parsed.CacheReadTokens > 0 {
		u.usage.CacheReadTokens = parsed.CacheReadTokens
	}
	if parsed.CacheCreation5mTokens > 0 {
		u.usage.CacheCreation5mTokens = parsed.CacheCreation5mTokens
	}
	if parsed.CacheCreation1hTokens > 0 {
		u.usage.CacheCreation1hTokens = parsed.CacheCreation1hTokens
	}
	if parsed.Complete {
		u.usage = parsed
	}
}

func (u *providerStreamUsage) snapshot() ProviderUsage {
	u.mu.RLock()
	defer u.mu.RUnlock()
	result := u.usage
	result.Raw = append(json.RawMessage(nil), result.Raw...)
	return result
}

func (u *providerStreamUsage) responseIDSnapshot() string {
	u.mu.RLock()
	defer u.mu.RUnlock()
	return u.responseID
}

func firstExistingJSONInt(body []byte, paths ...string) int {
	for _, path := range paths {
		value := gjson.GetBytes(body, path)
		if value.Exists() {
			return int(value.Int())
		}
	}
	return -1
}

type providerSSEBody struct {
	source                io.ReadCloser
	cancel                context.CancelFunc
	protocol              ProtocolFamily
	logicalModel          string
	maxTotal              int64
	maxEvent              int
	usage                 *providerStreamUsage
	suppressChatUsageOnly bool
	scanner               *bufio.Scanner
	buffer                bytes.Buffer
	total                 int64
	eventBytes            int
	closed                sync.Once
}

func newProviderSSEBody(
	source io.ReadCloser,
	cancel context.CancelFunc,
	protocol ProtocolFamily,
	logicalModel string,
	maxTotal int64,
	maxEvent int,
	usage *providerStreamUsage,
	suppressChatUsageOnly bool,
) *providerSSEBody {
	scanner := bufio.NewScanner(source)
	scanner.Buffer(make([]byte, 64*1024), maxEvent+1)
	return &providerSSEBody{
		source: source, cancel: cancel, protocol: protocol, logicalModel: logicalModel,
		maxTotal: maxTotal, maxEvent: maxEvent, usage: usage,
		suppressChatUsageOnly: suppressChatUsageOnly, scanner: scanner,
	}
}

func (b *providerSSEBody) Read(target []byte) (int, error) {
	for b.buffer.Len() == 0 {
		if !b.scanner.Scan() {
			if err := b.scanner.Err(); err != nil {
				return 0, fmt.Errorf("%w: %v", ErrProviderStreamEventTooLarge, err)
			}
			_ = b.Close()
			return 0, io.EOF
		}
		line := append([]byte(nil), b.scanner.Bytes()...)
		b.eventBytes += len(line) + 1
		if b.eventBytes > b.maxEvent {
			return 0, ErrProviderStreamEventTooLarge
		}
		if len(bytes.TrimSpace(line)) == 0 {
			b.eventBytes = 0
		}
		line = b.transformDataLine(line)
		if line == nil {
			continue
		}
		b.total += int64(len(line) + 1)
		if b.total > b.maxTotal {
			return 0, ErrProviderResponseTooLarge
		}
		b.buffer.Write(line)
		b.buffer.WriteByte('\n')
	}
	return b.buffer.Read(target)
}

func (b *providerSSEBody) transformDataLine(line []byte) []byte {
	trimmed := bytes.TrimSpace(line)
	if !bytes.HasPrefix(trimmed, []byte("data:")) {
		return line
	}
	payload := bytes.TrimSpace(bytes.TrimPrefix(trimmed, []byte("data:")))
	b.usage.observe(b.protocol, payload)
	if len(payload) == 0 || bytes.Equal(payload, []byte("[DONE]")) || !gjson.ValidBytes(payload) {
		return line
	}
	if b.protocol == ProtocolChatCompletions && b.suppressChatUsageOnly && providerChatUsageOnlyChunk(payload) {
		return nil
	}
	for _, path := range []string{"model", "response.model", "message.model"} {
		if gjson.GetBytes(payload, path).Exists() {
			if rewritten, err := sjson.SetBytes(payload, path, b.logicalModel); err == nil {
				payload = rewritten
			}
		}
	}
	return append([]byte("data: "), payload...)
}

func providerChatUsageOnlyChunk(payload []byte) bool {
	usage := gjson.GetBytes(payload, "usage")
	choices := gjson.GetBytes(payload, "choices")
	return usage.Exists() && usage.IsObject() && choices.Exists() && choices.IsArray() && len(choices.Array()) == 0
}

func (b *providerSSEBody) Close() error {
	var err error
	b.closed.Do(func() {
		b.cancel()
		err = b.source.Close()
	})
	return err
}

func validateProviderNativeRequest(protocol ProtocolFamily, profile ProviderFeatureProfile, body []byte) error {
	if !gjson.ValidBytes(body) || strings.TrimSpace(gjson.GetBytes(body, "model").String()) == "" {
		return ErrProviderFeatureUnsupported
	}
	stream := gjson.GetBytes(body, "stream").Bool()
	hasTools := gjson.GetBytes(body, "tools").Exists() || gjson.GetBytes(body, "tool_choice").Exists()
	if protocol == ProtocolEmbeddings {
		if profile != FeatureProfileEmbeddings || stream || hasTools || !gjson.GetBytes(body, "input").Exists() {
			return fmt.Errorf("%w: embeddings", ErrProviderFeatureUnsupported)
		}
		return nil
	}
	if !protocol.IsConversational() || profile == FeatureProfileEmbeddings {
		return fmt.Errorf("%w: protocol profile", ErrProviderFeatureUnsupported)
	}
	if stream && profile == FeatureProfileText {
		return fmt.Errorf("%w: stream", ErrProviderFeatureUnsupported)
	}
	if hasTools && profile != FeatureProfileTools {
		return fmt.Errorf("%w: tools", ErrProviderFeatureUnsupported)
	}
	return nil
}

func validateProviderEmbeddingResponse(body []byte) error {
	if !gjson.ValidBytes(body) {
		return fmt.Errorf("%w: embeddings response is not valid JSON", ErrProviderProtocolMismatch)
	}
	data := gjson.GetBytes(body, "data")
	if !data.Exists() || !data.IsArray() || len(data.Array()) == 0 {
		return fmt.Errorf("%w: embeddings response has no vectors", ErrProviderProtocolMismatch)
	}
	for _, item := range data.Array() {
		embedding := item.Get("embedding")
		if !embedding.Exists() || !embedding.IsArray() || len(embedding.Array()) == 0 {
			return fmt.Errorf("%w: embeddings response contains an invalid vector", ErrProviderProtocolMismatch)
		}
		for _, value := range embedding.Array() {
			if value.Type != gjson.Number {
				return fmt.Errorf("%w: embeddings vector contains a non-number", ErrProviderProtocolMismatch)
			}
		}
	}
	if !parseProviderUsage(ProtocolEmbeddings, body).Complete {
		return fmt.Errorf("%w: embeddings response usage is incomplete", ErrProviderProtocolMismatch)
	}
	return nil
}

func resolveProviderUpstreamTarget(ctx context.Context, rawURL string, allowInsecureHTTP bool) (string, net.IP, error) {
	validated, err := urlvalidator.ValidateHTTPURL(rawURL, allowInsecureHTTP, urlvalidator.ValidationOptions{})
	if err != nil {
		return "", nil, fmt.Errorf("%w: %v", ErrUnsafeProviderDestination, err)
	}
	parsed, err := url.Parse(validated)
	if err != nil {
		return "", nil, fmt.Errorf("%w: invalid URL", ErrUnsafeProviderDestination)
	}
	ips, err := lookupProviderHostIPs(ctx, parsed.Hostname())
	if err != nil || len(ips) == 0 {
		return "", nil, fmt.Errorf("%w: DNS resolution failed", ErrUnsafeProviderDestination)
	}
	for _, ip := range ips {
		if err := validateProviderDestinationIP(ip); err != nil {
			return "", nil, err
		}
	}
	return validated, append(net.IP(nil), ips[0]...), nil
}

func validateProviderDestinationIP(ip net.IP) error {
	if ip == nil || ip.IsUnspecified() || ip.IsLoopback() || ip.IsPrivate() ||
		ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() || ip.IsMulticast() {
		return fmt.Errorf("%w: prohibited IP", ErrUnsafeProviderDestination)
	}
	_, carrierNAT, _ := net.ParseCIDR("100.64.0.0/10")
	if carrierNAT.Contains(ip) || ip.Equal(net.ParseIP("169.254.169.254")) {
		return fmt.Errorf("%w: prohibited IP", ErrUnsafeProviderDestination)
	}
	return nil
}

func applyProviderRequestHeaders(target, incoming http.Header, configured map[string]string, protocol ProtocolFamily) {
	target.Set("Content-Type", "application/json")
	target.Set("Accept", "application/json")
	for name, values := range incoming {
		canonical := http.CanonicalHeaderKey(name)
		if !providerPassthroughHeader(canonical, protocol) {
			continue
		}
		for _, value := range values {
			target.Add(canonical, value)
		}
	}
	for name, value := range configured {
		if !IsAllowedProviderHeader(name) {
			continue
		}
		target.Set(http.CanonicalHeaderKey(name), value)
	}
}

func providerPassthroughHeader(name string, protocol ProtocolFamily) bool {
	switch name {
	case "Accept", "Content-Type", "Idempotency-Key", "Openai-Beta":
		return true
	case "Anthropic-Version", "Anthropic-Beta":
		return protocol == ProtocolAnthropicMessages
	default:
		return false
	}
}

func applyProviderAuthentication(headers http.Header, authType string, account *Account) error {
	credential := strings.TrimSpace(account.GetCredential("api_key"))
	if credential == "" {
		credential = strings.TrimSpace(account.GetCredential("access_token"))
	}
	switch strings.ToLower(strings.TrimSpace(authType)) {
	case "", "bearer":
		if credential == "" {
			return errors.New("provider credential is missing")
		}
		headers.Set("Authorization", "Bearer "+credential)
	case "x-api-key", "anthropic_api_key":
		if credential == "" {
			return errors.New("provider credential is missing")
		}
		headers.Set("X-Api-Key", credential)
	case "none":
		return nil
	default:
		return fmt.Errorf("unsupported provider auth type %q", authType)
	}
	return nil
}

func readProviderResponseBody(reader io.Reader, limit int64) ([]byte, error) {
	limited := &io.LimitedReader{R: reader, N: limit + 1}
	body, err := io.ReadAll(limited)
	if err != nil {
		return nil, err
	}
	if int64(len(body)) > limit {
		return nil, ErrProviderResponseTooLarge
	}
	return body, nil
}

func providerResponseHeaders(source http.Header) http.Header {
	target := make(http.Header)
	for name, values := range source {
		switch strings.ToLower(name) {
		case "content-type", "request-id", "x-request-id", "retry-after", "openai-processing-ms":
			for _, value := range values {
				target.Add(name, value)
			}
		}
	}
	return target
}

func providerRequestID(headers http.Header) string {
	if value := strings.TrimSpace(headers.Get("X-Request-ID")); value != "" {
		return value
	}
	return strings.TrimSpace(headers.Get("Request-ID"))
}

func providerStatusRetryable(status int) bool {
	return status == http.StatusRequestTimeout || status == http.StatusTooManyRequests || status >= http.StatusInternalServerError
}

func parseProviderUsage(protocol ProtocolFamily, body []byte) ProviderUsage {
	usagePath := "usage"
	usage := gjson.GetBytes(body, usagePath)
	if !usage.Exists() || !usage.IsObject() {
		return ProviderUsage{}
	}
	result := ProviderUsage{Raw: json.RawMessage(append([]byte(nil), []byte(usage.Raw)...))}
	switch protocol {
	case ProtocolChatCompletions:
		input, output := usage.Get("prompt_tokens"), usage.Get("completion_tokens")
		if input.Exists() && output.Exists() {
			result.InputTokens, result.OutputTokens, result.Complete = int(input.Int()), int(output.Int()), true
		}
	case ProtocolResponses, ProtocolAnthropicMessages:
		input, output := usage.Get("input_tokens"), usage.Get("output_tokens")
		if input.Exists() && output.Exists() {
			result.InputTokens, result.OutputTokens, result.Complete = int(input.Int()), int(output.Int()), true
		}
	case ProtocolEmbeddings:
		input := usage.Get("prompt_tokens")
		if input.Exists() {
			result.InputTokens, result.Complete = int(input.Int()), true
		}
	}
	result.CacheReadTokens = maxProviderUsageValue(0, firstExistingJSONInt(body,
		"usage.input_tokens_details.cached_tokens",
		"usage.prompt_tokens_details.cached_tokens",
		"usage.cache_read_input_tokens",
		"usage.cached_tokens",
	))
	result.CacheCreationTokens = maxProviderUsageValue(0, firstExistingJSONInt(body,
		"usage.input_tokens_details.cache_creation_tokens",
		"usage.prompt_tokens_details.cache_creation_tokens",
		"usage.cache_creation_input_tokens",
		"usage.cache_creation_tokens",
		"usage.cache_write_tokens",
	))
	result.CacheCreation5mTokens = maxProviderUsageValue(0, firstExistingJSONInt(body,
		"usage.cache_creation.ephemeral_5m_input_tokens",
	))
	result.CacheCreation1hTokens = maxProviderUsageValue(0, firstExistingJSONInt(body,
		"usage.cache_creation.ephemeral_1h_input_tokens",
	))
	return result
}

func maxProviderUsageValue(left, right int) int {
	if right > left {
		return right
	}
	return left
}
