package service

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestAPIKeyAuthSnapshotPreservesMediaGroupFlags(t *testing.T) {
	svc := &APIKeyService{}
	apiKey := &APIKey{
		Key:  "sk-media-cache",
		User: &User{},
		Group: &Group{
			AllowImageGeneration:      true,
			AllowVideoGeneration:      true,
			MediaCrossPlatformEnabled: true,
		},
	}

	snapshot := svc.snapshotFromAPIKey(apiKey)
	require.Equal(t, 8, snapshot.Version)
	require.NotNil(t, snapshot.Group)
	require.True(t, snapshot.Group.AllowImageGeneration)
	require.True(t, snapshot.Group.AllowVideoGeneration)
	require.True(t, snapshot.Group.MediaCrossPlatformEnabled)

	restored := svc.snapshotToAPIKey(apiKey.Key, snapshot)
	require.NotNil(t, restored.Group)
	require.True(t, restored.Group.AllowImageGeneration)
	require.True(t, restored.Group.AllowVideoGeneration)
	require.True(t, restored.Group.MediaCrossPlatformEnabled)
}
