package service

import (
	"bytes"
	"encoding/json"
	"io"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

const historicalMediaCandidateSnapshotV1 = `[{"account_id":7,"platform":"media","resolved_model":{"Provider":"relay","Adapter":"legacy-image","UpstreamModel":"upstream-image","NativeAsyncMode":"optional","RequestMapping":{}},"model_definition":{"ID":9,"ModelID":"grok-2-image","Vendor":"xai","MediaType":"image","Operations":["text_to_image"],"Constraints":{},"BillingUnit":"image","DefaultAdapter":"legacy-image","DefaultAsyncMode":"optional","Enabled":true,"CreatedAt":"2026-07-21T00:00:00Z","UpdatedAt":"2026-07-21T00:00:00Z"}}]`

type legacyStrictResolvedModelV1 struct {
	Provider        string              `json:"Provider"`
	Adapter         string              `json:"Adapter"`
	UpstreamModel   string              `json:"UpstreamModel"`
	NativeAsyncMode NativeAsyncMode     `json:"NativeAsyncMode"`
	RequestMapping  MediaRequestMapping `json:"RequestMapping"`
}

type legacyStrictMediaModelDefinitionV1 struct {
	ID               int64            `json:"ID"`
	ModelID          string           `json:"ModelID"`
	Vendor           string           `json:"Vendor"`
	MediaType        MediaType        `json:"MediaType"`
	Operations       []MediaOperation `json:"Operations"`
	Constraints      json.RawMessage  `json:"Constraints"`
	BillingUnit      string           `json:"BillingUnit"`
	DefaultAdapter   string           `json:"DefaultAdapter"`
	DefaultAsyncMode NativeAsyncMode  `json:"DefaultAsyncMode"`
	Enabled          bool             `json:"Enabled"`
	CreatedAt        time.Time        `json:"CreatedAt"`
	UpdatedAt        time.Time        `json:"UpdatedAt"`
}

type legacyStrictMediaCandidateV1 struct {
	AccountID       int64                               `json:"account_id"`
	Platform        string                              `json:"platform"`
	ResolvedModel   legacyStrictResolvedModelV1         `json:"resolved_model"`
	ResolvedRequest json.RawMessage                     `json:"resolved_request,omitempty"`
	ModelDefinition *legacyStrictMediaModelDefinitionV1 `json:"model_definition,omitempty"`
}

func TestMediaCandidateSnapshotV1StrictlyDecodesHistoricalJSON(t *testing.T) {
	candidates, err := decodeMediaCandidateSnapshotV1(json.RawMessage(historicalMediaCandidateSnapshotV1))
	require.NoError(t, err)
	require.Len(t, candidates, 1)
	require.Equal(t, "legacy-image", candidates[0].ResolvedModel.Adapter)
	require.Equal(t, NativeAsyncOptional, candidates[0].ResolvedModel.NativeAsyncMode)

	raw, err := encodeMediaCandidateSnapshotV1(candidates)
	require.NoError(t, err)
	require.Equal(t, byte('['), bytes.TrimSpace(raw)[0])
	require.NotContains(t, string(raw), "AdapterResolution")
}

func TestMediaCandidateSnapshotV1NewWriterIsReadableByLegacyStrictReader(t *testing.T) {
	definition := validImageModelDefinition()
	definition.ModelID = "grok-2-image"
	definition.Vendor = "xai"
	definition.Operations = []MediaOperation{MediaOperationTextToImage}
	definition.DefaultAdapter = "stale-db-key"
	definition.DefaultAsyncMode = NativeAsyncRequired
	definition.AdapterResolution = readyResolution(true, false)
	definition.AdapterResolution.ResolvedAdapter = "xai-image"
	candidates := []MediaAccountCandidateSnapshot{{
		AccountID: 7,
		Platform:  PlatformMedia,
		ResolvedModel: ResolvedMediaAccountModel{
			Provider: "xai", Adapter: "xai-image", UpstreamModel: "upstream-image",
			NativeAsyncMode: NativeAsyncUnsupported, RequestMapping: MediaRequestMapping{},
		},
		ModelDefinition: &definition,
	}}
	raw, err := encodeMediaCandidateSnapshotV1(candidates)
	require.NoError(t, err)
	require.Equal(t, "stale-db-key", definition.DefaultAdapter)
	require.Equal(t, NativeAsyncRequired, definition.DefaultAsyncMode)

	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	var legacy []legacyStrictMediaCandidateV1
	require.NoError(t, decoder.Decode(&legacy))
	var trailing json.RawMessage
	require.ErrorIs(t, decoder.Decode(&trailing), io.EOF)
	require.Equal(t, "xai-image", legacy[0].ModelDefinition.DefaultAdapter)
	require.Equal(t, NativeAsyncUnsupported, legacy[0].ModelDefinition.DefaultAsyncMode)
	require.Equal(t, "xai-image", legacy[0].ResolvedModel.Adapter)
}

func TestMediaCandidateSnapshotV1DurableRewritePreservesHistoricalCompatibilityFields(t *testing.T) {
	candidates, err := decodeMediaCandidateSnapshotV1(json.RawMessage(historicalMediaCandidateSnapshotV1))
	require.NoError(t, err)
	require.False(t, candidates[0].ModelDefinition.AdapterResolution.IsReady())

	raw, err := encodeMediaCandidateSnapshotV1(candidates)
	require.NoError(t, err)
	require.Contains(t, string(raw), `"DefaultAdapter":"legacy-image"`)
	require.Contains(t, string(raw), `"DefaultAsyncMode":"optional"`)
}

func TestMediaCandidateSnapshotV1RejectsUnknownFieldsAndTrailingValues(t *testing.T) {
	tests := []struct {
		name string
		raw  string
	}{
		{
			name: "unknown nested field",
			raw:  `[{"account_id":7,"platform":"media","resolved_model":{"Provider":"relay","Adapter":"legacy-image","UpstreamModel":"upstream-image","NativeAsyncMode":"optional","RequestMapping":{},"Credential":"secret"}}]`,
		},
		{name: "extra top level value", raw: historicalMediaCandidateSnapshotV1 + ` {}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := decodeMediaCandidateSnapshotV1(json.RawMessage(tt.raw))
			require.Error(t, err)
		})
	}
}

func TestMediaCandidateSnapshotV1WorkerRouteUsesResolvedAdapterNotDefinitionDefault(t *testing.T) {
	candidates, err := decodeMediaCandidateSnapshotV1(json.RawMessage(historicalMediaCandidateSnapshotV1))
	require.NoError(t, err)
	candidates[0].ResolvedModel.Adapter = "frozen-execution-adapter"
	candidates[0].ModelDefinition.DefaultAdapter = "untrusted-definition-adapter"

	raw, err := encodeMediaCandidateSnapshotV1(candidates)
	require.NoError(t, err)
	decoded, err := decodeWorkerCandidateSnapshot(raw)
	require.NoError(t, err)
	require.Equal(t, "frozen-execution-adapter", decoded[0].ResolvedModel.Adapter)
	require.Equal(t, "untrusted-definition-adapter", decoded[0].ModelDefinition.DefaultAdapter)
}
