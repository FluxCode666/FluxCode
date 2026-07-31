package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/apicompat"
	"github.com/google/uuid"
)

var ErrProviderAttemptsExhausted = errors.New("provider route attempts exhausted")

type ProviderRouteAttemptOutcome string

const (
	ProviderRouteAttemptSucceeded ProviderRouteAttemptOutcome = "succeeded"
	ProviderRouteAttemptFailed    ProviderRouteAttemptOutcome = "failed"
	ProviderRouteAttemptRejected  ProviderRouteAttemptOutcome = "rejected"
)

type ProviderRouteAttempt struct {
	TraceID           string
	RouteIdentity     string
	GroupID           int64
	LogicalModel      string
	IngressProtocol   ProtocolFamily
	UpstreamProtocol  ProtocolFamily
	Route             RouteIdentity
	Tier              RouteTier
	Outcome           ProviderRouteAttemptOutcome
	StatusCode        int
	FailureCategory   string
	UpstreamRequestID string
	UpstreamModel     string
	WireProfile       WireProfile
	ConversionUsed    bool
	BytesCommitted    int64
	FinalReason       string
	StartedAt         time.Time
	Duration          time.Duration
}

type ProviderRouteAttemptRecorder interface {
	RecordProviderRouteAttempt(ctx context.Context, attempt ProviderRouteAttempt) error
}

type ProviderRouteAttemptFilter struct {
	GroupID          int64
	ProviderID       int64
	LogicalModel     string
	IngressProtocol  ProtocolFamily
	UpstreamProtocol ProtocolFamily
	Outcome          ProviderRouteAttemptOutcome
	Tier             RouteTier
	Limit            int
}

type ProviderRouteAttemptReader interface {
	ListProviderRouteAttempts(ctx context.Context, filter ProviderRouteAttemptFilter) ([]ProviderRouteAttempt, error)
}

type ProviderRouteStickyStore interface {
	GetProviderStickyRoute(
		ctx context.Context,
		groupID int64,
		logicalModel string,
		protocol ProtocolFamily,
		tier RouteTier,
		sessionHash string,
	) (*RouteIdentity, error)
	SetProviderStickyRoute(
		ctx context.Context,
		groupID int64,
		logicalModel string,
		protocol ProtocolFamily,
		tier RouteTier,
		sessionHash string,
		route RouteIdentity,
		ttl time.Duration,
	) error
}

type ProviderRouteStateStore interface {
	ProviderRouteStickyStore
	GetProviderRouteBinding(ctx context.Context, responseID string) (*ProviderRouteBinding, error)
	SetProviderRouteBinding(ctx context.Context, responseID string, binding ProviderRouteBinding, ttl time.Duration) error
}

type ProviderRouteBinding struct {
	Route        RouteIdentity `json:"route"`
	UserID       int64         `json:"user_id"`
	APIKeyID     int64         `json:"api_key_id"`
	GroupID      int64         `json:"group_id"`
	LogicalModel string        `json:"logical_model"`
}

type ProviderGatewayService struct {
	resolver      *ProviderRouteResolver
	scheduler     *ProviderScheduler
	forwarder     *ProviderForwarder
	state         ProviderRouteStateStore
	attempts      ProviderRouteAttemptRecorder
	attemptReader ProviderRouteAttemptReader
}

func (s *ProviderGatewayService) SetRouteAttemptReader(reader ProviderRouteAttemptReader) {
	if s != nil {
		s.attemptReader = reader
	}
}

func (s *ProviderGatewayService) ListRouteAttempts(ctx context.Context, filter ProviderRouteAttemptFilter) ([]ProviderRouteAttempt, error) {
	if s == nil || s.attemptReader == nil {
		return nil, errors.New("provider route diagnostics are not configured")
	}
	return s.attemptReader.ListProviderRouteAttempts(ctx, filter)
}

func NewProviderGatewayService(
	resolver *ProviderRouteResolver,
	scheduler *ProviderScheduler,
	forwarder *ProviderForwarder,
	state ProviderRouteStateStore,
	attempts ProviderRouteAttemptRecorder,
) *ProviderGatewayService {
	return &ProviderGatewayService{
		resolver: resolver, scheduler: scheduler, forwarder: forwarder,
		state: state, attempts: attempts,
	}
}

type ProviderGatewayRequest struct {
	TraceID         string
	UserID          int64
	APIKeyID        int64
	GroupID         int64
	SnapshotVersion int64
	LogicalModel    string
	Protocol        ProtocolFamily
	Body            []byte
	Headers         http.Header
	SessionHash     string
	MaxSwitches     int
}

type ProviderGatewayResult struct {
	Candidate         RouteCandidate
	Body              []byte
	Headers           http.Header
	Usage             ProviderUsage
	StatusCode        int
	UpstreamRequestID string
	Converted         bool
	Duration          time.Duration
	Stream            bool
}

type ProviderGatewayStreamResult struct {
	Candidate         RouteCandidate
	Body              io.ReadCloser
	Headers           http.Header
	StatusCode        int
	UpstreamRequestID string
	Converted         bool
	Duration          time.Duration
	forwarded         *ProviderForwardStreamResult
}

func (r *ProviderGatewayStreamResult) Usage() ProviderUsage {
	if r == nil || r.forwarded == nil {
		return ProviderUsage{}
	}
	return r.forwarded.Usage()
}

func (r *ProviderGatewayStreamResult) BillingResult() *ProviderGatewayResult {
	if r == nil {
		return nil
	}
	return &ProviderGatewayResult{
		Candidate: r.Candidate, Headers: r.Headers, StatusCode: r.StatusCode,
		Usage: r.Usage(), UpstreamRequestID: r.UpstreamRequestID,
		Converted: r.Converted, Duration: r.Duration, Stream: true,
	}
}

func (s *ProviderGatewayService) Execute(ctx context.Context, request ProviderGatewayRequest) (*ProviderGatewayResult, error) {
	if s == nil || s.resolver == nil || s.scheduler == nil || s.forwarder == nil {
		return nil, errors.New("provider gateway is not initialized")
	}
	traceID := strings.TrimSpace(request.TraceID)
	if traceID == "" {
		traceID = uuid.NewString()
	}
	excludedRoutes := make(map[RouteIdentity]struct{})
	recordedRejections := make(map[string]struct{})
	budget := NewRouteSwitchBudget(request.MaxSwitches)
	var lastErr error
	var requiredRoute *RouteIdentity
	if request.Protocol == ProtocolResponses {
		if previousResponseID := providerPreviousResponseID(request.Body); previousResponseID != "" {
			if s.state == nil {
				return nil, ErrProviderContinuationUnavailable
			}
			bound, err := s.state.GetProviderRouteBinding(ctx, previousResponseID)
			if err != nil || bound == nil || !providerRouteBindingMatchesRequest(*bound, request) {
				return nil, ErrProviderContinuationUnavailable
			}
			requiredRoute = &bound.Route
		}
	}

	for {
		resolution, err := s.resolver.Resolve(ctx, ProviderRouteRequest{
			GroupID: request.GroupID, LogicalModel: request.LogicalModel,
			SnapshotVersion: request.SnapshotVersion,
			IngressProtocol: request.Protocol, RawRequest: request.Body,
			ExcludedRoutes: excludedRoutes, RequiredRoute: requiredRoute,
		})
		s.recordRejections(ctx, traceID, request.GroupID, resolution.Rejections, recordedRejections)
		if err != nil {
			if lastErr != nil {
				return nil, fmt.Errorf("%w: %v", ErrProviderAttemptsExhausted, lastErr)
			}
			return nil, err
		}

		var stickyRoute *RouteIdentity
		if s.state != nil && strings.TrimSpace(request.SessionHash) != "" {
			stickyRoute, _ = s.state.GetProviderStickyRoute(
				ctx, request.GroupID, request.LogicalModel, request.Protocol,
				resolution.Tier, request.SessionHash,
			)
		}
		selection, err := s.scheduler.Select(ctx, ProviderScheduleRequest{
			Tier: resolution.Tier, Candidates: resolution.Candidates,
			StickyRoute: stickyRoute, ExcludedRoutes: excludedRoutes,
		})
		if err != nil || selection == nil {
			if lastErr != nil {
				return nil, fmt.Errorf("%w: %v", ErrProviderAttemptsExhausted, lastErr)
			}
			return nil, err
		}

		candidate := selection.Candidate
		startedAt := time.Now()
		result, forwardErr := s.forward(ctx, request, candidate)
		if selection.ReleaseFunc != nil {
			selection.ReleaseFunc()
		}
		if forwardErr == nil {
			s.recordAttempt(ctx, ProviderRouteAttempt{
				TraceID: traceID, Route: candidate.Identity, Tier: candidate.Tier,
				GroupID: request.GroupID, LogicalModel: request.LogicalModel,
				IngressProtocol: request.Protocol, UpstreamProtocol: candidate.Capability.Protocol,
				Outcome: ProviderRouteAttemptSucceeded, StatusCode: result.StatusCode,
				UpstreamRequestID: result.UpstreamRequestID,
				UpstreamModel:     candidate.Capability.UpstreamModel,
				WireProfile:       candidate.Capability.WireProfile,
				ConversionUsed:    result.Converted,
				BytesCommitted:    int64(len(result.Body)), FinalReason: "completed",
				StartedAt: startedAt, Duration: time.Since(startedAt),
			})
			if s.state != nil && strings.TrimSpace(request.SessionHash) != "" {
				_ = s.state.SetProviderStickyRoute(
					ctx, request.GroupID, request.LogicalModel, request.Protocol,
					candidate.Tier, request.SessionHash, candidate.Identity, time.Hour,
				)
			}
			if s.state != nil && request.Protocol == ProtocolResponses {
				if responseID := providerResponseID(result.Body); responseID != "" {
					_ = s.state.SetProviderRouteBinding(ctx, responseID, providerRouteBinding(request, candidate.Identity), 24*time.Hour)
				}
			}
			return result, nil
		}

		status, category, retryable := providerGatewayFailure(forwardErr)
		s.recordAttempt(ctx, ProviderRouteAttempt{
			TraceID: traceID, Route: candidate.Identity, Tier: candidate.Tier,
			GroupID: request.GroupID, LogicalModel: request.LogicalModel,
			IngressProtocol: request.Protocol, UpstreamProtocol: candidate.Capability.Protocol,
			Outcome: ProviderRouteAttemptFailed, StatusCode: status,
			FailureCategory: category, UpstreamModel: candidate.Capability.UpstreamModel,
			WireProfile:    candidate.Capability.WireProfile,
			ConversionUsed: candidate.Tier == RouteTierConversion,
			FinalReason:    category, StartedAt: startedAt, Duration: time.Since(startedAt),
		})
		lastErr = forwardErr
		if !retryable || !budget.TrySwitch(candidate.Tier) {
			if retryable {
				return nil, fmt.Errorf("%w: %v", ErrProviderAttemptsExhausted, forwardErr)
			}
			return nil, forwardErr
		}
		excludedRoutes[candidate.Identity] = struct{}{}
	}
}

// ExecuteStream performs route selection and failover only until an upstream
// stream has been accepted. Once bytes are exposed, the observed body records
// the terminal outcome but never switches routes.
func (s *ProviderGatewayService) ExecuteStream(
	ctx context.Context,
	request ProviderGatewayRequest,
) (*ProviderGatewayStreamResult, error) {
	if s == nil || s.resolver == nil || s.scheduler == nil || s.forwarder == nil {
		return nil, errors.New("provider gateway is not initialized")
	}
	if request.Protocol == ProtocolEmbeddings {
		return nil, ErrProviderProtocolMismatch
	}
	traceID := strings.TrimSpace(request.TraceID)
	if traceID == "" {
		traceID = uuid.NewString()
	}
	excludedRoutes := make(map[RouteIdentity]struct{})
	recordedRejections := make(map[string]struct{})
	budget := NewRouteSwitchBudget(request.MaxSwitches)
	var lastErr error
	var requiredRoute *RouteIdentity
	if request.Protocol == ProtocolResponses {
		if previousResponseID := providerPreviousResponseID(request.Body); previousResponseID != "" {
			if s.state == nil {
				return nil, ErrProviderContinuationUnavailable
			}
			bound, err := s.state.GetProviderRouteBinding(ctx, previousResponseID)
			if err != nil || bound == nil || !providerRouteBindingMatchesRequest(*bound, request) {
				return nil, ErrProviderContinuationUnavailable
			}
			requiredRoute = &bound.Route
		}
	}

	for {
		resolution, err := s.resolver.Resolve(ctx, ProviderRouteRequest{
			GroupID: request.GroupID, LogicalModel: request.LogicalModel,
			SnapshotVersion: request.SnapshotVersion,
			IngressProtocol: request.Protocol, RawRequest: request.Body,
			ExcludedRoutes: excludedRoutes, RequiredRoute: requiredRoute,
		})
		s.recordRejections(ctx, traceID, request.GroupID, resolution.Rejections, recordedRejections)
		if err != nil {
			if lastErr != nil {
				return nil, fmt.Errorf("%w: %v", ErrProviderAttemptsExhausted, lastErr)
			}
			return nil, err
		}
		var stickyRoute *RouteIdentity
		if s.state != nil && strings.TrimSpace(request.SessionHash) != "" {
			stickyRoute, _ = s.state.GetProviderStickyRoute(
				ctx, request.GroupID, request.LogicalModel, request.Protocol,
				resolution.Tier, request.SessionHash,
			)
		}
		selection, err := s.scheduler.Select(ctx, ProviderScheduleRequest{
			Tier: resolution.Tier, Candidates: resolution.Candidates,
			StickyRoute: stickyRoute, ExcludedRoutes: excludedRoutes,
		})
		if err != nil || selection == nil {
			if lastErr != nil {
				return nil, fmt.Errorf("%w: %v", ErrProviderAttemptsExhausted, lastErr)
			}
			return nil, err
		}
		candidate := selection.Candidate
		startedAt := time.Now()
		forwarded, forwardErr := s.forwardStream(ctx, request, candidate)
		if forwardErr != nil && selection.ReleaseFunc != nil {
			selection.ReleaseFunc()
		}
		if forwardErr != nil {
			status, category, retryable := providerGatewayFailure(forwardErr)
			s.recordAttempt(ctx, ProviderRouteAttempt{
				TraceID: traceID, GroupID: request.GroupID, LogicalModel: request.LogicalModel,
				IngressProtocol: request.Protocol, UpstreamProtocol: candidate.Capability.Protocol,
				Route: candidate.Identity, Tier: candidate.Tier,
				Outcome: ProviderRouteAttemptFailed, StatusCode: status,
				FailureCategory: category, UpstreamModel: candidate.Capability.UpstreamModel,
				WireProfile:    candidate.Capability.WireProfile,
				ConversionUsed: candidate.Tier == RouteTierConversion,
				FinalReason:    category, StartedAt: startedAt, Duration: time.Since(startedAt),
			})
			lastErr = forwardErr
			if !retryable || !budget.TrySwitch(candidate.Tier) {
				if retryable {
					return nil, fmt.Errorf("%w: %v", ErrProviderAttemptsExhausted, forwardErr)
				}
				return nil, forwardErr
			}
			excludedRoutes[candidate.Identity] = struct{}{}
			continue
		}

		result := &ProviderGatewayStreamResult{
			Candidate: candidate, Headers: forwarded.Headers, StatusCode: forwarded.StatusCode,
			UpstreamRequestID: forwarded.UpstreamRequestID, Duration: forwarded.Duration,
			forwarded: forwarded,
		}
		result.Body = &providerObservedStreamBody{
			ReadCloser: forwarded.Body,
			done: func(bytesCommitted int64, streamErr error) {
				if selection.ReleaseFunc != nil {
					selection.ReleaseFunc()
				}
				result.Duration = time.Since(startedAt)
				attemptCtx := context.WithoutCancel(ctx)
				attempt := ProviderRouteAttempt{
					TraceID: traceID, GroupID: request.GroupID, LogicalModel: request.LogicalModel,
					IngressProtocol: request.Protocol, UpstreamProtocol: candidate.Capability.Protocol,
					Route: candidate.Identity, Tier: candidate.Tier, StatusCode: result.StatusCode,
					UpstreamRequestID: result.UpstreamRequestID,
					UpstreamModel:     candidate.Capability.UpstreamModel,
					WireProfile:       candidate.Capability.WireProfile,
					BytesCommitted:    bytesCommitted, StartedAt: startedAt, Duration: result.Duration,
				}
				if streamErr == nil {
					attempt.Outcome, attempt.FinalReason = ProviderRouteAttemptSucceeded, "completed"
					if s.state != nil && strings.TrimSpace(request.SessionHash) != "" {
						_ = s.state.SetProviderStickyRoute(
							attemptCtx, request.GroupID, request.LogicalModel, request.Protocol,
							candidate.Tier, request.SessionHash, candidate.Identity, time.Hour,
						)
					}
					if s.state != nil && request.Protocol == ProtocolResponses {
						if responseID := forwarded.ResponseID(); responseID != "" {
							_ = s.state.SetProviderRouteBinding(attemptCtx, responseID, providerRouteBinding(request, candidate.Identity), 24*time.Hour)
						}
					}
				} else {
					attempt.Outcome = ProviderRouteAttemptFailed
					attempt.FailureCategory, attempt.FinalReason = "stream_interrupted", "stream_interrupted"
				}
				s.recordAttempt(attemptCtx, attempt)
			},
		}
		return result, nil
	}
}

func providerRouteBinding(request ProviderGatewayRequest, route RouteIdentity) ProviderRouteBinding {
	return ProviderRouteBinding{
		Route: route, UserID: request.UserID, APIKeyID: request.APIKeyID,
		GroupID: request.GroupID, LogicalModel: strings.TrimSpace(request.LogicalModel),
	}
}

func providerRouteBindingMatchesRequest(binding ProviderRouteBinding, request ProviderGatewayRequest) bool {
	return binding.UserID == request.UserID && binding.APIKeyID == request.APIKeyID &&
		binding.GroupID == request.GroupID &&
		strings.EqualFold(strings.TrimSpace(binding.LogicalModel), strings.TrimSpace(request.LogicalModel))
}

func (s *ProviderGatewayService) forwardStream(
	ctx context.Context,
	request ProviderGatewayRequest,
	candidate RouteCandidate,
) (*ProviderForwardStreamResult, error) {
	if candidate.Tier != RouteTierNative {
		return nil, ErrProviderProtocolMismatch
	}
	input := ProviderForwardInput{Candidate: candidate, Body: request.Body, Headers: request.Headers}
	switch request.Protocol {
	case ProtocolChatCompletions:
		return s.forwarder.ForwardChatStream(ctx, input)
	case ProtocolResponses:
		return s.forwarder.ForwardResponsesStream(ctx, input)
	case ProtocolAnthropicMessages:
		return s.forwarder.ForwardMessagesStream(ctx, input)
	default:
		return nil, ErrProviderProtocolMismatch
	}
}

type providerObservedStreamBody struct {
	io.ReadCloser
	done  func(int64, error)
	once  sync.Once
	bytes int64
}

func (b *providerObservedStreamBody) Read(target []byte) (int, error) {
	n, err := b.ReadCloser.Read(target)
	b.bytes += int64(n)
	if err != nil {
		terminalErr := err
		if errors.Is(err, io.EOF) {
			terminalErr = nil
		}
		b.finish(terminalErr)
	}
	return n, err
}

func (b *providerObservedStreamBody) Close() error {
	err := b.ReadCloser.Close()
	b.finish(context.Canceled)
	return err
}

func (b *providerObservedStreamBody) finish(err error) {
	b.once.Do(func() {
		if b.done != nil {
			b.done(b.bytes, err)
		}
	})
}

func (s *ProviderGatewayService) recordRejections(
	ctx context.Context,
	traceID string,
	groupID int64,
	rejections []ProviderRouteRejection,
	recorded map[string]struct{},
) {
	for _, rejection := range rejections {
		key := rejection.Route.String() + "\x00" + rejection.Reason
		if _, exists := recorded[key]; exists {
			continue
		}
		recorded[key] = struct{}{}
		finalReason := rejection.Reason
		if rejection.Detail != "" {
			finalReason += ": " + rejection.Detail
		}
		s.recordAttempt(ctx, ProviderRouteAttempt{
			TraceID: traceID, GroupID: groupID, LogicalModel: rejection.LogicalModel,
			IngressProtocol: rejection.IngressProtocol, UpstreamProtocol: rejection.UpstreamProtocol,
			Route: rejection.Route, Tier: rejection.Tier, Outcome: ProviderRouteAttemptRejected,
			FailureCategory: rejection.Reason, UpstreamModel: rejection.UpstreamModel,
			WireProfile: rejection.WireProfile, ConversionUsed: rejection.ConversionUsed,
			FinalReason: finalReason, StartedAt: time.Now(),
		})
	}
}

func providerPreviousResponseID(body []byte) string {
	var envelope struct {
		PreviousResponseID string `json:"previous_response_id"`
	}
	if json.Unmarshal(body, &envelope) != nil {
		return ""
	}
	return strings.TrimSpace(envelope.PreviousResponseID)
}

func providerResponseID(body []byte) string {
	var envelope struct {
		ID string `json:"id"`
	}
	if json.Unmarshal(body, &envelope) != nil {
		return ""
	}
	return strings.TrimSpace(envelope.ID)
}

func (s *ProviderGatewayService) forward(
	ctx context.Context,
	request ProviderGatewayRequest,
	candidate RouteCandidate,
) (*ProviderGatewayResult, error) {
	if candidate.Tier == RouteTierConversion {
		return s.forwardConversion(ctx, request, candidate)
	}
	input := ProviderForwardInput{Candidate: candidate, Body: request.Body, Headers: request.Headers}
	var (
		forwarded *ProviderForwardResult
		err       error
	)
	switch request.Protocol {
	case ProtocolChatCompletions:
		forwarded, err = s.forwarder.ForwardChat(ctx, input)
	case ProtocolResponses:
		forwarded, err = s.forwarder.ForwardResponses(ctx, input)
	case ProtocolAnthropicMessages:
		forwarded, err = s.forwarder.ForwardMessages(ctx, input)
	case ProtocolEmbeddings:
		forwarded, err = s.forwarder.ForwardEmbeddings(ctx, input)
	default:
		err = ErrProviderProtocolMismatch
	}
	if err != nil {
		return nil, err
	}
	return providerGatewayResult(candidate, forwarded, false), nil
}

func (s *ProviderGatewayService) forwardConversion(
	ctx context.Context,
	request ProviderGatewayRequest,
	candidate RouteCandidate,
) (*ProviderGatewayResult, error) {
	if request.Protocol != ProtocolResponses || candidate.Capability.Protocol != ProtocolChatCompletions ||
		candidate.AdapterProfile != apicompat.ProfileNonStreamText {
		return nil, ErrProviderProtocolMismatch
	}
	var responsesRequest apicompat.ResponsesRequest
	if err := json.Unmarshal(request.Body, &responsesRequest); err != nil {
		return nil, err
	}
	chatRequest, err := apicompat.ResponsesToChatCompletionsRequest(&responsesRequest)
	if err != nil {
		return nil, err
	}
	chatBody, err := json.Marshal(chatRequest)
	if err != nil {
		return nil, err
	}
	transportCandidate := candidate
	transportCandidate.Tier = RouteTierNative
	transportCandidate.Identity.IngressProtocol = ProtocolChatCompletions
	transportCandidate.Identity.Adapter = ""
	transportCandidate.Identity.AdapterVersion = ""
	forwarded, err := s.forwarder.ForwardChat(ctx, ProviderForwardInput{
		Candidate: transportCandidate, Body: chatBody, Headers: request.Headers,
	})
	if err != nil {
		if upstreamErr := new(ProviderUpstreamError); errors.As(err, &upstreamErr) {
			upstreamErr.Route = candidate.Identity
		}
		return nil, err
	}
	var chatResponse apicompat.ChatCompletionsResponse
	if err := json.Unmarshal(forwarded.Body, &chatResponse); err != nil {
		return nil, fmt.Errorf("decode converted chat response: %w", err)
	}
	responsesResponse, err := apicompat.ChatCompletionsToResponsesResponse(&chatResponse, candidate.LogicalModel.Name)
	if err != nil {
		return nil, fmt.Errorf("convert chat response: %w", err)
	}
	responseBody, err := json.Marshal(responsesResponse)
	if err != nil {
		return nil, err
	}
	forwarded.Route = candidate.Identity
	forwarded.Body = responseBody
	forwarded.Usage = parseProviderUsage(ProtocolResponses, responseBody)
	return providerGatewayResult(candidate, forwarded, true), nil
}

func providerGatewayResult(candidate RouteCandidate, forwarded *ProviderForwardResult, converted bool) *ProviderGatewayResult {
	return &ProviderGatewayResult{
		Candidate: candidate, Body: forwarded.Body, Headers: forwarded.Headers,
		Usage: forwarded.Usage, StatusCode: forwarded.StatusCode,
		UpstreamRequestID: forwarded.UpstreamRequestID,
		Converted:         converted, Duration: forwarded.Duration,
	}
}

func providerGatewayFailure(err error) (status int, category string, retryable bool) {
	var upstreamErr *ProviderUpstreamError
	if errors.As(err, &upstreamErr) {
		return upstreamErr.StatusCode, "upstream_status", upstreamErr.Retryable
	}
	var transportErr *ProviderTransportError
	if errors.As(err, &transportErr) {
		return 0, "transport", transportErr.RequestNotWritten
	}
	switch {
	case errors.Is(err, context.DeadlineExceeded):
		return 0, "timeout", true
	case errors.Is(err, ErrProviderResponseTooLarge), errors.Is(err, ErrProviderStreamEventTooLarge):
		return 0, "protocol_violation", false
	default:
		return 0, "internal", false
	}
}

func (s *ProviderGatewayService) recordAttempt(ctx context.Context, attempt ProviderRouteAttempt) {
	if s == nil || s.attempts == nil {
		return
	}
	_ = s.attempts.RecordProviderRouteAttempt(ctx, attempt)
}
