package service

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"math"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/util/urlvalidator"
	"github.com/tidwall/sjson"
)

const (
	defaultEmbeddingRequestMaxBytes   int64 = config.EmbeddingRequestMaxBytesHardLimit
	defaultEmbeddingResponseMaxBytes  int64 = config.EmbeddingResponseMaxBytesHardLimit
	defaultEmbeddingMaxJSONDepth            = config.EmbeddingMaxJSONDepthHardLimit
	defaultEmbeddingMaxInputItems           = config.EmbeddingMaxInputItemsHardLimit
	defaultEmbeddingMaxInputItemBytes       = config.EmbeddingMaxInputItemBytesHardLimit
	defaultEmbeddingMaxTokenValue     int64 = config.EmbeddingMaxTokenValueHardLimit
	defaultEmbeddingTimeout                 = config.EmbeddingUpstreamTimeoutSecondsHardLimit * time.Second
	defaultEmbeddingHeaderTimeout           = config.EmbeddingResponseHeaderTimeoutSecHardLimit * time.Second
	defaultEmbeddingMaxConcurrent           = config.EmbeddingMaxConcurrentRequestsHardLimit
	embeddingPoolModeRetryDelay             = 500 * time.Millisecond
)

var lookupEmbeddingHostIP = func(ctx context.Context, host string) ([]net.IP, error) {
	return net.DefaultResolver.LookupIP(ctx, "ip", host)
}

var (
	ErrEmbeddingRequestInvalid  = errors.New("invalid embedding request")
	ErrEmbeddingUnavailable     = errors.New("embedding model is unavailable")
	ErrEmbeddingUpstream        = errors.New("embedding upstream request failed")
	ErrEmbeddingUnsupportedMode = errors.New("embedding is unavailable in simple run mode")
)

// EmbeddingForwardError intentionally exposes only a stable category and
// status. The upstream response body, headers, credentials and vectors never
// cross this error boundary.
type EmbeddingForwardError struct {
	Category      string
	StatusCode    int
	Retryable     bool
	AccountID     int64
	ChannelID     int64
	UpstreamModel string
}

func (e *EmbeddingForwardError) Error() string {
	return ErrEmbeddingUpstream.Error()
}

// EmbeddingForwardInput contains the small amount of request metadata needed
// before U4 performs the synchronous billing transaction.
type EmbeddingForwardInput struct {
	GroupID *int64
	Body    []byte
}

// EmbeddingForwardResult is deliberately body-only: callers can return the
// OpenAI-compatible payload after successful billing without retaining it in
// logs, operations context, idempotency storage or usage records.
type EmbeddingForwardResult struct {
	Body         []byte
	Headers      http.Header
	PromptTokens int
	Eligibility  EmbeddingModelEligibility
	Duration     time.Duration
}

type embeddingForwardLimits struct {
	requestMaxBytes    int64
	responseMaxBytes   int64
	maxJSONDepth       int
	maxInputItems      int
	maxInputItemBytes  int
	maxTokenValue      int64
	timeout            time.Duration
	headerTimeout      time.Duration
	maxConcurrent      int
	allowedHosts       []string
	allowedPrivateCIDR []string
}

func (s *OpenAIGatewayService) embeddingLimits() embeddingForwardLimits {
	if s == nil {
		return embeddingLimitsFromConfig(nil)
	}
	return embeddingLimitsFromConfig(s.cfg)
}

func embeddingLimitsFromConfig(cfg *config.Config) embeddingForwardLimits {
	limits := embeddingForwardLimits{
		requestMaxBytes:   defaultEmbeddingRequestMaxBytes,
		responseMaxBytes:  defaultEmbeddingResponseMaxBytes,
		maxJSONDepth:      defaultEmbeddingMaxJSONDepth,
		maxInputItems:     defaultEmbeddingMaxInputItems,
		maxInputItemBytes: defaultEmbeddingMaxInputItemBytes,
		maxTokenValue:     defaultEmbeddingMaxTokenValue,
		timeout:           defaultEmbeddingTimeout,
		headerTimeout:     defaultEmbeddingHeaderTimeout,
		maxConcurrent:     defaultEmbeddingMaxConcurrent,
	}
	if cfg == nil {
		return limits
	}
	configured := cfg.Gateway.Embedding
	if configured.RequestMaxBytes > 0 {
		limits.requestMaxBytes = min(configured.RequestMaxBytes, config.EmbeddingRequestMaxBytesHardLimit)
	}
	if configured.ResponseMaxBytes > 0 {
		limits.responseMaxBytes = min(configured.ResponseMaxBytes, config.EmbeddingResponseMaxBytesHardLimit)
	}
	if configured.MaxJSONDepth > 0 {
		limits.maxJSONDepth = min(configured.MaxJSONDepth, config.EmbeddingMaxJSONDepthHardLimit)
	}
	if configured.MaxInputItems > 0 {
		limits.maxInputItems = min(configured.MaxInputItems, config.EmbeddingMaxInputItemsHardLimit)
	}
	if configured.MaxInputItemBytes > 0 {
		limits.maxInputItemBytes = min(configured.MaxInputItemBytes, config.EmbeddingMaxInputItemBytesHardLimit)
	}
	if configured.MaxTokenValue > 0 {
		limits.maxTokenValue = min(configured.MaxTokenValue, config.EmbeddingMaxTokenValueHardLimit)
	}
	if configured.UpstreamTimeoutSeconds > 0 {
		limits.timeout = time.Duration(min(configured.UpstreamTimeoutSeconds, config.EmbeddingUpstreamTimeoutSecondsHardLimit)) * time.Second
	}
	if configured.ResponseHeaderTimeoutSec > 0 {
		limits.headerTimeout = time.Duration(min(configured.ResponseHeaderTimeoutSec, config.EmbeddingResponseHeaderTimeoutSecHardLimit)) * time.Second
	}
	if configured.MaxConcurrentRequests > 0 {
		limits.maxConcurrent = min(configured.MaxConcurrentRequests, config.EmbeddingMaxConcurrentRequestsHardLimit)
	}
	limits.allowedHosts = append([]string(nil), configured.AllowedHosts...)
	limits.allowedPrivateCIDR = append([]string(nil), configured.AllowedPrivateCIDRs...)
	return limits
}

func (s *OpenAIGatewayService) acquireEmbeddingForwardSlot() (func(), bool) {
	if s == nil {
		return nil, false
	}
	s.embeddingForwardOnce.Do(func() {
		s.embeddingForwardSem = make(chan struct{}, s.embeddingLimits().maxConcurrent)
	})
	select {
	case s.embeddingForwardSem <- struct{}{}:
		return func() { <-s.embeddingForwardSem }, true
	default:
		return nil, false
	}
}

// ForwardEmbeddings performs the non-streaming, non-billing half of the
// embedding flow. It reads neither inbound Authorization nor upstream error
// bodies, and it returns a vector only after strict usage validation succeeds.
func (s *OpenAIGatewayService) ForwardEmbeddings(ctx context.Context, input EmbeddingForwardInput) (*EmbeddingForwardResult, error) {
	if s != nil && s.cfg != nil && s.cfg.RunMode == config.RunModeSimple {
		return nil, ErrEmbeddingUnsupportedMode
	}
	if input.GroupID == nil {
		return nil, ErrEmbeddingUnavailable
	}
	limits := s.embeddingLimits()
	publicModel, err := validateEmbeddingRequest(input.Body, limits)
	if err != nil {
		return nil, ErrEmbeddingRequestInvalid
	}

	releaseForward, acquired := s.acquireEmbeddingForwardSlot()
	if !acquired {
		return nil, &EmbeddingForwardError{Category: "concurrency_limit"}
	}
	defer releaseForward()

	candidates, err := s.ResolveEmbeddingModelEligibility(ctx, input.GroupID, publicModel)
	if err != nil || len(candidates) == 0 {
		return nil, ErrEmbeddingUnavailable
	}

	maxAttempts := len(candidates)
	if s.cfg != nil && s.cfg.Gateway.MaxAccountSwitches > 0 && s.cfg.Gateway.MaxAccountSwitches+1 < maxAttempts {
		maxAttempts = s.cfg.Gateway.MaxAccountSwitches + 1
	}
	eligibleAccountIDs := make(map[int64]struct{}, len(candidates))
	for i := range candidates {
		eligibleAccountIDs[candidates[i].Account.ID] = struct{}{}
	}
	excluded := make(map[int64]struct{}, len(candidates))
	var lastErr error
	for attempts := 0; attempts < maxAttempts; {
		selection, selectErr := s.SelectAccountWithLoadAwarenessForPlatform(
			ctx,
			PlatformEmbedding,
			input.GroupID,
			"",
			publicModel,
			excluded,
		)
		if selectErr != nil || selection == nil || selection.Account == nil || !selection.Acquired || selection.ReleaseFunc == nil {
			break
		}
		if _, alreadyExcluded := excluded[selection.Account.ID]; alreadyExcluded {
			selection.ReleaseFunc()
			break
		}
		if _, eligible := eligibleAccountIDs[selection.Account.ID]; !eligible {
			selection.ReleaseFunc()
			excluded[selection.Account.ID] = struct{}{}
			continue
		}

		freshCandidates := s.resolveEmbeddingModelEligibilityFromAccounts(ctx, input.GroupID, publicModel, []Account{*selection.Account})
		if len(freshCandidates) != 1 {
			selection.ReleaseFunc()
			excluded[selection.Account.ID] = struct{}{}
			continue
		}
		candidate := freshCandidates[0]
		attempts++
		poolRetryLimit := 0
		if candidate.Account.IsPoolMode() {
			poolRetryLimit = candidate.Account.GetPoolModeRetryCount()
		}
		var result *EmbeddingForwardResult
		var forwardErr error
		for poolRetryCount := 0; ; poolRetryCount++ {
			result, forwardErr = s.forwardEmbeddingCandidate(ctx, input.Body, candidate, limits)
			if forwardErr == nil {
				break
			}
			if typed, ok := forwardErr.(*EmbeddingForwardError); ok {
				typed.AccountID = candidate.Account.ID
				typed.ChannelID = candidate.ChannelMapping.ChannelID
				typed.UpstreamModel = candidate.UpstreamModel
			}
			upstreamErr, ok := forwardErr.(*EmbeddingForwardError)
			if !ok || !upstreamErr.Retryable || !isPoolModeRetryableStatus(upstreamErr.StatusCode) || poolRetryCount >= poolRetryLimit {
				break
			}
			if !waitEmbeddingPoolRetry(ctx) {
				selection.ReleaseFunc()
				return nil, ctx.Err()
			}
		}
		selection.ReleaseFunc()
		if forwardErr == nil {
			return result, nil
		}
		lastErr = forwardErr
		upstreamErr, retryable := forwardErr.(*EmbeddingForwardError)
		if !retryable || !upstreamErr.Retryable {
			return nil, forwardErr
		}
		excluded[selection.Account.ID] = struct{}{}
	}
	if lastErr != nil {
		return nil, lastErr
	}
	return nil, ErrEmbeddingUnavailable
}

func waitEmbeddingPoolRetry(ctx context.Context) bool {
	timer := time.NewTimer(embeddingPoolModeRetryDelay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-timer.C:
		return true
	}
}

func (s *OpenAIGatewayService) forwardEmbeddingCandidate(
	ctx context.Context,
	body []byte,
	candidate EmbeddingModelEligibility,
	limits embeddingForwardLimits,
) (*EmbeddingForwardResult, error) {
	attemptCtx, cancel := context.WithTimeout(ctx, limits.timeout)
	defer cancel()
	startedAt := time.Now()
	if strings.TrimSpace(candidate.Account.EffectiveProxyURL()) != "" {
		return nil, &EmbeddingForwardError{Category: "proxy_not_supported", Retryable: true}
	}
	targetURL, destinationIP, err := resolveEmbeddingUpstreamTarget(attemptCtx, candidate.Account.GetEmbeddingBaseURL(), limits)
	if err != nil {
		return nil, &EmbeddingForwardError{Category: "unsafe_upstream", Retryable: true}
	}

	outboundBody, err := sjson.SetBytes(body, "model", candidate.UpstreamModel)
	if err != nil {
		return nil, ErrEmbeddingRequestInvalid
	}
	req, err := http.NewRequestWithContext(attemptCtx, http.MethodPost, targetURL, bytes.NewReader(outboundBody))
	if err != nil {
		return nil, &EmbeddingForwardError{Category: "request_build"}
	}
	// bytes.Reader normally gives net/http a GetBody replay hook. Embedding
	// deliberately removes it so no middleware or future transport path can
	// automatically replay an upstream-billed request.
	req.GetBody = nil
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Authorization", "Bearer "+candidate.Account.GetEmbeddingAPIKey())

	doer, ok := s.httpUpstream.(EmbeddingHTTPUpstream)
	if !ok || doer == nil {
		return nil, &EmbeddingForwardError{Category: "secure_transport_unavailable"}
	}
	resp, err := doer.DoEmbedding(req, EmbeddingUpstreamPolicy{
		ValidatedIP:           destinationIP,
		ResponseHeaderTimeout: limits.headerTimeout,
	})
	if err != nil {
		var transportErr *EmbeddingTransportError
		if errors.As(err, &transportErr) && transportErr.RequestNotWritten {
			return nil, &EmbeddingForwardError{Category: "transport_not_written", Retryable: true}
		}
		// Once a connection was handed to HTTP, net/http cannot prove that zero
		// request bytes were written. Treat the result as unknown and never replay.
		return nil, &EmbeddingForwardError{Category: "transport_unknown"}
	}
	if resp == nil || resp.Body == nil {
		return nil, &EmbeddingForwardError{Category: "empty_response"}
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		_, _ = readUpstreamResponseBodyLimited(resp.Body, limits.responseMaxBytes)
		return nil, &EmbeddingForwardError{
			Category:   embeddingStatusCategory(resp.StatusCode),
			StatusCode: resp.StatusCode,
			Retryable:  resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden || resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= http.StatusInternalServerError,
		}
	}

	responseBody, err := readUpstreamResponseBodyLimited(resp.Body, limits.responseMaxBytes)
	if err != nil {
		return nil, &EmbeddingForwardError{Category: "response_read"}
	}
	promptTokens, err := parseEmbeddingPromptTokens(responseBody, limits.maxJSONDepth)
	if err != nil {
		// A malformed 2xx has an uncertain semantic result. Do not fail over or
		// return the vector, because the upstream may already have charged it.
		return nil, &EmbeddingForwardError{Category: "invalid_usage"}
	}
	if err := validateEmbeddingResponseData(responseBody); err != nil {
		return nil, &EmbeddingForwardError{Category: "invalid_response"}
	}
	publicResponse, err := sjson.SetBytes(responseBody, "model", candidate.PublicModel)
	if err != nil {
		return nil, &EmbeddingForwardError{Category: "invalid_response"}
	}

	return &EmbeddingForwardResult{
		Body:         publicResponse,
		Headers:      embeddingResponseHeaders(resp.Header),
		PromptTokens: promptTokens,
		Eligibility:  candidate,
		Duration:     time.Since(startedAt),
	}, nil
}

func validateEmbeddingRequest(body []byte, limits embeddingForwardLimits) (string, error) {
	if len(body) == 0 || int64(len(body)) > limits.requestMaxBytes {
		return "", errors.New("request body size is invalid")
	}
	if err := validateEmbeddingJSONDepth(body, limits.maxJSONDepth); err != nil {
		return "", err
	}
	var envelope map[string]json.RawMessage
	if err := json.Unmarshal(body, &envelope); err != nil {
		return "", err
	}
	modelRaw, ok := envelope["model"]
	if !ok {
		return "", errors.New("model is required")
	}
	var model string
	if err := json.Unmarshal(modelRaw, &model); err != nil || strings.TrimSpace(model) == "" {
		return "", errors.New("model must be a non-empty string")
	}
	inputRaw, ok := envelope["input"]
	if !ok {
		return "", errors.New("input is required")
	}
	decoder := json.NewDecoder(bytes.NewReader(inputRaw))
	decoder.UseNumber()
	var input any
	if err := decoder.Decode(&input); err != nil {
		return "", errors.New("input has invalid JSON")
	}
	if err := validateEmbeddingInput(input, limits); err != nil {
		return "", err
	}
	return strings.TrimSpace(model), nil
}

func validateEmbeddingInput(input any, limits embeddingForwardLimits) error {
	switch value := input.(type) {
	case string:
		if len(value) > limits.maxInputItemBytes {
			return errors.New("input item is too large")
		}
		return nil
	case []any:
		if len(value) > limits.maxInputItems {
			return errors.New("input has too many items")
		}
		if len(value) == 0 {
			return nil
		}
		switch value[0].(type) {
		case string:
			for _, item := range value {
				text, ok := item.(string)
				if !ok || len(text) > limits.maxInputItemBytes {
					return errors.New("input must be a bounded string array")
				}
			}
			return nil
		case json.Number:
			return validateEmbeddingTokenArray(value, limits.maxInputItems, limits.maxTokenValue)
		case []any:
			total := 0
			for _, item := range value {
				tokens, ok := item.([]any)
				if !ok {
					return errors.New("input must not mix token array shapes")
				}
				if err := validateEmbeddingTokenArray(tokens, limits.maxInputItems-total, limits.maxTokenValue); err != nil {
					return err
				}
				total += len(tokens)
			}
			return nil
		default:
			return errors.New("input has an unsupported shape")
		}
	default:
		return errors.New("input has an unsupported shape")
	}
}

func validateEmbeddingTokenArray(values []any, remaining int, maxTokenValue int64) error {
	if remaining < 0 || len(values) > remaining {
		return errors.New("input has too many token values")
	}
	for _, value := range values {
		number, ok := value.(json.Number)
		if !ok {
			return errors.New("token input must contain integers")
		}
		parsed, err := number.Int64()
		if err != nil || parsed < 0 || parsed > maxTokenValue {
			return errors.New("token input must contain bounded non-negative integers")
		}
	}
	return nil
}

func parseEmbeddingPromptTokens(body []byte, maxDepth int) (int, error) {
	if err := validateEmbeddingJSONDepth(body, maxDepth); err != nil {
		return 0, err
	}
	var envelope struct {
		Usage json.RawMessage `json:"usage"`
	}
	if err := json.Unmarshal(body, &envelope); err != nil || len(envelope.Usage) == 0 {
		return 0, errors.New("usage is required")
	}
	var usage struct {
		PromptTokens json.RawMessage `json:"prompt_tokens"`
	}
	if err := json.Unmarshal(envelope.Usage, &usage); err != nil || len(usage.PromptTokens) == 0 {
		return 0, errors.New("usage.prompt_tokens is required")
	}
	rawPromptTokens := bytes.TrimSpace(usage.PromptTokens)
	if len(rawPromptTokens) == 0 || rawPromptTokens[0] == '"' || rawPromptTokens[0] == 'n' {
		return 0, errors.New("usage.prompt_tokens must be a JSON number")
	}
	decoder := json.NewDecoder(bytes.NewReader(rawPromptTokens))
	decoder.UseNumber()
	var value json.Number
	if err := decoder.Decode(&value); err != nil {
		return 0, errors.New("usage.prompt_tokens must be an integer")
	}
	promptTokens, err := value.Int64()
	if err != nil || promptTokens <= 0 || promptTokens > math.MaxInt {
		return 0, errors.New("usage.prompt_tokens must be a positive integer")
	}
	return int(promptTokens), nil
}

func validateEmbeddingResponseData(body []byte) error {
	var envelope struct {
		Data []struct {
			Embedding json.RawMessage `json:"embedding"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &envelope); err != nil || len(envelope.Data) == 0 {
		return errors.New("embedding data is required")
	}
	for _, item := range envelope.Data {
		raw := bytes.TrimSpace(item.Embedding)
		if len(raw) == 0 {
			return errors.New("embedding value is required")
		}
		switch raw[0] {
		case '[':
			var vector []float64
			if err := json.Unmarshal(raw, &vector); err != nil || len(vector) == 0 {
				return errors.New("embedding vector is invalid")
			}
			for _, value := range vector {
				if math.IsNaN(value) || math.IsInf(value, 0) {
					return errors.New("embedding vector is invalid")
				}
			}
		case '"':
			var encoded string
			if err := json.Unmarshal(raw, &encoded); err != nil || encoded == "" {
				return errors.New("embedding base64 is invalid")
			}
			if _, err := base64.StdEncoding.Strict().DecodeString(encoded); err != nil {
				return errors.New("embedding base64 is invalid")
			}
		default:
			return errors.New("embedding value has an unsupported format")
		}
	}
	return nil
}

func validateEmbeddingJSONDepth(body []byte, maxDepth int) error {
	depth := 0
	inString := false
	escaped := false
	for _, ch := range body {
		if inString {
			if escaped {
				escaped = false
				continue
			}
			switch ch {
			case '\\':
				escaped = true
			case '"':
				inString = false
			}
			continue
		}
		switch ch {
		case '"':
			inString = true
		case '{', '[':
			depth++
			if depth > maxDepth {
				return errors.New("JSON nesting exceeds the embedding limit")
			}
		case '}', ']':
			depth--
			if depth < 0 {
				return errors.New("JSON nesting is invalid")
			}
		}
	}
	if inString || depth != 0 {
		return errors.New("JSON structure is invalid")
	}
	return nil
}

func resolveEmbeddingUpstreamTarget(ctx context.Context, rawBaseURL string, limits embeddingForwardLimits) (string, net.IP, error) {
	if len(limits.allowedHosts) == 0 {
		return "", nil, errors.New("embedding allowed_hosts is not configured")
	}
	normalized, err := urlvalidator.ValidateHTTPSURL(rawBaseURL, urlvalidator.ValidationOptions{
		AllowedHosts:     limits.allowedHosts,
		RequireAllowlist: true,
		AllowPrivate:     true,
	})
	if err != nil {
		return "", nil, err
	}
	parsed, err := url.Parse(normalized)
	if err != nil {
		return "", nil, err
	}
	ips, err := lookupEmbeddingHostIP(ctx, parsed.Hostname())
	if err != nil || len(ips) == 0 {
		return "", nil, errors.New("embedding upstream DNS resolution failed")
	}
	allowedPrivateNets, err := parseEmbeddingPrivateCIDRs(limits.allowedPrivateCIDR)
	if err != nil {
		return "", nil, err
	}
	for _, ip := range ips {
		if !isAllowedEmbeddingIP(ip, allowedPrivateNets) {
			return "", nil, errors.New("embedding upstream resolved to a blocked address")
		}
	}
	return buildOpenAIEmbeddingsURL(normalized), ips[0], nil
}

func parseEmbeddingPrivateCIDRs(rawCIDRs []string) ([]*net.IPNet, error) {
	result := make([]*net.IPNet, 0, len(rawCIDRs))
	for _, raw := range rawCIDRs {
		_, cidr, err := net.ParseCIDR(strings.TrimSpace(raw))
		if err != nil {
			return nil, errors.New("embedding private CIDR is invalid")
		}
		result = append(result, cidr)
	}
	return result, nil
}

func isAllowedEmbeddingIP(ip net.IP, allowedPrivateNets []*net.IPNet) bool {
	if ip == nil || ip.IsLoopback() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() || ip.IsUnspecified() || ip.IsMulticast() || !ip.IsGlobalUnicast() {
		return false
	}
	for _, blocked := range blockedEmbeddingNetworks {
		if blocked.Contains(ip) {
			return false
		}
	}
	if !ip.IsPrivate() {
		return true
	}
	for _, cidr := range allowedPrivateNets {
		if cidr.Contains(ip) {
			return true
		}
	}
	return false
}

var blockedEmbeddingNetworks = mustParseEmbeddingNetworks(
	"100.64.0.0/10",   // carrier-grade NAT/shared address space
	"192.0.0.0/24",    // IETF protocol assignments
	"192.0.2.0/24",    // documentation
	"198.18.0.0/15",   // benchmarking
	"198.51.100.0/24", // documentation
	"203.0.113.0/24",  // documentation
	"240.0.0.0/4",     // reserved
	"2001:db8::/32",   // documentation
)

func mustParseEmbeddingNetworks(rawCIDRs ...string) []*net.IPNet {
	networks := make([]*net.IPNet, 0, len(rawCIDRs))
	for _, raw := range rawCIDRs {
		_, network, err := net.ParseCIDR(raw)
		if err != nil {
			panic("invalid built-in embedding CIDR: " + raw)
		}
		networks = append(networks, network)
	}
	return networks
}

// buildOpenAIEmbeddingsURL accepts an upstream root, /v1 root, or a complete
// embeddings endpoint and produces exactly one /v1/embeddings suffix.
func buildOpenAIEmbeddingsURL(base string) string {
	parsed, err := url.Parse(strings.TrimSpace(base))
	if err != nil {
		return ""
	}
	path := strings.TrimRight(parsed.Path, "/")
	switch {
	case strings.HasSuffix(path, "/v1/embeddings"):
	case strings.HasSuffix(path, "/v1"):
		path += "/embeddings"
	default:
		path += "/v1/embeddings"
	}
	parsed.Path = path
	parsed.RawPath = ""
	parsed.RawQuery = ""
	parsed.Fragment = ""
	return parsed.String()
}

func embeddingStatusCategory(statusCode int) string {
	switch {
	case statusCode == http.StatusUnauthorized || statusCode == http.StatusForbidden:
		return "upstream_auth"
	case statusCode == http.StatusTooManyRequests:
		return "upstream_rate_limited"
	case statusCode >= http.StatusInternalServerError:
		return "upstream_server"
	default:
		return "upstream_rejected"
	}
}

func embeddingResponseHeaders(upstream http.Header) http.Header {
	headers := make(http.Header)
	contentType := "application/json"
	if upstream != nil && strings.HasPrefix(strings.ToLower(upstream.Get("Content-Type")), "application/json") {
		contentType = upstream.Get("Content-Type")
	}
	headers.Set("Content-Type", contentType)
	return headers
}
