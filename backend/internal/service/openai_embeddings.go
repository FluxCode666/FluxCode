package service

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
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
	defaultEmbeddingRequestMaxBytes   int64 = 1 * 1024 * 1024
	defaultEmbeddingResponseMaxBytes  int64 = 8 * 1024 * 1024
	defaultEmbeddingMaxJSONDepth            = 32
	defaultEmbeddingMaxInputItems           = 2048
	defaultEmbeddingMaxInputItemBytes       = 64 * 1024
	defaultEmbeddingMaxTokenValue     int64 = 2147483647
	defaultEmbeddingTimeout                 = 60 * time.Second
	defaultEmbeddingHeaderTimeout           = 30 * time.Second
	defaultEmbeddingMaxConcurrent           = 128
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
	Category   string
	StatusCode int
	Retryable  bool
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
	if s == nil || s.cfg == nil {
		return limits
	}
	configured := s.cfg.Gateway.Embedding
	if configured.RequestMaxBytes > 0 {
		limits.requestMaxBytes = configured.RequestMaxBytes
	}
	if configured.ResponseMaxBytes > 0 {
		limits.responseMaxBytes = configured.ResponseMaxBytes
	}
	if configured.MaxJSONDepth > 0 {
		limits.maxJSONDepth = configured.MaxJSONDepth
	}
	if configured.MaxInputItems > 0 {
		limits.maxInputItems = configured.MaxInputItems
	}
	if configured.MaxInputItemBytes > 0 {
		limits.maxInputItemBytes = configured.MaxInputItemBytes
	}
	if configured.MaxTokenValue > 0 && configured.MaxTokenValue <= defaultEmbeddingMaxTokenValue {
		limits.maxTokenValue = configured.MaxTokenValue
	}
	if configured.UpstreamTimeoutSeconds > 0 {
		limits.timeout = time.Duration(configured.UpstreamTimeoutSeconds) * time.Second
	}
	if configured.ResponseHeaderTimeoutSec > 0 {
		limits.headerTimeout = time.Duration(configured.ResponseHeaderTimeoutSec) * time.Second
	}
	if configured.MaxConcurrentRequests > 0 {
		limits.maxConcurrent = configured.MaxConcurrentRequests
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
	excluded := make(map[int64]struct{}, len(candidates))
	var lastErr error
	for attempts := 0; attempts < maxAttempts; attempts++ {
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

		freshCandidates := s.resolveEmbeddingModelEligibilityFromAccounts(ctx, input.GroupID, publicModel, []Account{*selection.Account})
		if len(freshCandidates) != 1 {
			selection.ReleaseFunc()
			excluded[selection.Account.ID] = struct{}{}
			continue
		}
		candidate := freshCandidates[0]
		result, forwardErr := s.forwardEmbeddingCandidate(ctx, input.Body, candidate, limits)
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

func (s *OpenAIGatewayService) forwardEmbeddingCandidate(
	ctx context.Context,
	body []byte,
	candidate EmbeddingModelEligibility,
	limits embeddingForwardLimits,
) (*EmbeddingForwardResult, error) {
	if strings.TrimSpace(candidate.Account.EffectiveProxyURL()) != "" {
		return nil, &EmbeddingForwardError{Category: "proxy_not_supported", Retryable: true}
	}
	targetURL, destinationIP, err := resolveEmbeddingUpstreamTarget(ctx, candidate.Account.GetEmbeddingBaseURL(), limits)
	if err != nil {
		return nil, &EmbeddingForwardError{Category: "unsafe_upstream", Retryable: true}
	}

	outboundBody, err := sjson.SetBytes(body, "model", candidate.UpstreamModel)
	if err != nil {
		return nil, ErrEmbeddingRequestInvalid
	}
	timeoutCtx, cancel := context.WithTimeout(ctx, limits.timeout)
	defer cancel()
	req, err := http.NewRequestWithContext(timeoutCtx, http.MethodPost, targetURL, bytes.NewReader(outboundBody))
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
	startedAt := time.Now()
	resp, err := doer.DoEmbedding(req, candidate.Account.ID, candidate.Account.Concurrency, EmbeddingUpstreamPolicy{
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
	defer resp.Body.Close()

	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		_, _ = readEmbeddingResponseBody(resp.Body, limits.responseMaxBytes)
		return nil, &EmbeddingForwardError{
			Category:   embeddingStatusCategory(resp.StatusCode),
			StatusCode: resp.StatusCode,
			Retryable:  resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden || resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= http.StatusInternalServerError,
		}
	}

	responseBody, err := readEmbeddingResponseBody(resp.Body, limits.responseMaxBytes)
	if err != nil {
		return nil, &EmbeddingForwardError{Category: "response_read"}
	}
	promptTokens, err := parseEmbeddingPromptTokens(responseBody, limits.maxJSONDepth)
	if err != nil {
		// A malformed 2xx has an uncertain semantic result. Do not fail over or
		// return the vector, because the upstream may already have charged it.
		return nil, &EmbeddingForwardError{Category: "invalid_usage"}
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
			if ch == '\\' {
				escaped = true
			} else if ch == '"' {
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

func readEmbeddingResponseBody(reader io.Reader, maxBytes int64) ([]byte, error) {
	if reader == nil || maxBytes <= 0 {
		return nil, errors.New("embedding response is invalid")
	}
	body, err := io.ReadAll(io.LimitReader(reader, maxBytes+1))
	if err != nil {
		return nil, err
	}
	if int64(len(body)) > maxBytes {
		return nil, errors.New("embedding response exceeds the size limit")
	}
	return body, nil
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
