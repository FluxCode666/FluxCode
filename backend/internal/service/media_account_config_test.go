package service

import (
	"encoding/json"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestAccountResolveMediaConfigUsesModelOverride(t *testing.T) {
	account := &Account{Extra: map[string]any{
		"media_config": map[string]any{
			"adapter":           "gemini",
			"native_async_mode": "optional",
			"model_overrides": map[string]any{
				"veo-3.1": map[string]any{
					"upstream_model":    "veo-3.1-generate",
					"native_async_mode": "required",
				},
			},
		},
	}}

	resolved := account.ResolveMediaModel("veo-3.1")
	require.Equal(t, "gemini", resolved.Adapter)
	require.Equal(t, "veo-3.1-generate", resolved.UpstreamModel)
	require.Equal(t, NativeAsyncRequired, resolved.NativeAsyncMode)
}

func TestAccountResolveMediaConfigInheritsMissingOverrideFields(t *testing.T) {
	account := &Account{Extra: map[string]any{
		"media_config": map[string]any{
			"adapter":           "gemini",
			"native_async_mode": "optional",
			"model_overrides": map[string]any{
				"veo-upstream": map[string]any{"upstream_model": "  veo-provider  "},
				"veo-mode":     map[string]any{"native_async_mode": "required"},
				"veo-empty": map[string]any{
					"upstream_model": "  ", "native_async_mode": "  ",
				},
			},
		},
	}}

	upstream := account.ResolveMediaModel("  veo-upstream  ")
	require.Equal(t, "veo-provider", upstream.UpstreamModel)
	require.Equal(t, NativeAsyncOptional, upstream.NativeAsyncMode)

	mode := account.ResolveMediaModel("  veo-mode  ")
	require.Equal(t, "veo-mode", mode.UpstreamModel)
	require.Equal(t, NativeAsyncRequired, mode.NativeAsyncMode)

	missing := account.ResolveMediaModel("  VEO-MODE  ")
	require.Equal(t, "gemini", missing.Adapter)
	require.Equal(t, "VEO-MODE", missing.UpstreamModel)
	require.Equal(t, NativeAsyncOptional, missing.NativeAsyncMode)

	empty := account.ResolveMediaModel(" veo-empty ")
	require.Equal(t, "veo-empty", empty.UpstreamModel)
	require.Equal(t, NativeAsyncOptional, empty.NativeAsyncMode)
}

func TestAccountResolveMediaConfigFallsBackForUnconfiguredOrMalformedExtra(t *testing.T) {
	tests := []struct {
		name    string
		account *Account
	}{
		{name: "nil account", account: nil},
		{name: "nil extra", account: &Account{}},
		{name: "missing media config", account: &Account{Extra: map[string]any{"other": true}}},
		{name: "malformed media config", account: &Account{Extra: map[string]any{"media_config": "invalid"}}},
		{name: "invalid mode", account: &Account{Extra: map[string]any{"media_config": map[string]any{
			"adapter": "gemini", "native_async_mode": "sometimes",
		}}}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resolved := tt.account.ResolveMediaModel("  veo-3.1  ")
			require.Empty(t, resolved.Adapter)
			require.Equal(t, "veo-3.1", resolved.UpstreamModel)
			require.Equal(t, NativeAsyncUnsupported, resolved.NativeAsyncMode)
		})
	}
}

func TestNormalizeMediaAccountConfigNormalizesValues(t *testing.T) {
	normalized, err := NormalizeMediaAccountConfig(MediaAccountConfig{
		Adapter:         "  GeMiNi  ",
		NativeAsyncMode: "  OPTIONAL  ",
		ModelOverrides: map[string]MediaAccountModelOverride{
			"  Veo-3.1  ": {
				UpstreamModel:   "  provider-model  ",
				NativeAsyncMode: "  REQUIRED ",
			},
		},
	})
	require.NoError(t, err)
	require.Equal(t, "gemini", normalized.Adapter)
	require.Equal(t, NativeAsyncOptional, normalized.NativeAsyncMode)
	require.Equal(t, map[string]MediaAccountModelOverride{
		"Veo-3.1": {
			UpstreamModel:   "provider-model",
			NativeAsyncMode: NativeAsyncRequired,
		},
	}, normalized.ModelOverrides)
}

func TestNormalizeMediaAccountConfigDefaultsMissingModesToUnsupported(t *testing.T) {
	normalized, err := NormalizeMediaAccountConfig(MediaAccountConfig{
		Adapter: "gemini",
		ModelOverrides: map[string]MediaAccountModelOverride{
			"veo-3.1": {},
		},
	})
	require.NoError(t, err)
	require.Equal(t, NativeAsyncUnsupported, normalized.NativeAsyncMode)
	require.Empty(t, normalized.ModelOverrides["veo-3.1"].NativeAsyncMode)

	resolved := (&Account{Extra: map[string]any{"media_config": normalized}}).ResolveMediaModel("veo-3.1")
	require.Equal(t, NativeAsyncUnsupported, resolved.NativeAsyncMode)
}

func TestNormalizeMediaAccountConfigRejectsUnknownMode(t *testing.T) {
	_, err := NormalizeMediaAccountConfig(MediaAccountConfig{Adapter: "gemini", NativeAsyncMode: "sometimes"})
	require.ErrorIs(t, err, ErrInvalidNativeAsyncMode)
	_, err = NormalizeMediaAccountConfig(MediaAccountConfig{
		Adapter: "gemini",
		ModelOverrides: map[string]MediaAccountModelOverride{
			"veo-3.1": {NativeAsyncMode: "sometimes"},
		},
	})
	require.ErrorIs(t, err, ErrInvalidNativeAsyncMode)
}

func TestNormalizeMediaAccountConfigRejectsInvalidIdentityFields(t *testing.T) {
	tests := []struct {
		name   string
		config MediaAccountConfig
	}{
		{name: "empty adapter", config: MediaAccountConfig{Adapter: "  "}},
		{name: "empty model key", config: MediaAccountConfig{Adapter: "gemini", ModelOverrides: map[string]MediaAccountModelOverride{"  ": {}}}},
		{name: "duplicate trimmed model key", config: MediaAccountConfig{Adapter: "gemini", ModelOverrides: map[string]MediaAccountModelOverride{
			"veo-3.1":   {},
			" veo-3.1 ": {},
		}}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := NormalizeMediaAccountConfig(tt.config)
			require.True(t, errors.Is(err, ErrInvalidMediaAccountConfig), err)
		})
	}
}

func TestNormalizeMediaAccountConfigExtraRejectsExplicitNull(t *testing.T) {
	typedNil := (*MediaAccountConfig)(nil)
	decodedNullMode := mustDecodeMediaAccountExtra(t, `{
		"media_config":{"adapter":"gemini","native_async_mode":null}
	}`)

	tests := []struct {
		name  string
		extra map[string]any
	}{
		{name: "top level interface nil", extra: map[string]any{"media_config": nil}},
		{name: "top level typed nil", extra: map[string]any{"media_config": typedNil}},
		{name: "top level raw message null", extra: map[string]any{"media_config": json.RawMessage(`null`)}},
		{name: "adapter null", extra: map[string]any{"media_config": map[string]any{
			"adapter": nil,
		}}},
		{name: "native async mode null", extra: map[string]any{"media_config": map[string]any{
			"adapter": "gemini", "native_async_mode": nil,
		}}},
		{name: "model overrides null", extra: map[string]any{"media_config": map[string]any{
			"adapter": "gemini", "model_overrides": nil,
		}}},
		{name: "override value null", extra: map[string]any{"media_config": map[string]any{
			"adapter": "gemini", "model_overrides": map[string]any{"veo": nil},
		}}},
		{name: "upstream model null", extra: map[string]any{"media_config": map[string]any{
			"adapter": "gemini", "model_overrides": map[string]any{
				"veo": map[string]any{"upstream_model": nil},
			},
		}}},
		{name: "override native async mode null", extra: map[string]any{"media_config": map[string]any{
			"adapter": "gemini", "model_overrides": map[string]any{
				"veo": map[string]any{"native_async_mode": nil},
			},
		}}},
		{name: "raw message nested null", extra: map[string]any{
			"media_config": json.RawMessage(`{"adapter":"gemini","model_overrides":{"veo":{"upstream_model":null}}}`),
		}},
		{name: "json round trip null", extra: decodedNullMode},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := normalizeMediaAccountConfigInExtra(tt.extra)
			require.ErrorIs(t, err, ErrInvalidMediaAccountConfig)
		})
	}
}

func TestNormalizeMediaAccountConfigExtraRejectsWrongJSONTypes(t *testing.T) {
	tests := []struct {
		name string
		raw  any
	}{
		{name: "model overrides array", raw: map[string]any{"adapter": "gemini", "model_overrides": []any{}}},
		{name: "override string", raw: map[string]any{"adapter": "gemini", "model_overrides": map[string]any{"veo": "bad"}}},
		{name: "upstream model number", raw: map[string]any{"adapter": "gemini", "model_overrides": map[string]any{
			"veo": map[string]any{"upstream_model": 42},
		}}},
		{name: "override mode boolean", raw: map[string]any{"adapter": "gemini", "model_overrides": map[string]any{
			"veo": map[string]any{"native_async_mode": true},
		}}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := normalizeMediaAccountConfigInExtra(map[string]any{"media_config": tt.raw})
			require.ErrorIs(t, err, ErrInvalidMediaAccountConfig)
		})
	}
}

func TestMediaAccountConfigVersionOneBindings(t *testing.T) {
	extra := map[string]any{"media_config": map[string]any{
		"version": 1, "provider": " xai ",
		"models": map[string]any{
			" image-one ": map[string]any{
				"enabled": true, "upstream_model_id": " upstream-image ", "async_mode": "native",
				"request_mapping": map[string]any{"rules": []any{}},
			},
			"disabled": map[string]any{"enabled": false, "upstream_model_id": "up-disabled", "async_mode": "unsupported"},
		},
	}}

	require.NoError(t, normalizeMediaAccountConfigInExtra(extra))
	stored := extra["media_config"].(map[string]any)
	require.Equal(t, 1, stored["version"])
	require.Equal(t, "xai", stored["provider"])
	account := &Account{Extra: extra}
	binding, ok := account.ResolveMediaModelBinding("image-one")
	require.True(t, ok)
	require.Equal(t, "upstream-image", binding.UpstreamModel)
	require.Equal(t, NativeAsyncRequired, binding.NativeAsyncMode)
	require.True(t, account.HasMediaModel("image-one"))
	require.False(t, account.HasMediaModel("disabled"))
	require.False(t, account.HasMediaModel("missing"))

	encoded, err := json.Marshal(stored)
	require.NoError(t, err)
	var roundTrip map[string]any
	require.NoError(t, json.Unmarshal(encoded, &roundTrip))
	roundTripExtra := map[string]any{mediaAccountConfigExtraKey: roundTrip}
	require.NoError(t, normalizeMediaAccountConfigInExtra(roundTripExtra))
	roundTripBinding, ok := (&Account{Extra: roundTripExtra}).ResolveMediaModelBinding("image-one")
	require.True(t, ok)
	require.JSONEq(t, `{"rules":[]}`, string(mustMarshalMediaRequestMapping(t, roundTripBinding.RequestMapping)))
}

func TestMediaAccountConfigVersionOneRejectsInvalidBindings(t *testing.T) {
	validBinding := func() map[string]any {
		return map[string]any{"enabled": true, "upstream_model_id": "upstream", "async_mode": "native"}
	}
	tests := []struct {
		name   string
		models map[string]any
	}{
		{name: "empty model", models: map[string]any{" ": validBinding()}},
		{name: "empty upstream", models: map[string]any{"model": map[string]any{"enabled": true, "upstream_model_id": " ", "async_mode": "native"}}},
		{name: "invalid async mode", models: map[string]any{"model": map[string]any{"enabled": true, "upstream_model_id": "up", "async_mode": "optional"}}},
		{name: "duplicate normalized model", models: map[string]any{"model": validBinding(), " model ": validBinding()}},
		{name: "unknown binding field", models: map[string]any{"model": map[string]any{"enabled": true, "upstream_model_id": "up", "async_mode": "native", "unexpected": true}}},
		{name: "request mapping must be object", models: map[string]any{"model": map[string]any{"enabled": true, "upstream_model_id": "up", "async_mode": "native", "request_mapping": []any{}}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := normalizeMediaAccountConfigInExtra(map[string]any{"media_config": map[string]any{
				"version": 1, "provider": "xai", "models": tt.models,
			}})
			require.Error(t, err)
		})
	}
}

func TestMediaAccountConfigVersionOneRejectsUnknownTopLevelField(t *testing.T) {
	err := normalizeMediaAccountConfigInExtra(map[string]any{"media_config": map[string]any{
		"version": 1, "provider": "xai", "models": map[string]any{}, "unexpected": true,
	}})
	require.ErrorIs(t, err, ErrInvalidMediaAccountConfig)
}

func mustDecodeMediaAccountExtra(t *testing.T, raw string) map[string]any {
	t.Helper()
	var extra map[string]any
	require.NoError(t, json.Unmarshal([]byte(raw), &extra))
	return extra
}

func mustMarshalMediaRequestMapping(t *testing.T, mapping MediaRequestMapping) []byte {
	t.Helper()
	encoded, err := json.Marshal(mapping)
	require.NoError(t, err)
	return encoded
}
