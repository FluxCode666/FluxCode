package service

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
)

const (
	userAccessKeyPrefix       = "uk_"
	userAccessKeyEntropyBytes = 32
	// base64.RawURLEncoding encodes 32 bytes into 43 URL-safe characters.
	userAccessKeyEncodedLength = len(userAccessKeyPrefix) + 43
)

var (
	// ErrInvalidUserAccessKey intentionally does not reveal whether a key used to exist.
	ErrInvalidUserAccessKey               = infraerrors.Unauthorized("INVALID_USER_ACCESS_KEY", "invalid user access key")
	ErrUserAccessKeyCorrupt               = infraerrors.InternalServer("USER_ACCESS_KEY_CORRUPT", "user access key cannot be recovered")
	ErrUserAccessKeyEncryptionKeyRequired = infraerrors.ServiceUnavailable("USER_ACCESS_KEY_ENCRYPTION_KEY_REQUIRED", "TOTP_ENCRYPTION_KEY must be configured before user access keys can be used")
)

// UserAccessKeyRepository is intentionally separate from UserRepository.  It keeps
// the developer-key persistence surface small and avoids widening every user-service
// test double with a security-specific concern.
type UserAccessKeyRepository interface {
	GetByID(ctx context.Context, userID int64) (*User, error)
	GetByAccessKeyHash(ctx context.Context, hash string) (*User, error)
	CreateAccessKeyIfAbsent(ctx context.Context, userID int64, hash, encrypted string, createdAt time.Time) (bool, error)
}

// UserAccessKeyInfo is safe to return to the key's owner over an authenticated session.
// The key is never included in the ordinary User DTOs.
type UserAccessKeyInfo struct {
	Key       string     `json:"key"`
	Exists    bool       `json:"exists"`
	Available bool       `json:"available"`
	CreatedAt *time.Time `json:"created_at,omitempty"`
}

// UserAccessKeyService manages a persistent, recoverable key for user-scoped developer APIs.
// It keeps a SHA-256 lookup value for request authentication and AES-GCM encrypted plaintext
// so the owner can copy a previously generated key again.
type UserAccessKeyService struct {
	repo      UserAccessKeyRepository
	encryptor SecretEncryptor
	available bool
}

func NewUserAccessKeyService(repo UserAccessKeyRepository, encryptor SecretEncryptor, cfg *config.Config) *UserAccessKeyService {
	// Recoverable credentials must use an externally managed AES key. The
	// development fallback lives in the application database and therefore
	// cannot provide database-backup isolation for a long-lived API credential.
	available := cfg != nil && cfg.Totp.EncryptionKeyConfigured
	return &UserAccessKeyService{repo: repo, encryptor: encryptor, available: available}
}

// Get returns the current user's existing access key, if one was generated before.
func (s *UserAccessKeyService) Get(ctx context.Context, userID int64) (*UserAccessKeyInfo, error) {
	if s == nil || s.repo == nil || s.encryptor == nil {
		return nil, fmt.Errorf("user access key service is not configured")
	}
	if !s.available {
		return &UserAccessKeyInfo{Available: false}, nil
	}

	user, err := s.repo.GetByID(ctx, userID)
	if err != nil {
		return nil, err
	}
	return s.infoFromUser(user)
}

// GetOrCreate is deliberately idempotent: repeated POSTs return the same key instead of
// silently rotating it.  This satisfies the UX requirement that any generated key remains
// available for later copying, and prevents accidental client outages.
func (s *UserAccessKeyService) GetOrCreate(ctx context.Context, userID int64) (*UserAccessKeyInfo, error) {
	if s == nil || s.repo == nil || s.encryptor == nil {
		return nil, fmt.Errorf("user access key service is not configured")
	}
	if !s.available {
		return nil, ErrUserAccessKeyEncryptionKeyRequired
	}

	existing, err := s.Get(ctx, userID)
	if err != nil {
		return nil, err
	}
	if existing.Exists {
		return existing, nil
	}

	key, err := generateUserAccessKey()
	if err != nil {
		return nil, err
	}
	encrypted, err := s.encryptor.Encrypt(key)
	if err != nil {
		return nil, fmt.Errorf("encrypt user access key: %w", err)
	}
	createdAt := time.Now().UTC()
	created, err := s.repo.CreateAccessKeyIfAbsent(ctx, userID, hashUserAccessKey(key), encrypted, createdAt)
	if err != nil {
		return nil, err
	}
	if created {
		return &UserAccessKeyInfo{Key: key, Exists: true, Available: true, CreatedAt: &createdAt}, nil
	}

	// A concurrent request created the key after the first read. Return that key instead
	// of rotating it, so every successful caller receives a valid persistent credential.
	return s.Get(ctx, userID)
}

// Validate resolves a user key to its active owner. It only compares the lookup hash and
// never decrypts the persisted secret on the request path.
func (s *UserAccessKeyService) Validate(ctx context.Context, rawKey string) (*User, error) {
	if s == nil || s.repo == nil || !s.available {
		return nil, ErrInvalidUserAccessKey
	}
	key := strings.TrimSpace(rawKey)
	if !isValidUserAccessKeyFormat(key) {
		return nil, ErrInvalidUserAccessKey
	}

	user, err := s.repo.GetByAccessKeyHash(ctx, hashUserAccessKey(key))
	if err != nil || user == nil || !user.IsActive() {
		return nil, ErrInvalidUserAccessKey
	}
	return user, nil
}

func (s *UserAccessKeyService) infoFromUser(user *User) (*UserAccessKeyInfo, error) {
	if user == nil || (user.UserAccessKeyHash == nil && user.UserAccessKeyEncrypted == nil) {
		return &UserAccessKeyInfo{Exists: false, Available: true}, nil
	}
	if user.UserAccessKeyHash == nil || user.UserAccessKeyEncrypted == nil {
		return nil, ErrUserAccessKeyCorrupt
	}
	key, err := s.encryptor.Decrypt(*user.UserAccessKeyEncrypted)
	if err != nil || strings.TrimSpace(key) == "" {
		return nil, ErrUserAccessKeyCorrupt
	}
	if hashUserAccessKey(key) != *user.UserAccessKeyHash {
		return nil, ErrUserAccessKeyCorrupt
	}
	return &UserAccessKeyInfo{
		Key:       key,
		Exists:    true,
		Available: true,
		CreatedAt: user.UserAccessKeyCreatedAt,
	}, nil
}

func generateUserAccessKey() (string, error) {
	bytes := make([]byte, userAccessKeyEntropyBytes)
	if _, err := rand.Read(bytes); err != nil {
		return "", fmt.Errorf("generate user access key: %w", err)
	}
	return userAccessKeyPrefix + base64.RawURLEncoding.EncodeToString(bytes), nil
}

func isValidUserAccessKeyFormat(key string) bool {
	if len(key) != userAccessKeyEncodedLength || !strings.HasPrefix(key, userAccessKeyPrefix) {
		return false
	}
	decoded, err := base64.RawURLEncoding.DecodeString(key[len(userAccessKeyPrefix):])
	return err == nil && len(decoded) == userAccessKeyEntropyBytes
}

func hashUserAccessKey(key string) string {
	digest := sha256.Sum256([]byte(key))
	return hex.EncodeToString(digest[:])
}
