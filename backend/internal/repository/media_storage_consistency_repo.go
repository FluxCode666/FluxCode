package repository

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"
	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/ent/mediaartifact"
	"github.com/Wei-Shaw/sub2api/ent/setting"
	"github.com/Wei-Shaw/sub2api/internal/service"
)

// mediaStorageConsistencyAdvisoryLockID is shared by every application
// instance. The ASCII value is "MEDIASTO" and fits in PostgreSQL bigint.
const mediaStorageConsistencyAdvisoryLockID int64 = 0x4d4544494153544f

type mediaStorageConsistencyRepository struct {
	db *sql.DB
}

func NewMediaStorageConsistencyRepository(db *sql.DB) service.MediaStorageConsistencyRepository {
	return &mediaStorageConsistencyRepository{db: db}
}

func (r *mediaStorageConsistencyRepository) CommitConfig(
	ctx context.Context,
	expectedRevision string,
	encodedConfig string,
	changedProviders []string,
) error {
	if strings.TrimSpace(expectedRevision) == "" || strings.TrimSpace(encodedConfig) == "" {
		return service.ErrMediaStorageIntegrity
	}
	err := r.withLockedTx(ctx, func(client *dbent.Client) error {
		current, err := mediaStorageSettingRevisionInTx(ctx, client)
		if err != nil {
			return err
		}
		if current != expectedRevision {
			return service.ErrMediaStorageConfigChanged
		}
		seen := make(map[string]struct{}, len(changedProviders))
		for _, rawProvider := range changedProviders {
			provider := strings.ToLower(strings.TrimSpace(rawProvider))
			if provider != service.MediaStorageProviderLocal && provider != service.MediaStorageProviderMinIO {
				return service.ErrInvalidMediaStorageProvider
			}
			if _, duplicate := seen[provider]; duplicate {
				continue
			}
			seen[provider] = struct{}{}
			inUse, err := client.MediaArtifact.Query().
				Where(mediaartifact.StorageProviderEQ(provider)).
				Exist(ctx)
			if err != nil {
				return fmt.Errorf("check media storage artifacts for provider %s: %w", provider, err)
			}
			if inUse {
				return fmt.Errorf("%w: %s", service.ErrMediaStorageLocationInUse, provider)
			}
		}
		return client.Setting.Create().
			SetKey(service.MediaStorageSettingKey).
			SetValue(encodedConfig).
			OnConflictColumns(setting.FieldKey).
			UpdateNewValues().
			Exec(ctx)
	})
	if err != nil && errors.Is(err, service.ErrMediaStorageCommitOutcomeUnknown) {
		return r.reconcileConfigCommit(ctx, encodedConfig, err)
	}
	return err
}

func (r *mediaStorageConsistencyRepository) reconcileConfigCommit(
	ctx context.Context,
	encodedConfig string,
	commitErr error,
) error {
	reconcileCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer cancel()
	var stored string
	err := r.db.QueryRowContext(
		reconcileCtx,
		"SELECT value FROM settings WHERE key = $1",
		service.MediaStorageSettingKey,
	).Scan(&stored)
	switch {
	case err == nil && stored == encodedConfig:
		return nil
	case err == nil:
		return service.ErrMediaStorageConfigChanged
	case errors.Is(err, sql.ErrNoRows):
		return fmt.Errorf("%w: guarded config commit was not applied: %v", service.ErrMediaStorageIntegrity, commitErr)
	default:
		return errors.Join(commitErr, fmt.Errorf("reconcile guarded config commit: %w", err))
	}
}

func (r *mediaStorageConsistencyRepository) CommitArtifact(
	ctx context.Context,
	expectedRevision string,
	artifact *service.MediaArtifact,
) (created *service.MediaArtifact, err error) {
	if strings.TrimSpace(expectedRevision) == "" || artifact == nil {
		return nil, service.ErrMediaStorageIntegrity
	}
	err = r.withLockedTx(ctx, func(client *dbent.Client) error {
		existing, lookupErr := mediaStorageArtifactAtSlot(ctx, client, artifact)
		switch {
		case lookupErr == nil:
			if !mediaArtifactContentIdentityEqual(existing, artifact) {
				return fmt.Errorf(
					"%w for task %d %s position %d",
					service.ErrMediaArtifactConflict,
					artifact.TaskID,
					artifact.Direction,
					artifact.Position,
				)
			}
			existing.StorageRevision = artifact.StorageRevision
			created = existing
			return nil
		case !dbent.IsNotFound(lookupErr):
			return fmt.Errorf("load existing media artifact before guarded commit: %w", lookupErr)
		}

		current, revisionErr := mediaStorageSettingRevisionInTx(ctx, client)
		if revisionErr != nil {
			return revisionErr
		}
		if current != expectedRevision {
			return service.ErrMediaStorageConfigChanged
		}
		created, revisionErr = (&mediaArtifactRepository{client: client}).Create(ctx, artifact)
		if revisionErr == nil && created != nil {
			// Keep the optimistic revision alive for immediate compensation. It is
			// intentionally not persisted in media_artifacts.
			created.StorageRevision = artifact.StorageRevision
		}
		return revisionErr
	})
	if err != nil && errors.Is(err, service.ErrMediaStorageCommitOutcomeUnknown) {
		return r.reconcileArtifactCommit(ctx, artifact, err)
	}
	return created, err
}

func (r *mediaStorageConsistencyRepository) reconcileArtifactCommit(
	ctx context.Context,
	artifact *service.MediaArtifact,
	commitErr error,
) (*service.MediaArtifact, error) {
	reconcileCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer cancel()
	driver := entsql.OpenDB(dialect.Postgres, r.db)
	client := dbent.NewClient(dbent.Driver(driver))
	existing, err := mediaStorageArtifactAtSlot(reconcileCtx, client, artifact)
	if err == nil {
		if !mediaArtifactContentIdentityEqual(existing, artifact) {
			return nil, fmt.Errorf(
				"%w for task %d %s position %d after ambiguous commit",
				service.ErrMediaArtifactConflict,
				artifact.TaskID,
				artifact.Direction,
				artifact.Position,
			)
		}
		existing.StorageRevision = artifact.StorageRevision
		return existing, nil
	}
	if dbent.IsNotFound(err) {
		// The primary database definitively has no row, so callers may safely
		// compensate the object written before this transaction.
		return nil, fmt.Errorf("%w: guarded artifact commit was not applied: %v", service.ErrMediaStorageIntegrity, commitErr)
	}
	return nil, errors.Join(commitErr, fmt.Errorf("reconcile guarded artifact commit: %w", err))
}

func (r *mediaStorageConsistencyRepository) withLockedTx(
	ctx context.Context,
	fn func(*dbent.Client) error,
) error {
	if r == nil || r.db == nil || fn == nil {
		return errors.New("media storage consistency repository is unavailable")
	}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin media storage consistency transaction: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck
	if _, err := tx.ExecContext(ctx, "SELECT pg_advisory_xact_lock($1)", mediaStorageConsistencyAdvisoryLockID); err != nil {
		return fmt.Errorf("lock media storage consistency transaction: %w", err)
	}
	driver := entsql.NewDriver(dialect.Postgres, entsql.Conn{ExecQuerier: tx})
	client := dbent.NewClient(dbent.Driver(driver))
	if err := fn(client); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return errors.Join(
			service.ErrMediaStorageCommitOutcomeUnknown,
			fmt.Errorf("commit media storage consistency transaction: %w", err),
		)
	}
	return nil
}

func mediaStorageArtifactAtSlot(
	ctx context.Context,
	client *dbent.Client,
	artifact *service.MediaArtifact,
) (*service.MediaArtifact, error) {
	stored, err := client.MediaArtifact.Query().
		Where(
			mediaartifact.TaskIDEQ(artifact.TaskID),
			mediaartifact.DirectionEQ(artifact.Direction),
			mediaartifact.PositionEQ(artifact.Position),
		).
		Only(ctx)
	if err != nil {
		return nil, err
	}
	return mediaArtifactFromEnt(stored), nil
}

func mediaStorageSettingRevisionInTx(ctx context.Context, client *dbent.Client) (string, error) {
	stored, err := client.Setting.Query().
		Where(setting.KeyEQ(service.MediaStorageSettingKey)).
		Only(ctx)
	if err == nil {
		return service.MediaStorageSettingRevision(stored.Value, true), nil
	}
	if dbent.IsNotFound(err) {
		return service.MediaStorageSettingRevision("", false), nil
	}
	return "", fmt.Errorf("load media storage setting revision: %w", err)
}
