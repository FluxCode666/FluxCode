package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
)

const MediaStorageSettingKey = "media_storage_config"

var (
	// ErrMediaStorageConfigChanged indicates that an object was written with a
	// storage configuration which is no longer current. Callers must discard the
	// uncommitted object instead of creating an unreadable artifact index.
	ErrMediaStorageConfigChanged = errors.New("media storage configuration changed")
	// ErrMediaStorageLocationInUse is returned by the guarded settings commit
	// when a locator changed after the optimistic service-layer usage check and a
	// historical artifact now depends on it.
	ErrMediaStorageLocationInUse = errors.New("media storage location is in use")
	// ErrMediaStorageCommitOutcomeUnknown means PostgreSQL did not confirm
	// whether COMMIT succeeded and a detached reconciliation query also failed.
	// Callers must retain the object because deleting it could break a committed
	// Artifact row.
	ErrMediaStorageCommitOutcomeUnknown = errors.New("media storage commit outcome is unknown")
)

// MediaStorageConsistencyRepository is the database serialization boundary
// shared by storage settings updates and artifact index creation. Production
// implementations must acquire the same cross-instance lock and perform the
// revision check plus the final write in one database transaction.
type MediaStorageConsistencyRepository interface {
	CommitConfig(
		ctx context.Context,
		expectedRevision string,
		encodedConfig string,
		changedProviders []string,
	) error
	CommitArtifact(
		ctx context.Context,
		expectedRevision string,
		artifact *MediaArtifact,
	) (*MediaArtifact, error)
}

// MediaStorageSettingRevision is an opaque, non-secret revision derived from
// the exact encrypted settings row. It is therefore stable across processes
// sharing a database, while still distinguishing a missing row from an empty
// value. Secrets are never copied into task or artifact data.
func MediaStorageSettingRevision(raw string, found bool) string {
	prefix := byte(0)
	if found {
		prefix = 1
	}
	hash := sha256.New()
	_, _ = hash.Write([]byte{prefix})
	_, _ = hash.Write([]byte(raw))
	return hex.EncodeToString(hash.Sum(nil))
}

func commitMediaArtifact(
	ctx context.Context,
	repo MediaArtifactRepository,
	consistency MediaStorageConsistencyRepository,
	artifact *MediaArtifact,
) (*MediaArtifact, error) {
	if artifact == nil {
		return nil, ErrInvalidMediaInput
	}
	if repo == nil {
		return nil, ErrMediaContentUnavailable
	}
	if artifact.StorageRevision == "" {
		return repo.Create(ctx, artifact)
	}
	if consistency == nil {
		return nil, ErrMediaContentUnavailable
	}
	return consistency.CommitArtifact(ctx, artifact.StorageRevision, artifact)
}
