package service

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestEmailService_SendEmailWithResendConfig_PostsPayload(t *testing.T) {
	var gotAuth string
	var gotContentType string
	var gotUserAgent string
	var gotPayload resendEmailRequest

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, http.MethodPost, r.Method)
		gotAuth = r.Header.Get("Authorization")
		gotContentType = r.Header.Get("Content-Type")
		gotUserAgent = r.Header.Get("User-Agent")
		require.NoError(t, json.NewDecoder(r.Body).Decode(&gotPayload))
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"id":"email_123"}`))
	}))
	defer server.Close()

	svc := NewEmailService(nil, nil)
	svc.httpClient = server.Client()
	svc.resendURL = server.URL

	err := svc.SendEmailWithResendConfig(context.Background(), &ResendConfig{
		APIKey:   "re_test",
		From:     "no-reply@example.com",
		FromName: "FluxCode",
	}, "user@example.com", "Subject\r\nInjected", "<p>Hello</p>")
	require.NoError(t, err)
	require.Equal(t, "Bearer re_test", gotAuth)
	require.Equal(t, "application/json", gotContentType)
	require.Equal(t, "FluxCode/1.0", gotUserAgent)
	require.Equal(t, "FluxCode <no-reply@example.com>", gotPayload.From)
	require.Equal(t, []string{"user@example.com"}, gotPayload.To)
	require.Equal(t, "SubjectInjected", gotPayload.Subject)
	require.Equal(t, "<p>Hello</p>", gotPayload.HTML)
}

func TestEmailService_SendEmailWithResendConfig_ReturnsAPIError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnprocessableEntity)
		_, _ = w.Write([]byte(`{"message":"invalid from address"}`))
	}))
	defer server.Close()

	svc := NewEmailService(nil, nil)
	svc.httpClient = server.Client()
	svc.resendURL = server.URL

	err := svc.SendEmailWithResendConfig(context.Background(), &ResendConfig{
		APIKey: "re_test",
		From:   "no-reply@example.com",
	}, "user@example.com", "Subject", "<p>Hello</p>")
	require.Error(t, err)
	require.Contains(t, err.Error(), "status 422")
	require.Contains(t, err.Error(), "invalid from address")
}

func TestEmailService_GetEmailConfig_UsesResendProvider(t *testing.T) {
	repo := &resendEmailSettingRepoStub{
		values: map[string]string{
			SettingKeyEmailProvider:  EmailProviderResend,
			SettingKeyResendAPIKey:   "re_test",
			SettingKeyResendFrom:     "no-reply@example.com",
			SettingKeyResendFromName: "FluxCode",
		},
	}
	svc := NewEmailService(repo, nil)

	config, err := svc.GetEmailConfig(context.Background())

	require.NoError(t, err)
	require.Equal(t, EmailProviderResend, config.Provider)
	require.Nil(t, config.SMTP)
	require.Equal(t, &ResendConfig{
		APIKey:   "re_test",
		From:     "no-reply@example.com",
		FromName: "FluxCode",
	}, config.Resend)
}

func TestEmailService_SendEmail_UsesResendProvider(t *testing.T) {
	var gotPayload resendEmailRequest
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, http.MethodPost, r.Method)
		require.Equal(t, "Bearer re_test", r.Header.Get("Authorization"))
		require.NoError(t, json.NewDecoder(r.Body).Decode(&gotPayload))
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"id":"email_123"}`))
	}))
	defer server.Close()

	repo := &resendEmailSettingRepoStub{
		values: map[string]string{
			SettingKeyEmailProvider:  EmailProviderResend,
			SettingKeyResendAPIKey:   "re_test",
			SettingKeyResendFrom:     "no-reply@example.com",
			SettingKeyResendFromName: "FluxCode",
		},
	}
	svc := NewEmailService(repo, nil)
	svc.httpClient = server.Client()
	svc.resendURL = server.URL

	err := svc.SendEmail(context.Background(), "user@example.com", "Subject", "<p>Hello</p>")

	require.NoError(t, err)
	require.Equal(t, "FluxCode <no-reply@example.com>", gotPayload.From)
	require.Equal(t, []string{"user@example.com"}, gotPayload.To)
	require.Equal(t, "Subject", gotPayload.Subject)
	require.Equal(t, "<p>Hello</p>", gotPayload.HTML)
}

type resendEmailSettingRepoStub struct {
	values map[string]string
}

func (s *resendEmailSettingRepoStub) Get(ctx context.Context, key string) (*Setting, error) {
	panic("unexpected Get call")
}

func (s *resendEmailSettingRepoStub) GetValue(ctx context.Context, key string) (string, error) {
	panic("unexpected GetValue call")
}

func (s *resendEmailSettingRepoStub) Set(ctx context.Context, key, value string) error {
	panic("unexpected Set call")
}

func (s *resendEmailSettingRepoStub) GetMultiple(ctx context.Context, keys []string) (map[string]string, error) {
	result := make(map[string]string, len(keys))
	for _, key := range keys {
		result[key] = s.values[key]
	}
	return result, nil
}

func (s *resendEmailSettingRepoStub) SetMultiple(ctx context.Context, settings map[string]string) error {
	panic("unexpected SetMultiple call")
}

func (s *resendEmailSettingRepoStub) GetAll(ctx context.Context) (map[string]string, error) {
	panic("unexpected GetAll call")
}

func (s *resendEmailSettingRepoStub) Delete(ctx context.Context, key string) error {
	panic("unexpected Delete call")
}
