package service

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"testing"

	"github.com/stretchr/testify/require"
)

type mediaInputArtifactRepoStub struct{ artifacts []MediaArtifact }

func (r mediaInputArtifactRepoStub) Create(context.Context, *MediaArtifact) (*MediaArtifact, error) {
	return nil, nil
}
func (r mediaInputArtifactRepoStub) DeleteExact(context.Context, *MediaArtifact) (bool, error) {
	return false, nil
}
func (r mediaInputArtifactRepoStub) ListByTaskID(context.Context, int64) ([]MediaArtifact, error) {
	return append([]MediaArtifact(nil), r.artifacts...), nil
}

type mediaInputObjectStoreStub struct{ data map[string][]byte }

func (s mediaInputObjectStoreStub) Put(context.Context, MediaArtifactInput) (*MediaArtifact, error) {
	return nil, nil
}
func (s mediaInputObjectStoreStub) Open(_ context.Context, artifact *MediaArtifact, _ string) (*MediaContent, error) {
	data := s.data[artifact.ObjectKey]
	return &MediaContent{
		Body: io.NopCloser(bytes.NewReader(data)), StatusCode: 200,
		ContentType: artifact.ContentType, ContentLength: int64(len(data)),
	}, nil
}
func (s mediaInputObjectStoreStub) Discard(context.Context, MediaArtifactInput) error { return nil }

func TestMediaContentServiceMaterializesInputsInSpecOrder(t *testing.T) {
	first := testMediaPNG(t)
	second := append([]byte(nil), first...)
	firstChecksum := sha256.Sum256(first)
	secondChecksum := sha256.Sum256(second)
	repo := mediaInputArtifactRepoStub{artifacts: []MediaArtifact{
		{ID: 11, TaskID: 7, Direction: "input", Position: 0, MediaType: MediaTypeImage, ContentType: "image/png", SizeBytes: int64(len(first)), ChecksumSHA256: hex.EncodeToString(firstChecksum[:]), StorageProvider: MediaStorageProviderLocal, ObjectKey: "tasks/first.png"},
		{ID: 12, TaskID: 7, Direction: "input", Position: 1, MediaType: MediaTypeImage, ContentType: "image/png", SizeBytes: int64(len(second)), ChecksumSHA256: hex.EncodeToString(secondChecksum[:]), StorageProvider: MediaStorageProviderLocal, ObjectKey: "tasks/second.png"},
	}}
	svc := NewMediaContentService(nil, repo, nil, nil, nil, nil, mediaInputObjectStoreStub{data: map[string][]byte{
		"tasks/first.png": first, "tasks/second.png": second,
	}})

	inputs, err := svc.LoadInputs(context.Background(), &MediaTask{ID: 7}, MediaSpec{Image: &ImageSpec{
		Prompt: "edit", Count: 1, InputArtifactIDs: []int64{12, 11},
	}}, &Account{ID: 1})
	require.NoError(t, err)
	require.Len(t, inputs, 2)
	require.Equal(t, 0, inputs[0].Position)
	require.Equal(t, "tasks/second.png", inputs[0].ObjectKey)
	require.Equal(t, second, inputs[0].Data)
	require.Equal(t, 1, inputs[1].Position)
	require.Equal(t, "tasks/first.png", inputs[1].ObjectKey)
}

func TestMediaContentServiceRejectsDuplicateOrForeignInputArtifact(t *testing.T) {
	repo := mediaInputArtifactRepoStub{artifacts: []MediaArtifact{{
		ID: 11, TaskID: 8, Direction: "input", MediaType: MediaTypeImage, ContentType: "image/png",
		StorageProvider: MediaStorageProviderLocal, ObjectKey: "tasks/first.png",
	}}}
	svc := NewMediaContentService(nil, repo, nil, nil, nil, nil, mediaInputObjectStoreStub{})

	_, err := svc.LoadInputs(context.Background(), &MediaTask{ID: 7}, MediaSpec{Image: &ImageSpec{
		Prompt: "edit", Count: 1, InputArtifactIDs: []int64{11, 11},
	}}, &Account{ID: 1})
	require.ErrorIs(t, err, ErrInvalidMediaInput)
}
