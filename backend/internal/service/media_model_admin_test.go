package service

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/stretchr/testify/require"
)

type mediaModelAdminServiceStore struct {
	record   *MediaModelAdminRecord
	onCreate func()
	listErr  error
}

func (s *mediaModelAdminServiceStore) ListEnabled(ctx context.Context) ([]MediaModelDefinition, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if s.listErr != nil {
		return nil, s.listErr
	}
	if s.record == nil || !s.record.Definition.Enabled {
		return []MediaModelDefinition{}, nil
	}
	return []MediaModelDefinition{s.record.Definition}, nil
}

func (s *mediaModelAdminServiceStore) ListAll(ctx context.Context) ([]MediaModelAlias, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if s.record == nil || !s.record.Definition.Enabled {
		return []MediaModelAlias{}, nil
	}
	aliases := make([]MediaModelAlias, 0, len(s.record.Aliases))
	for _, alias := range s.record.Aliases {
		aliases = append(aliases, MediaModelAlias{RequestedModelID: alias, ModelDefinitionID: s.record.Definition.ID})
	}
	return aliases, nil
}

func (s *mediaModelAdminServiceStore) ListAdmin(context.Context) ([]MediaModelAdminRecord, error) {
	if s.record == nil {
		return []MediaModelAdminRecord{}, nil
	}
	return []MediaModelAdminRecord{*s.record}, nil
}

func (s *mediaModelAdminServiceStore) GetAdminByID(context.Context, int64) (*MediaModelAdminRecord, error) {
	if s.record == nil {
		return nil, ErrMediaModelDefinitionNotFound
	}
	copy := *s.record
	return &copy, nil
}

func (s *mediaModelAdminServiceStore) CreateAdmin(_ context.Context, record MediaModelAdminRecord) (*MediaModelAdminRecord, error) {
	record.Definition.ID = 1
	s.record = &record
	if s.onCreate != nil {
		s.onCreate()
	}
	copy := record
	return &copy, nil
}

func (s *mediaModelAdminServiceStore) UpdateAdmin(_ context.Context, _ int64, record MediaModelAdminRecord) (*MediaModelAdminRecord, error) {
	record.Definition.ID = 1
	s.record = &record
	copy := record
	return &copy, nil
}

func (s *mediaModelAdminServiceStore) DeleteAdmin(context.Context, int64) error {
	s.record = nil
	return nil
}

func validMediaModelAdminServiceRecord() MediaModelAdminRecord {
	return MediaModelAdminRecord{
		Definition: MediaModelDefinition{
			ModelID:          "image-model",
			Vendor:           "openai",
			MediaType:        MediaTypeImage,
			Operations:       []MediaOperation{MediaOperationTextToImage},
			Constraints:      json.RawMessage(`{"image_sizes":["1024x1024"]}`),
			BillingUnit:      "image",
			DefaultAdapter:   "openai-images",
			DefaultAsyncMode: NativeAsyncOptional,
			Enabled:          true,
		},
		Aliases: []string{"image-alias"},
	}
}

func TestMediaModelAdminServiceRefreshesAfterCommitEvenWhenRequestIsCanceled(t *testing.T) {
	store := &mediaModelAdminServiceStore{}
	registry := NewMediaModelRegistry(store)
	require.NoError(t, registry.Refresh(context.Background()))
	ctx, cancel := context.WithCancel(context.Background())
	store.onCreate = cancel
	svc := NewMediaModelAdminService(store, nil, nil, registry)

	_, err := svc.Create(ctx, validMediaModelAdminServiceRecord())
	require.NoError(t, err)
	resolved, err := registry.Resolve("image-alias", MediaOperationTextToImage)
	require.NoError(t, err)
	require.Equal(t, "image-model", resolved.ModelID)
}

func TestMediaModelAdminServiceDoesNotReportCommittedCreateAsFailedWhenRefreshFails(t *testing.T) {
	store := &mediaModelAdminServiceStore{}
	registry := NewMediaModelRegistry(store)
	require.NoError(t, registry.Refresh(context.Background()))
	store.onCreate = func() { store.listErr = errors.New("registry read unavailable") }
	svc := NewMediaModelAdminService(store, nil, nil, registry)

	created, err := svc.Create(context.Background(), validMediaModelAdminServiceRecord())
	require.NoError(t, err)
	require.NotNil(t, created)
	require.Equal(t, "image-model", store.record.Definition.ModelID)
}

func TestMediaModelAdminServiceStrictValidation(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*MediaModelAdminRecord)
	}{
		{name: "model id", mutate: func(r *MediaModelAdminRecord) { r.Definition.ModelID = "bad model" }},
		{name: "vendor", mutate: func(r *MediaModelAdminRecord) { r.Definition.Vendor = "bad.vendor" }},
		{name: "adapter", mutate: func(r *MediaModelAdminRecord) { r.Definition.DefaultAdapter = "bad adapter" }},
		{name: "media type", mutate: func(r *MediaModelAdminRecord) { r.Definition.MediaType = "audio" }},
		{name: "operation", mutate: func(r *MediaModelAdminRecord) { r.Definition.Operations = []MediaOperation{"unknown"} }},
		{name: "duplicate operation", mutate: func(r *MediaModelAdminRecord) {
			r.Definition.Operations = []MediaOperation{MediaOperationTextToImage, MediaOperationTextToImage}
		}},
		{name: "async mode", mutate: func(r *MediaModelAdminRecord) { r.Definition.DefaultAsyncMode = "sometimes" }},
		{name: "constraints shape", mutate: func(r *MediaModelAdminRecord) { r.Definition.Constraints = json.RawMessage(`[]`) }},
		{name: "constraints unknown field", mutate: func(r *MediaModelAdminRecord) { r.Definition.Constraints = json.RawMessage(`{"script":"x"}`) }},
		{name: "constraints type", mutate: func(r *MediaModelAdminRecord) {
			r.Definition.Constraints = json.RawMessage(`{"max_image_count":"many"}`)
		}},
		{name: "alias equals canonical", mutate: func(r *MediaModelAdminRecord) { r.Aliases = []string{"image-model"} }},
		{name: "duplicate alias", mutate: func(r *MediaModelAdminRecord) { r.Aliases = []string{"alias", " ALIAS "} }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			record := validMediaModelAdminServiceRecord()
			tt.mutate(&record)
			_, err := normalizeAndValidateMediaModelAdminRecord(record)
			require.Error(t, err)
			require.Equal(t, "INVALID_MEDIA_MODEL", infraerrors.Reason(err))
		})
	}
}
