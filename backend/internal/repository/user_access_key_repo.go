package repository

import (
	"context"
	"time"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	dbuser "github.com/Wei-Shaw/sub2api/ent/user"
	"github.com/Wei-Shaw/sub2api/internal/service"
)

// userAccessKeyRepository owns only the storage paths required by the user developer key.
// Keeping it separate from userRepository makes regular user updates unable to accidentally
// write plaintext or rotate the access credential.
type userAccessKeyRepository struct {
	client *dbent.Client
}

func NewUserAccessKeyRepository(client *dbent.Client) service.UserAccessKeyRepository {
	return &userAccessKeyRepository{client: client}
}

func (r *userAccessKeyRepository) GetByID(ctx context.Context, userID int64) (*service.User, error) {
	entity, err := r.client.User.Query().Where(dbuser.IDEQ(userID)).Only(ctx)
	if err != nil {
		return nil, translatePersistenceError(err, service.ErrUserNotFound, nil)
	}
	return userEntityToService(entity), nil
}

func (r *userAccessKeyRepository) GetByAccessKeyHash(ctx context.Context, hash string) (*service.User, error) {
	entity, err := r.client.User.Query().Where(
		dbuser.UserAccessKeyHashEQ(hash),
		dbuser.DeletedAtIsNil(),
	).Only(ctx)
	if err != nil {
		return nil, translatePersistenceError(err, service.ErrUserNotFound, nil)
	}
	return userEntityToService(entity), nil
}

// CreateAccessKeyIfAbsent atomically persists the first generated key. If another request
// already won the race, false is returned and the caller reads that existing key instead.
func (r *userAccessKeyRepository) CreateAccessKeyIfAbsent(
	ctx context.Context,
	userID int64,
	hash string,
	encrypted string,
	createdAt time.Time,
) (bool, error) {
	affected, err := r.client.User.Update().
		Where(dbuser.IDEQ(userID), dbuser.UserAccessKeyHashIsNil()).
		SetUserAccessKeyHash(hash).
		SetUserAccessKeyEncrypted(encrypted).
		SetUserAccessKeyCreatedAt(createdAt).
		Save(ctx)
	if err != nil {
		return false, translatePersistenceError(err, service.ErrUserNotFound, nil)
	}
	return affected > 0, nil
}
