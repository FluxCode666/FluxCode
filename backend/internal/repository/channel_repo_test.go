//go:build unit

package repository

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/lib/pq"
	"github.com/stretchr/testify/require"
)

// --- marshalModelMapping ---

func TestMarshalModelMapping(t *testing.T) {
	tests := []struct {
		name     string
		input    map[string]map[string]string
		wantJSON string // expected JSON output (exact match)
	}{
		{
			name:     "empty map",
			input:    map[string]map[string]string{},
			wantJSON: "{}",
		},
		{
			name:     "nil map",
			input:    nil,
			wantJSON: "{}",
		},
		{
			name: "populated map",
			input: map[string]map[string]string{
				"openai": {"gpt-4": "gpt-4-turbo"},
			},
		},
		{
			name: "nested values",
			input: map[string]map[string]string{
				"openai":    {"*": "gpt-5.4"},
				"anthropic": {"claude-old": "claude-new"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := marshalModelMapping(tt.input)
			require.NoError(t, err)

			if tt.wantJSON != "" {
				require.Equal(t, []byte(tt.wantJSON), result)
			} else {
				// round-trip: unmarshal and compare with input
				var parsed map[string]map[string]string
				require.NoError(t, json.Unmarshal(result, &parsed))
				require.Equal(t, tt.input, parsed)
			}
		})
	}
}

// --- unmarshalModelMapping ---

func TestUnmarshalModelMapping(t *testing.T) {
	tests := []struct {
		name    string
		input   []byte
		wantNil bool
		want    map[string]map[string]string
	}{
		{
			name:    "nil data",
			input:   nil,
			wantNil: true,
		},
		{
			name:    "empty data",
			input:   []byte{},
			wantNil: true,
		},
		{
			name:    "invalid JSON",
			input:   []byte("not-json"),
			wantNil: true,
		},
		{
			name:    "type error - number",
			input:   []byte("42"),
			wantNil: true,
		},
		{
			name:    "type error - array",
			input:   []byte("[1,2,3]"),
			wantNil: true,
		},
		{
			name:  "valid JSON",
			input: []byte(`{"openai":{"gpt-4":"gpt-4-turbo"},"anthropic":{"old":"new"}}`),
			want: map[string]map[string]string{
				"openai":    {"gpt-4": "gpt-4-turbo"},
				"anthropic": {"old": "new"},
			},
		},
		{
			name:  "empty object",
			input: []byte("{}"),
			want:  map[string]map[string]string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := unmarshalModelMapping(tt.input)
			if tt.wantNil {
				require.Nil(t, result)
			} else {
				require.NotNil(t, result)
				require.Equal(t, tt.want, result)
			}
		})
	}
}

// --- escapeLike ---

func TestEscapeLike(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "no special chars",
			input: "hello",
			want:  "hello",
		},
		{
			name:  "backslash",
			input: `a\b`,
			want:  `a\\b`,
		},
		{
			name:  "percent",
			input: "50%",
			want:  `50\%`,
		},
		{
			name:  "underscore",
			input: "a_b",
			want:  `a\_b`,
		},
		{
			name:  "all special chars",
			input: `a\b%c_d`,
			want:  `a\\b\%c\_d`,
		},
		{
			name:  "empty string",
			input: "",
			want:  "",
		},
		{
			name:  "consecutive special chars",
			input: "%_%",
			want:  `\%\_\%`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, escapeLike(tt.input))
		})
	}
}

// --- isUniqueViolation ---

func TestIsUniqueViolation(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{
			name: "unique violation code 23505",
			err:  &pq.Error{Code: "23505"},
			want: true,
		},
		{
			name: "different pq error code",
			err:  &pq.Error{Code: "23503"},
			want: false,
		},
		{
			name: "non-pq error",
			err:  errors.New("some generic error"),
			want: false,
		},
		{
			name: "typed nil pq.Error",
			err: func() error {
				var pqErr *pq.Error
				return pqErr
			}(),
			want: false,
		},
		{
			name: "bare nil",
			err:  nil,
			want: false,
		},
		{
			name: "wrapped pq error with 23505",
			err:  fmt.Errorf("wrapped: %w", &pq.Error{Code: "23505"}),
			want: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, isUniqueViolation(tt.err))
		})
	}
}

func TestChannelListOrderBy_AllowsDescendingIDSort(t *testing.T) {
	params := pagination.PaginationParams{
		SortBy:    "id",
		SortOrder: "desc",
	}

	require.Equal(t, "c.id DESC, c.id DESC", channelListOrderBy(params))
}

func TestChannelRepositoryCreateModelPricingPersistsCapabilities(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	repo := NewChannelRepository(db)
	pricing := service.ChannelModelPricing{
		ChannelID:    12,
		Platform:     "anthropic",
		Models:       []string{"claude-sonnet-4"},
		Capabilities: []string{"chat", "image"},
		BillingMode:  service.BillingModeToken,
	}

	mock.ExpectQuery(`INSERT INTO channel_model_pricing`).
		WithArgs(
			int64(12),
			"anthropic",
			[]byte(`["claude-sonnet-4"]`),
			service.BillingModeToken,
			pricing.InputPrice,
			pricing.OutputPrice,
			pricing.CacheWritePrice,
			pricing.CacheReadPrice,
			pricing.ImageOutputPrice,
			pricing.PerRequestPrice,
			[]byte(`["chat","image"]`),
		).
		WillReturnRows(sqlmock.NewRows([]string{"id", "created_at", "updated_at"}).
			AddRow(int64(99), time.Now(), time.Now()))

	err = repo.CreateModelPricing(context.Background(), &pricing)
	require.NoError(t, err)
	require.NoError(t, mock.ExpectationsWereMet())
	require.Equal(t, int64(99), pricing.ID)
}

func TestChannelRepositoryListModelPricingScansCapabilities(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	repo := NewChannelRepository(db)
	now := time.Now()

	mock.ExpectQuery(`SELECT id, channel_id, platform, models, capabilities, billing_mode`).
		WithArgs(int64(12)).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "channel_id", "platform", "models", "capabilities", "billing_mode",
			"input_price", "output_price", "cache_write_price", "cache_read_price",
			"image_output_price", "per_request_price", "created_at", "updated_at",
		}).
			AddRow(
				int64(99),
				int64(12),
				"anthropic",
				[]byte(`["claude-sonnet-4"]`),
				[]byte(`["chat","bad","image","chat"]`),
				service.BillingModeToken,
				nil, nil, nil, nil, nil, nil,
				now, now,
			).
			AddRow(
				int64(100),
				int64(12),
				"openai",
				[]byte(`["gpt-5.1"]`),
				[]byte(`{`),
				service.BillingModePerRequest,
				nil, nil, nil, nil, nil, nil,
				now, now,
			))

	mock.ExpectQuery(`SELECT id, pricing_id, min_tokens, max_tokens, tier_label,`).
		WithArgs(sqlmock.AnyArg()).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "pricing_id", "min_tokens", "max_tokens", "tier_label",
			"input_price", "output_price", "cache_write_price", "cache_read_price",
			"per_request_price", "sort_order", "created_at", "updated_at",
		}))

	pricing, err := repo.ListModelPricing(context.Background(), 12)
	require.NoError(t, err)
	require.Len(t, pricing, 2)
	require.Equal(t, []string{"chat", "image"}, pricing[0].Capabilities)
	require.Equal(t, []string{}, pricing[1].Capabilities)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestChannelRepositoryUpdateModelPricingPersistsCapabilities(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	repo := NewChannelRepository(db)
	inputPrice := 0.01
	outputPrice := 0.02
	cacheWritePrice := 0.003
	cacheReadPrice := 0.001
	imageOutputPrice := 0.05
	perRequestPrice := 0.6
	pricing := service.ChannelModelPricing{
		ID:               99,
		Platform:         "anthropic",
		Models:           []string{"claude-sonnet-4"},
		Capabilities:     []string{"chat", "bad", "image", "chat"},
		BillingMode:      service.BillingModeToken,
		InputPrice:       &inputPrice,
		OutputPrice:      &outputPrice,
		CacheWritePrice:  &cacheWritePrice,
		CacheReadPrice:   &cacheReadPrice,
		ImageOutputPrice: &imageOutputPrice,
		PerRequestPrice:  &perRequestPrice,
	}

	mock.ExpectExec(`UPDATE channel_model_pricing`).
		WithArgs(
			[]byte(`["claude-sonnet-4"]`),
			service.BillingModeToken,
			pricing.InputPrice,
			pricing.OutputPrice,
			pricing.CacheWritePrice,
			pricing.CacheReadPrice,
			pricing.ImageOutputPrice,
			pricing.PerRequestPrice,
			"anthropic",
			[]byte(`["chat","image"]`),
			int64(99),
		).
		WillReturnResult(sqlmock.NewResult(0, 1))

	err = repo.UpdateModelPricing(context.Background(), &pricing)
	require.NoError(t, err)
	require.NoError(t, mock.ExpectationsWereMet())
}
