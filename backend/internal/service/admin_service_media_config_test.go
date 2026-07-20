package service

import (
	"context"
	"net/http"
	"testing"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/stretchr/testify/require"
)

type adminServiceMediaConfigRepo struct {
	AccountRepository
	account     *Account
	created     *Account
	createCalls int
	updateCalls int
}

func (r *adminServiceMediaConfigRepo) Create(_ context.Context, account *Account) error {
	r.createCalls++
	r.created = account
	return nil
}

func (r *adminServiceMediaConfigRepo) GetByID(_ context.Context, _ int64) (*Account, error) {
	return r.account, nil
}

func (r *adminServiceMediaConfigRepo) Update(_ context.Context, account *Account) error {
	r.updateCalls++
	r.account = account
	return nil
}

func (r *adminServiceMediaConfigRepo) LastCreated() *Account {
	return r.created
}

func newAdminServiceMediaConfigFixture(t *testing.T) (*adminServiceImpl, *adminServiceMediaConfigRepo) {
	t.Helper()
	repo := &adminServiceMediaConfigRepo{}
	return &adminServiceImpl{accountRepo: repo}, repo
}

func TestAdminServiceCreateAccountNormalizesMediaConfig(t *testing.T) {
	svc, repo := newAdminServiceMediaConfigFixture(t)
	_, err := svc.CreateAccount(context.Background(), &CreateAccountInput{
		Name: "media", Platform: PlatformMedia, Type: AccountTypeAPIKey,
		Credentials:          map[string]any{"api_key": "media-key", "base_url": "https://media.example.com"},
		SkipDefaultGroupBind: true,
		Extra: map[string]any{
			"preserved": "value",
			"media_config": map[string]any{
				"adapter": " GeMiNi ", "native_async_mode": "OPTIONAL",
				"model_overrides": map[string]any{
					"  Veo-3.1  ": map[string]any{
						"upstream_model": " provider-model ", "native_async_mode": "REQUIRED",
					},
				},
			},
		},
	})
	require.NoError(t, err)
	require.Equal(t, 1, repo.createCalls)
	require.Equal(t, "value", repo.LastCreated().Extra["preserved"])
	stored := repo.LastCreated().Extra["media_config"].(map[string]any)
	require.Equal(t, 1, stored["version"])
	require.Equal(t, "gemini", stored["provider"])
	models := stored["models"].(map[string]any)
	require.NotContains(t, models, "  Veo-3.1  ")
	require.Equal(t, map[string]any{
		"enabled":           true,
		"upstream_model_id": "provider-model",
		"async_mode":        "native",
		"request_mapping":   MediaRequestMapping{},
	}, models["Veo-3.1"])
}

func TestAdminServiceCreateAccountAllowsExtraWithoutMediaConfig(t *testing.T) {
	svc, repo := newAdminServiceMediaConfigFixture(t)
	_, err := svc.CreateAccount(context.Background(), &CreateAccountInput{
		Name: "legacy", Platform: PlatformGemini, Type: AccountTypeAPIKey,
		SkipDefaultGroupBind: true,
		Extra:                map[string]any{"legacy": true},
	})
	require.NoError(t, err)
	require.Equal(t, 1, repo.createCalls)
	require.Equal(t, map[string]any{"legacy": true}, repo.LastCreated().Extra)
}

func TestAdminServiceCreateAccountRejectsMediaConfigOnTextPlatform(t *testing.T) {
	svc, repo := newAdminServiceMediaConfigFixture(t)
	_, err := svc.CreateAccount(context.Background(), &CreateAccountInput{
		Name: "text", Platform: PlatformGemini, Type: AccountTypeAPIKey,
		SkipDefaultGroupBind: true,
		Extra: map[string]any{"media_config": map[string]any{
			"version": 1, "provider": "xai", "models": map[string]any{
				"image": map[string]any{"enabled": true, "upstream_model_id": "image-up", "async_mode": "unsupported"},
			},
		}},
	})
	require.ErrorIs(t, err, ErrInvalidMediaAccountConfig)
	require.Zero(t, repo.createCalls)
}

func TestAdminServiceCreateAccountRequiresCompleteMediaIdentity(t *testing.T) {
	tests := []struct {
		name        string
		credentials map[string]any
		extra       map[string]any
	}{
		{name: "api key", credentials: map[string]any{"base_url": "https://media.example.com"}, extra: validMediaAccountExtra()},
		{name: "base url", credentials: map[string]any{"api_key": "media-key"}, extra: validMediaAccountExtra()},
		{name: "config", credentials: map[string]any{"api_key": "media-key", "base_url": "https://media.example.com"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc, repo := newAdminServiceMediaConfigFixture(t)
			_, err := svc.CreateAccount(context.Background(), &CreateAccountInput{
				Name: "media", Platform: PlatformMedia, Type: AccountTypeAPIKey,
				Credentials: tt.credentials, Extra: tt.extra, SkipDefaultGroupBind: true,
			})
			require.Error(t, err)
			require.Equal(t, http.StatusBadRequest, infraerrors.Code(err))
			require.Zero(t, repo.createCalls)
		})
	}
}

func validMediaAccountExtra() map[string]any {
	return map[string]any{"media_config": map[string]any{
		"version": 1, "provider": "xai", "models": map[string]any{
			"image": map[string]any{"enabled": true, "upstream_model_id": "image-up", "async_mode": "unsupported"},
		},
	}}
}

func TestAdminServiceCreateAccountRejectsMalformedMediaConfigBeforePersist(t *testing.T) {
	tests := []struct {
		name  string
		raw   any
		isErr error
	}{
		{name: "wrong shape", raw: "gemini", isErr: ErrInvalidMediaAccountConfig},
		{name: "empty config", raw: map[string]any{}, isErr: ErrInvalidMediaAccountConfig},
		{name: "wrong adapter type", raw: map[string]any{"adapter": 42}, isErr: ErrInvalidMediaAccountConfig},
		{name: "native mode null", raw: map[string]any{"adapter": "gemini", "native_async_mode": nil}, isErr: ErrInvalidMediaAccountConfig},
		{name: "unknown mode", raw: map[string]any{"adapter": "gemini", "native_async_mode": "sometimes"}, isErr: ErrInvalidNativeAsyncMode},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc, repo := newAdminServiceMediaConfigFixture(t)
			_, err := svc.CreateAccount(context.Background(), &CreateAccountInput{
				Name: "media", Platform: PlatformMedia, Type: AccountTypeAPIKey,
				Credentials:          map[string]any{"api_key": "media-key", "base_url": "https://media.example.com"},
				SkipDefaultGroupBind: true,
				Extra:                map[string]any{"media_config": tt.raw, "preserved": true},
			})
			require.ErrorIs(t, err, tt.isErr)
			require.Zero(t, repo.createCalls)
		})
	}
}

func TestAdminServiceUpdateAccountNormalizesMediaConfigAndKeepsPayloadExtra(t *testing.T) {
	svc, repo := newAdminServiceMediaConfigFixture(t)
	repo.account = &Account{
		ID: 7, Platform: PlatformMedia, Type: AccountTypeAPIKey, Status: StatusActive,
		Credentials: map[string]any{"api_key": "media-key", "base_url": "https://media.example.com"},
		Extra:       map[string]any{"old_key": "must-not-be-merged"},
	}

	updated, err := svc.UpdateAccount(context.Background(), 7, &UpdateAccountInput{Extra: map[string]any{
		"preserved": true,
		"media_config": map[string]any{
			"adapter":         " Gemini ",
			"model_overrides": map[string]any{"veo": map[string]any{"upstream_model": "veo-up"}},
		},
	}})
	require.NoError(t, err)
	require.Equal(t, 1, repo.updateCalls)
	require.NotContains(t, updated.Extra, "old_key")
	require.Equal(t, true, updated.Extra["preserved"])
	stored := updated.Extra["media_config"].(map[string]any)
	require.Equal(t, 1, stored["version"])
	require.Equal(t, "gemini", stored["provider"])
	require.Contains(t, stored["models"].(map[string]any), "veo")
}

func TestAdminServiceUpdateAccountRejectsInvalidMediaConfigBeforePersist(t *testing.T) {
	tests := []struct {
		name  string
		raw   any
		isErr error
	}{
		{name: "unknown mode", raw: map[string]any{
			"adapter": "gemini", "native_async_mode": "sometimes",
		}, isErr: ErrInvalidNativeAsyncMode},
		{name: "override upstream model null", raw: map[string]any{
			"adapter": "gemini",
			"model_overrides": map[string]any{
				"veo": map[string]any{"upstream_model": nil},
			},
		}, isErr: ErrInvalidMediaAccountConfig},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc, repo := newAdminServiceMediaConfigFixture(t)
			repo.account = &Account{
				ID: 8, Platform: PlatformMedia, Type: AccountTypeAPIKey, Status: StatusActive,
				Credentials: map[string]any{"api_key": "media-key", "base_url": "https://media.example.com"},
				Extra:       map[string]any{"preserved": true},
			}

			_, err := svc.UpdateAccount(context.Background(), 8, &UpdateAccountInput{Extra: map[string]any{
				"media_config": tt.raw,
			}})
			require.ErrorIs(t, err, tt.isErr)
			require.Zero(t, repo.updateCalls)
			require.Equal(t, map[string]any{"preserved": true}, repo.account.Extra)
		})
	}
}

func TestAccountServiceUpdateRejectsInvalidMediaConfigBeforeMutatingLoadedAccount(t *testing.T) {
	repo := &adminServiceMediaConfigRepo{account: &Account{
		ID: 9, Name: "before", Platform: PlatformMedia, Type: AccountTypeAPIKey,
		Credentials: map[string]any{"api_key": "media-key", "base_url": "https://media.example.com"},
		Extra:       map[string]any{"preserved": true},
	}}
	svc := NewAccountService(repo, nil)
	after := "after"
	invalidExtra := map[string]any{"media_config": map[string]any{
		"version": 1, "provider": "xai", "models": map[string]any{
			"image": map[string]any{"enabled": true, "upstream_model_id": "up", "async_mode": "sometimes"},
		},
	}}

	_, err := svc.Update(context.Background(), 9, UpdateAccountRequest{Name: &after, Extra: &invalidExtra})
	require.ErrorIs(t, err, ErrInvalidMediaAccountConfig)
	require.Equal(t, "before", repo.account.Name)
	require.Equal(t, map[string]any{"preserved": true}, repo.account.Extra)
	require.Zero(t, repo.updateCalls)
}

func TestAccountServiceCreateNormalizesVersionOneMediaConfig(t *testing.T) {
	repo := &adminServiceMediaConfigRepo{}
	svc := NewAccountService(repo, nil)

	created, err := svc.Create(context.Background(), CreateAccountRequest{
		Name: "media", Platform: PlatformMedia, Type: AccountTypeAPIKey,
		Credentials: map[string]any{"api_key": "media-key", "base_url": "https://media.example.com"},
		Extra: map[string]any{"media_config": map[string]any{
			"version": 1, "provider": " xai ", "models": map[string]any{
				" image ": map[string]any{"enabled": true, "upstream_model_id": " image-up ", "async_mode": "native"},
			},
		}},
	})
	require.NoError(t, err)
	require.Equal(t, 1, repo.createCalls)
	stored := created.Extra[mediaAccountConfigExtraKey].(map[string]any)
	require.Equal(t, "xai", stored["provider"])
	require.Contains(t, stored["models"].(map[string]any), "image")
}

func TestAccountServiceUpdateUpgradesLegacyMediaConfigOnOrdinarySave(t *testing.T) {
	repo := &adminServiceMediaConfigRepo{account: &Account{
		ID: 10, Name: "before", Platform: PlatformMedia, Type: AccountTypeAPIKey,
		Credentials: map[string]any{"api_key": "media-key", "base_url": "https://media.example.com"},
		Extra: map[string]any{"media_config": map[string]any{
			"adapter": "gemini", "native_async_mode": "optional",
			"model_overrides": map[string]any{"veo": map[string]any{"upstream_model": "veo-up"}},
		}},
	}}
	svc := NewAccountService(repo, nil)
	after := "after"

	updated, err := svc.Update(context.Background(), 10, UpdateAccountRequest{Name: &after})
	require.NoError(t, err)
	require.Equal(t, 1, repo.updateCalls)
	stored := updated.Extra[mediaAccountConfigExtraKey].(map[string]any)
	require.Equal(t, 1, stored["version"])
	require.Equal(t, "gemini", stored["provider"])
	require.Contains(t, stored["models"].(map[string]any), "veo")
}
