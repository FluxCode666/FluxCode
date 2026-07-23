package service

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func swapMonitorHTTPClient(t *testing.T) {
	t.Helper()
	orig := monitorHTTPClient
	monitorHTTPClient = &http.Client{Timeout: 5 * time.Second}
	t.Cleanup(func() { monitorHTTPClient = orig })
}

func TestRunCheckForModel_RejectsRedirectsToDifferentOrigin(t *testing.T) {
	orig := monitorHTTPClient
	monitorHTTPClient = &http.Client{
		Timeout: 5 * time.Second,
		Transport: &http.Transport{
			ForceAttemptHTTP2: true,
		},
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
	t.Cleanup(func() { monitorHTTPClient = orig })

	var redirectedHits int
	redirectTarget := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		redirectedHits++
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"choices": []map[string]any{{"message": map[string]any{"content": "0"}}},
		})
	}))
	t.Cleanup(redirectTarget.Close)

	redirectSource := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, redirectTarget.URL, http.StatusFound)
	}))
	t.Cleanup(redirectSource.Close)

	res := runCheckForModel(context.Background(), MonitorProviderOpenAI, redirectSource.URL, "sk-openai", "gpt-test", nil)

	if res.Status != MonitorStatusError {
		t.Fatalf("redirected request should fail, got status=%s message=%q", res.Status, res.Message)
	}
	if redirectedHits != 0 {
		t.Fatalf("redirect target should not be reached, got %d hits", redirectedHits)
	}
}

func TestCallProvider_AnthropicExtractsTextAfterThinking(t *testing.T) {
	swapMonitorHTTPClient(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"content": []map[string]any{
				{"type": "thinking", "thinking": ""},
				{"type": "text", "text": "74"},
			},
		})
	}))
	t.Cleanup(server.Close)

	text, _, status, err := callProvider(
		context.Background(),
		MonitorProviderAnthropic,
		server.URL,
		"sk-anthropic",
		"claude-opus-4-7",
		"challenge",
		nil,
	)

	if err != nil {
		t.Fatalf("callProvider() error = %v", err)
	}
	if status != http.StatusOK {
		t.Fatalf("callProvider() status = %d, want %d", status, http.StatusOK)
	}
	if text != "74" {
		t.Fatalf("callProvider() text = %q, want %q", text, "74")
	}
}
