package service

import (
	"context"
	"testing"

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
		Name: "media", Platform: PlatformGemini, Type: AccountTypeAPIKey,
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
	require.Equal(t, "gemini", stored["adapter"])
	require.Equal(t, "optional", stored["native_async_mode"])
	overrides := stored["model_overrides"].(map[string]any)
	require.NotContains(t, overrides, "  Veo-3.1  ")
	require.Equal(t, map[string]any{
		"upstream_model":    "provider-model",
		"native_async_mode": "required",
	}, overrides["Veo-3.1"])
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
				Name: "media", Platform: PlatformGemini, Type: AccountTypeAPIKey,
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
		ID: 7, Platform: PlatformAntigravity, Type: AccountTypeAPIKey, Status: StatusActive,
		Extra: map[string]any{"old_key": "must-not-be-merged"},
	}

	updated, err := svc.UpdateAccount(context.Background(), 7, &UpdateAccountInput{Extra: map[string]any{
		"preserved": true,
		"media_config": map[string]any{
			"adapter": " Gemini ",
		},
	}})
	require.NoError(t, err)
	require.Equal(t, 1, repo.updateCalls)
	require.NotContains(t, updated.Extra, "old_key")
	require.Equal(t, true, updated.Extra["preserved"])
	stored := updated.Extra["media_config"].(map[string]any)
	require.Equal(t, "gemini", stored["adapter"])
	require.Equal(t, "unsupported", stored["native_async_mode"])
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
				ID: 8, Platform: PlatformGemini, Type: AccountTypeAPIKey, Status: StatusActive,
				Extra: map[string]any{"preserved": true},
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
