package service

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestIsResponsesEndpointSupportedByStatus(t *testing.T) {
	for _, tt := range []struct {
		name   string
		status int
		want   bool
	}{
		{name: "not found", status: http.StatusNotFound, want: false},
		{name: "method not allowed", status: http.StatusMethodNotAllowed, want: false},
		{name: "bad request means endpoint exists", status: http.StatusBadRequest, want: true},
		{name: "unauthorized means endpoint exists", status: http.StatusUnauthorized, want: true},
		{name: "server error keeps responses route", status: http.StatusBadGateway, want: true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, isResponsesEndpointSupportedByStatus(tt.status))
		})
	}
}

func TestBuildOpenAIResponsesURL(t *testing.T) {
	for _, tt := range []struct {
		base string
		want string
	}{
		{base: "https://compat.example", want: "https://compat.example/v1/responses"},
		{base: "https://compat.example/v1", want: "https://compat.example/v1/responses"},
		{base: "https://compat.example/v1/responses", want: "https://compat.example/v1/responses"},
	} {
		require.Equal(t, tt.want, buildOpenAIResponsesURL(tt.base))
	}
}
