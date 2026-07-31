package dto

import (
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
)

type ProviderEndpointRequest struct {
	Protocol    service.ProtocolFamily `json:"protocol" binding:"required"`
	WireProfile service.WireProfile    `json:"wire_profile"`
	BaseURL     string                 `json:"base_url"`
	Path        string                 `json:"path"`
	Headers     map[string]string      `json:"headers"`
	AuthType    string                 `json:"auth_type"`
	Enabled     *bool                  `json:"enabled"`
}

type ProviderCapabilityRequest struct {
	LogicalModel        string                         `json:"logical_model" binding:"required"`
	LogicalModelDisplay string                         `json:"logical_model_display"`
	Protocol            service.ProtocolFamily         `json:"protocol" binding:"required"`
	UpstreamModel       string                         `json:"upstream_model" binding:"required"`
	WireProfile         service.WireProfile            `json:"wire_profile"`
	FeatureProfile      service.ProviderFeatureProfile `json:"feature_profile" binding:"required"`
	Enabled             *bool                          `json:"enabled"`
}

type ProviderWriteRequest struct {
	Name                    string                      `json:"name" binding:"required"`
	BaseURL                 string                      `json:"base_url" binding:"required"`
	Headers                 map[string]string           `json:"headers"`
	AuthType                string                      `json:"auth_type"`
	APIKey                  *string                     `json:"api_key"`
	ClearAPIKey             bool                        `json:"clear_api_key"`
	AllowProtocolConversion bool                        `json:"allow_protocol_conversion"`
	GroupIDs                []int64                     `json:"group_ids"`
	Concurrency             int                         `json:"concurrency"`
	RateMultiplier          *float64                    `json:"rate_multiplier"`
	Endpoints               []ProviderEndpointRequest   `json:"endpoints"`
	Capabilities            []ProviderCapabilityRequest `json:"capabilities"`
	Version                 int64                       `json:"version"`
}

func (r ProviderWriteRequest) ServiceInput() service.ProviderWriteInput {
	endpoints := make([]service.ProviderEndpointInput, 0, len(r.Endpoints))
	for _, item := range r.Endpoints {
		enabled := true
		if item.Enabled != nil {
			enabled = *item.Enabled
		}
		endpoints = append(endpoints, service.ProviderEndpointInput{
			Protocol: item.Protocol, WireProfile: item.WireProfile, BaseURL: item.BaseURL,
			Path: item.Path, Headers: item.Headers, AuthType: item.AuthType, Enabled: enabled,
		})
	}
	capabilities := make([]service.ProviderCapabilityInput, 0, len(r.Capabilities))
	for _, item := range r.Capabilities {
		enabled := true
		if item.Enabled != nil {
			enabled = *item.Enabled
		}
		capabilities = append(capabilities, service.ProviderCapabilityInput{
			LogicalModelName: item.LogicalModel, LogicalModelDisplayName: item.LogicalModelDisplay,
			Protocol: item.Protocol, UpstreamModel: item.UpstreamModel, WireProfile: item.WireProfile,
			FeatureProfile: item.FeatureProfile, Enabled: enabled,
		})
	}
	return service.ProviderWriteInput{
		Name: r.Name, BaseURL: r.BaseURL, Headers: r.Headers, AuthType: r.AuthType,
		APIKey: r.APIKey, ClearAPIKey: r.ClearAPIKey,
		AllowProtocolConversion: r.AllowProtocolConversion,
		GroupIDs:                r.GroupIDs, Concurrency: r.Concurrency, RateMultiplier: r.RateMultiplier,
		Endpoints: endpoints, Capabilities: capabilities, Version: r.Version,
	}
}

type ProviderEndpoint struct {
	ID          int64                  `json:"id"`
	Protocol    service.ProtocolFamily `json:"protocol"`
	WireProfile service.WireProfile    `json:"wire_profile"`
	BaseURL     string                 `json:"base_url"`
	Path        string                 `json:"path"`
	Headers     map[string]string      `json:"headers"`
	AuthType    string                 `json:"auth_type"`
	Enabled     bool                   `json:"enabled"`
	Version     int64                  `json:"version"`
}

type ProviderCapability struct {
	ID                  int64                          `json:"id"`
	LogicalModelID      int64                          `json:"logical_model_id"`
	LogicalModel        string                         `json:"logical_model"`
	LogicalModelDisplay string                         `json:"logical_model_display"`
	Protocol            service.ProtocolFamily         `json:"protocol"`
	UpstreamModel       string                         `json:"upstream_model"`
	WireProfile         service.WireProfile            `json:"wire_profile"`
	FeatureProfile      service.ProviderFeatureProfile `json:"feature_profile"`
	EndpointID          *int64                         `json:"endpoint_id"`
	Enabled             bool                           `json:"enabled"`
	Version             int64                          `json:"version"`
}

type ProviderCapabilityTestResponse struct {
	ProviderID        int64                  `json:"provider_id"`
	CapabilityID      int64                  `json:"capability_id"`
	Protocol          service.ProtocolFamily `json:"protocol"`
	LogicalModel      string                 `json:"logical_model"`
	UpstreamModel     string                 `json:"upstream_model"`
	StatusCode        int                    `json:"status_code"`
	DurationMs        int64                  `json:"duration_ms"`
	UpstreamRequestID string                 `json:"upstream_request_id"`
}

func ProviderCapabilityTestFromService(result *service.ProviderCapabilityTestResult) *ProviderCapabilityTestResponse {
	if result == nil {
		return nil
	}
	return &ProviderCapabilityTestResponse{
		ProviderID: result.ProviderID, CapabilityID: result.CapabilityID,
		Protocol: result.Protocol, LogicalModel: result.LogicalModel, UpstreamModel: result.UpstreamModel,
		StatusCode: result.StatusCode, DurationMs: result.Duration.Milliseconds(),
		UpstreamRequestID: result.UpstreamRequestID,
	}
}

type Provider struct {
	ID                      int64                  `json:"id"`
	Name                    string                 `json:"name"`
	Status                  service.ProviderStatus `json:"status"`
	AllowProtocolConversion bool                   `json:"allow_protocol_conversion"`
	BaseURL                 string                 `json:"base_url"`
	Headers                 map[string]string      `json:"headers"`
	AuthType                string                 `json:"auth_type"`
	CredentialConfigured    bool                   `json:"credential_configured"`
	GroupIDs                []int64                `json:"group_ids"`
	Concurrency             int                    `json:"concurrency"`
	RateMultiplier          float64                `json:"rate_multiplier"`
	Version                 int64                  `json:"version"`
	Endpoints               []ProviderEndpoint     `json:"endpoints"`
	Capabilities            []ProviderCapability   `json:"capabilities"`
	CreatedAt               time.Time              `json:"created_at"`
	UpdatedAt               time.Time              `json:"updated_at"`
}

func ProviderFromService(aggregate *service.ProviderAggregate) *Provider {
	if aggregate == nil || aggregate.Profile == nil || aggregate.Account == nil {
		return nil
	}
	profile, account := aggregate.Profile, aggregate.Account
	out := &Provider{
		ID: profile.ID, Name: profile.Name, Status: profile.Status,
		AllowProtocolConversion: profile.AllowProtocolConversion,
		BaseURL:                 profile.Connection.BaseURL, Headers: profile.Connection.Headers,
		AuthType:             profile.Connection.AuthType,
		CredentialConfigured: strings.TrimSpace(account.GetCredential("api_key")) != "" || strings.TrimSpace(account.GetCredential("access_token")) != "",
		GroupIDs:             append([]int64(nil), account.GroupIDs...), Concurrency: account.Concurrency,
		RateMultiplier: account.BillingRateMultiplier(), Version: profile.Version,
		Endpoints:    make([]ProviderEndpoint, 0, len(profile.Endpoints)),
		Capabilities: make([]ProviderCapability, 0, len(profile.Capabilities)),
		CreatedAt:    profile.CreatedAt, UpdatedAt: profile.UpdatedAt,
	}
	for _, endpoint := range profile.Endpoints {
		out.Endpoints = append(out.Endpoints, ProviderEndpoint{
			ID: endpoint.ID, Protocol: endpoint.Protocol, WireProfile: endpoint.WireProfile,
			BaseURL: endpoint.BaseURL, Path: endpoint.Path, Headers: endpoint.Headers,
			AuthType: endpoint.AuthType, Enabled: endpoint.Enabled, Version: endpoint.Version,
		})
	}
	for _, capability := range profile.Capabilities {
		model := aggregate.LogicalModels[capability.LogicalModelID]
		out.Capabilities = append(out.Capabilities, ProviderCapability{
			ID: capability.ID, LogicalModelID: model.ID, LogicalModel: model.Name,
			LogicalModelDisplay: model.DisplayName, Protocol: capability.Protocol,
			UpstreamModel: capability.UpstreamModel, WireProfile: capability.WireProfile,
			FeatureProfile: capability.FeatureProfile, EndpointID: capability.EndpointID,
			Enabled: capability.Enabled, Version: capability.Version,
		})
	}
	return out
}

func ProvidersFromService(aggregates []*service.ProviderAggregate) []*Provider {
	result := make([]*Provider, 0, len(aggregates))
	for _, aggregate := range aggregates {
		if item := ProviderFromService(aggregate); item != nil {
			result = append(result, item)
		}
	}
	return result
}
