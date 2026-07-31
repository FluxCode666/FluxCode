package repository

import (
	"context"
	"errors"
	"strings"
	"time"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/ent/predicate"
	"github.com/Wei-Shaw/sub2api/ent/providerrouteattempt"
	"github.com/Wei-Shaw/sub2api/internal/service"
)

type providerRouteAttemptRepository struct {
	client *dbent.Client
}

func NewProviderRouteAttemptRepository(client *dbent.Client) *providerRouteAttemptRepository {
	return &providerRouteAttemptRepository{client: client}
}

func (r *providerRouteAttemptRepository) ListProviderRouteAttempts(ctx context.Context, filter service.ProviderRouteAttemptFilter) ([]service.ProviderRouteAttempt, error) {
	if r == nil || r.client == nil {
		return nil, errors.New("provider route attempt repository is not initialized")
	}
	if filter.Limit <= 0 || filter.Limit > 500 {
		filter.Limit = 100
	}
	predicates := make([]predicate.ProviderRouteAttempt, 0, 5)
	if filter.GroupID > 0 {
		predicates = append(predicates, providerrouteattempt.GroupIDEQ(filter.GroupID))
	}
	if filter.ProviderID > 0 {
		predicates = append(predicates, providerrouteattempt.ProviderIDEQ(filter.ProviderID))
	}
	if strings.TrimSpace(filter.LogicalModel) != "" {
		predicates = append(predicates, providerrouteattempt.LogicalModelEqualFold(strings.TrimSpace(filter.LogicalModel)))
	}
	if filter.IngressProtocol != "" {
		predicates = append(predicates, providerrouteattempt.IngressProtocolEQ(string(filter.IngressProtocol)))
	}
	if filter.UpstreamProtocol != "" {
		predicates = append(predicates, providerrouteattempt.UpstreamProtocolEQ(string(filter.UpstreamProtocol)))
	}
	if filter.Outcome != "" {
		predicates = append(predicates, providerrouteattempt.OutcomeEQ(string(filter.Outcome)))
	}
	if filter.Tier != "" {
		predicates = append(predicates, providerrouteattempt.RouteTierEQ(string(filter.Tier)))
	}
	items, err := r.client.ProviderRouteAttempt.Query().Where(predicates...).Order(dbent.Desc(providerrouteattempt.FieldCreatedAt)).Limit(filter.Limit).All(ctx)
	if err != nil {
		return nil, err
	}
	result := make([]service.ProviderRouteAttempt, 0, len(items))
	for _, item := range items {
		result = append(result, service.ProviderRouteAttempt{
			TraceID: item.TraceID, RouteIdentity: item.RouteIdentity, GroupID: item.GroupID, LogicalModel: item.LogicalModel,
			IngressProtocol: service.ProtocolFamily(item.IngressProtocol), UpstreamProtocol: service.ProtocolFamily(item.UpstreamProtocol),
			Route: service.RouteIdentity{ProviderID: item.ProviderID, CapabilityID: item.CapabilityID, EndpointID: item.EndpointID, IngressProtocol: service.ProtocolFamily(item.IngressProtocol), UpstreamProtocol: service.ProtocolFamily(item.UpstreamProtocol)},
			Tier:  service.RouteTier(item.RouteTier), Outcome: service.ProviderRouteAttemptOutcome(item.Outcome), StatusCode: item.StatusCode,
			FailureCategory: item.FailureCategory, UpstreamRequestID: item.UpstreamRequestID, UpstreamModel: item.UpstreamModel,
			WireProfile: service.WireProfile(item.WireProfile), ConversionUsed: item.ConversionUsed, BytesCommitted: item.BytesCommitted,
			FinalReason: stringValue(item.FinalReason), StartedAt: item.CreatedAt, Duration: time.Duration(item.DurationMs) * time.Millisecond,
		})
	}
	return result, nil
}

func stringValue(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

func (r *providerRouteAttemptRepository) RecordProviderRouteAttempt(
	ctx context.Context,
	attempt service.ProviderRouteAttempt,
) error {
	if r == nil || r.client == nil {
		return errors.New("provider route attempt repository is not initialized")
	}
	if strings.TrimSpace(attempt.TraceID) == "" || attempt.GroupID <= 0 ||
		attempt.Route.ProviderID <= 0 || attempt.Route.CapabilityID <= 0 ||
		!attempt.IngressProtocol.IsValid() || !attempt.UpstreamProtocol.IsValid() ||
		!attempt.Tier.IsValid() {
		return errors.New("invalid provider route attempt")
	}
	switch attempt.Outcome {
	case service.ProviderRouteAttemptSucceeded,
		service.ProviderRouteAttemptFailed,
		service.ProviderRouteAttemptRejected:
	default:
		return errors.New("invalid provider route attempt outcome")
	}

	durationMillis := attempt.Duration.Milliseconds()
	if durationMillis < 0 {
		durationMillis = 0
	}
	bytesCommitted := attempt.BytesCommitted
	if bytesCommitted < 0 {
		bytesCommitted = 0
	}
	create := r.client.ProviderRouteAttempt.Create().
		SetTraceID(strings.TrimSpace(attempt.TraceID)).
		SetGroupID(attempt.GroupID).
		SetProviderID(attempt.Route.ProviderID).
		SetCapabilityID(attempt.Route.CapabilityID).
		SetEndpointID(attempt.Route.EndpointID).
		SetRouteIdentity(attempt.Route.String()).
		SetLogicalModel(strings.TrimSpace(attempt.LogicalModel)).
		SetUpstreamModel(strings.TrimSpace(attempt.UpstreamModel)).
		SetIngressProtocol(string(attempt.IngressProtocol)).
		SetUpstreamProtocol(string(attempt.UpstreamProtocol)).
		SetWireProfile(string(attempt.WireProfile)).
		SetRouteTier(string(attempt.Tier)).
		SetConversionUsed(attempt.ConversionUsed).
		SetOutcome(string(attempt.Outcome)).
		SetStatusCode(attempt.StatusCode).
		SetFailureCategory(strings.TrimSpace(attempt.FailureCategory)).
		SetUpstreamRequestID(strings.TrimSpace(attempt.UpstreamRequestID)).
		SetDurationMs(durationMillis).
		SetBytesCommitted(bytesCommitted)
	if finalReason := strings.TrimSpace(attempt.FinalReason); finalReason != "" {
		create.SetFinalReason(finalReason)
	}
	if !attempt.StartedAt.IsZero() {
		create.SetCreatedAt(attempt.StartedAt)
	}
	return create.Exec(ctx)
}

var _ service.ProviderRouteAttemptRecorder = (*providerRouteAttemptRepository)(nil)
var _ service.ProviderRouteAttemptReader = (*providerRouteAttemptRepository)(nil)
