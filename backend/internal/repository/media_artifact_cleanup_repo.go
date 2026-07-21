package repository

import (
	"context"
	"errors"
	"strings"

	"github.com/Wei-Shaw/sub2api/ent/mediaartifact"
	"github.com/Wei-Shaw/sub2api/internal/service"
)

// DeleteExact is intentionally narrower than a general ID delete. It is used
// only to compensate rows written by an unsuccessful media workflow and
// matches every immutable locator that identifies that exact row.
func (r *mediaArtifactRepository) DeleteExact(ctx context.Context, artifact *service.MediaArtifact) (bool, error) {
	if r == nil || r.client == nil {
		return false, errors.New("media artifact repository is unavailable")
	}
	if artifact == nil || artifact.ID <= 0 || artifact.TaskID <= 0 ||
		(artifact.Direction != "input" && artifact.Direction != "output") || artifact.Position < 0 {
		return false, errors.New("media artifact identity is invalid")
	}

	deleted, err := r.client.MediaArtifact.Delete().
		Where(
			mediaartifact.IDEQ(artifact.ID),
			mediaartifact.TaskIDEQ(artifact.TaskID),
			mediaartifact.DirectionEQ(artifact.Direction),
			mediaartifact.PositionEQ(artifact.Position),
			mediaartifact.StorageProviderEQ(normalizedMediaArtifactStorageProvider(artifact.StorageProvider)),
			mediaartifact.ObjectKeyEQ(strings.TrimSpace(artifact.ObjectKey)),
			mediaartifact.ChecksumSha256EQ(strings.TrimSpace(artifact.ChecksumSHA256)),
		).
		Exec(ctx)
	if err != nil {
		return false, err
	}
	return deleted == 1, nil
}
