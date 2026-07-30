package service

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
)

type userAccessKeyRepoStub struct {
	user   *User
	byHash map[string]*User
}

func (r *userAccessKeyRepoStub) GetByID(_ context.Context, _ int64) (*User, error) {
	if r.user == nil {
		return nil, ErrUserNotFound
	}
	return r.user, nil
}

func (r *userAccessKeyRepoStub) GetByAccessKeyHash(_ context.Context, hash string) (*User, error) {
	user := r.byHash[hash]
	if user == nil {
		return nil, ErrUserNotFound
	}
	return user, nil
}

func (r *userAccessKeyRepoStub) CreateAccessKeyIfAbsent(_ context.Context, _ int64, hash, encrypted string, createdAt time.Time) (bool, error) {
	if r.user == nil {
		return false, ErrUserNotFound
	}
	if r.user.UserAccessKeyHash != nil {
		return false, nil
	}
	r.user.UserAccessKeyHash = &hash
	r.user.UserAccessKeyEncrypted = &encrypted
	r.user.UserAccessKeyCreatedAt = &createdAt
	if r.byHash == nil {
		r.byHash = make(map[string]*User)
	}
	r.byHash[hash] = r.user
	return true, nil
}

type reversibleEncryptorStub struct {
	err error
}

func (e reversibleEncryptorStub) Encrypt(plain string) (string, error) {
	if e.err != nil {
		return "", e.err
	}
	return "encrypted:" + plain, nil
}

func (e reversibleEncryptorStub) Decrypt(cipher string) (string, error) {
	if e.err != nil {
		return "", e.err
	}
	return strings.TrimPrefix(cipher, "encrypted:"), nil
}

func TestUserAccessKeyServiceGetOrCreateReturnsPersistentKey(t *testing.T) {
	user := &User{ID: 42, Status: StatusActive}
	repo := &userAccessKeyRepoStub{user: user}
	svc := NewUserAccessKeyService(repo, reversibleEncryptorStub{}, configuredUserAccessKeyTestConfig())

	first, err := svc.GetOrCreate(context.Background(), user.ID)
	if err != nil {
		t.Fatalf("GetOrCreate() error = %v", err)
	}
	if !first.Available || !first.Exists || !strings.HasPrefix(first.Key, userAccessKeyPrefix) || first.CreatedAt == nil {
		t.Fatalf("first key = %#v, want a generated persistent user access key", first)
	}
	if user.UserAccessKeyEncrypted == nil || *user.UserAccessKeyEncrypted == first.Key {
		t.Fatalf("key must be persisted in encrypted form, got %#v", user.UserAccessKeyEncrypted)
	}

	second, err := svc.GetOrCreate(context.Background(), user.ID)
	if err != nil {
		t.Fatalf("second GetOrCreate() error = %v", err)
	}
	if second.Key != first.Key || !second.Exists {
		t.Fatalf("second key = %#v, want same key %q", second, first.Key)
	}

	readBack, err := svc.Get(context.Background(), user.ID)
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if readBack.Key != first.Key {
		t.Fatalf("Get() key = %q, want %q", readBack.Key, first.Key)
	}
}

func TestUserAccessKeyServiceValidateUsesActiveKeyOwner(t *testing.T) {
	user := &User{ID: 7, Status: StatusActive}
	repo := &userAccessKeyRepoStub{user: user}
	svc := NewUserAccessKeyService(repo, reversibleEncryptorStub{}, configuredUserAccessKeyTestConfig())

	info, err := svc.GetOrCreate(context.Background(), user.ID)
	if err != nil {
		t.Fatalf("GetOrCreate() error = %v", err)
	}

	got, err := svc.Validate(context.Background(), info.Key)
	if err != nil || got.ID != user.ID {
		t.Fatalf("Validate(valid key) = (%#v, %v), want active user", got, err)
	}

	if _, err := svc.Validate(context.Background(), "uk_invalid"); !errors.Is(err, ErrInvalidUserAccessKey) {
		t.Fatalf("Validate(invalid key) error = %v, want ErrInvalidUserAccessKey", err)
	}
	if _, err := svc.Validate(context.Background(), userAccessKeyPrefix+strings.Repeat("A", 4096)); !errors.Is(err, ErrInvalidUserAccessKey) {
		t.Fatalf("Validate(oversized key) error = %v, want ErrInvalidUserAccessKey", err)
	}

	user.Status = "inactive"
	if _, err := svc.Validate(context.Background(), info.Key); !errors.Is(err, ErrInvalidUserAccessKey) {
		t.Fatalf("Validate(inactive owner) error = %v, want ErrInvalidUserAccessKey", err)
	}
}

func TestUserAccessKeyServiceGetReportsNoKeyBeforeGeneration(t *testing.T) {
	svc := NewUserAccessKeyService(
		&userAccessKeyRepoStub{user: &User{ID: 1, Status: StatusActive}},
		reversibleEncryptorStub{},
		configuredUserAccessKeyTestConfig(),
	)

	info, err := svc.Get(context.Background(), 1)
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if !info.Available || info.Exists || info.Key != "" || info.CreatedAt != nil {
		t.Fatalf("Get() = %#v, want no generated key", info)
	}
}

func TestUserAccessKeyServiceRequiresConfiguredEncryptionKey(t *testing.T) {
	user := &User{ID: 1, Status: StatusActive}
	svc := NewUserAccessKeyService(
		&userAccessKeyRepoStub{user: user},
		reversibleEncryptorStub{},
		&config.Config{},
	)

	info, err := svc.Get(context.Background(), user.ID)
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if info.Available {
		t.Fatalf("Get() = %#v, want unavailable when encryption key is not explicitly configured", info)
	}

	if _, err := svc.GetOrCreate(context.Background(), user.ID); !errors.Is(err, ErrUserAccessKeyEncryptionKeyRequired) {
		t.Fatalf("GetOrCreate() error = %v, want ErrUserAccessKeyEncryptionKeyRequired", err)
	}
	if _, err := svc.Validate(context.Background(), userAccessKeyPrefix+strings.Repeat("A", 43)); !errors.Is(err, ErrInvalidUserAccessKey) {
		t.Fatalf("Validate() error = %v, want ErrInvalidUserAccessKey", err)
	}
}

func configuredUserAccessKeyTestConfig() *config.Config {
	return &config.Config{Totp: config.TotpConfig{EncryptionKeyConfigured: true}}
}
