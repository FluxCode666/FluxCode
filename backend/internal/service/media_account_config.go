package service

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

const mediaAccountConfigExtraKey = "media_config"

var (
	ErrInvalidMediaAccountConfig = errors.New("invalid media account config")
	ErrInvalidNativeAsyncMode    = errors.New("invalid native async mode")
)

type MediaAccountConfig struct {
	Adapter         string                               `json:"adapter"`
	NativeAsyncMode NativeAsyncMode                      `json:"native_async_mode"`
	ModelOverrides  map[string]MediaAccountModelOverride `json:"model_overrides,omitempty"`
}

type MediaAccountModelOverride struct {
	UpstreamModel   string          `json:"upstream_model,omitempty"`
	NativeAsyncMode NativeAsyncMode `json:"native_async_mode,omitempty"`
}

type ResolvedMediaAccountModel struct {
	Adapter         string
	UpstreamModel   string
	NativeAsyncMode NativeAsyncMode
}

type mediaAccountConfigJSON struct {
	Adapter         json.RawMessage `json:"adapter"`
	NativeAsyncMode json.RawMessage `json:"native_async_mode"`
	ModelOverrides  json.RawMessage `json:"model_overrides"`
}

type mediaAccountModelOverrideJSON struct {
	UpstreamModel   json.RawMessage `json:"upstream_model"`
	NativeAsyncMode json.RawMessage `json:"native_async_mode"`
}

func NormalizeMediaAccountConfig(config MediaAccountConfig) (MediaAccountConfig, error) {
	config.Adapter = strings.ToLower(strings.TrimSpace(config.Adapter))
	if config.Adapter == "" {
		return MediaAccountConfig{}, fmt.Errorf("%w: adapter is empty", ErrInvalidMediaAccountConfig)
	}

	mode, err := normalizeMediaAccountNativeAsyncMode(config.NativeAsyncMode, true)
	if err != nil {
		return MediaAccountConfig{}, err
	}
	config.NativeAsyncMode = mode

	if config.ModelOverrides == nil {
		return config, nil
	}
	normalizedOverrides := make(map[string]MediaAccountModelOverride, len(config.ModelOverrides))
	for model, override := range config.ModelOverrides {
		normalizedModel := strings.TrimSpace(model)
		if normalizedModel == "" {
			return MediaAccountConfig{}, fmt.Errorf("%w: model override key is empty", ErrInvalidMediaAccountConfig)
		}
		if _, exists := normalizedOverrides[normalizedModel]; exists {
			return MediaAccountConfig{}, fmt.Errorf("%w: duplicate normalized model override key", ErrInvalidMediaAccountConfig)
		}

		override.UpstreamModel = strings.TrimSpace(override.UpstreamModel)
		override.NativeAsyncMode, err = normalizeMediaAccountNativeAsyncMode(override.NativeAsyncMode, false)
		if err != nil {
			return MediaAccountConfig{}, err
		}
		normalizedOverrides[normalizedModel] = override
	}
	config.ModelOverrides = normalizedOverrides
	return config, nil
}

func normalizeMediaAccountNativeAsyncMode(mode NativeAsyncMode, defaultUnsupported bool) (NativeAsyncMode, error) {
	normalized := NativeAsyncMode(strings.ToLower(strings.TrimSpace(string(mode))))
	if normalized == "" {
		if defaultUnsupported {
			return NativeAsyncUnsupported, nil
		}
		return "", nil
	}
	switch normalized {
	case NativeAsyncUnsupported, NativeAsyncOptional, NativeAsyncRequired:
		return normalized, nil
	default:
		return "", ErrInvalidNativeAsyncMode
	}
}

func mediaAccountConfigFromExtra(extra map[string]any) (MediaAccountConfig, bool, error) {
	if extra == nil {
		return MediaAccountConfig{}, false, nil
	}
	raw, exists := extra[mediaAccountConfigExtraKey]
	if !exists {
		return MediaAccountConfig{}, false, nil
	}

	encoded, err := json.Marshal(raw)
	if err != nil {
		return MediaAccountConfig{}, true, fmt.Errorf("%w: media_config cannot be encoded", ErrInvalidMediaAccountConfig)
	}
	config, err := decodeMediaAccountConfig(encoded)
	if err != nil {
		return MediaAccountConfig{}, true, fmt.Errorf("%w: invalid media_config shape", ErrInvalidMediaAccountConfig)
	}
	config, err = NormalizeMediaAccountConfig(config)
	if err != nil {
		return MediaAccountConfig{}, true, err
	}
	return config, true, nil
}

func decodeMediaAccountConfig(encoded []byte) (MediaAccountConfig, error) {
	if !isJSONObject(encoded) {
		return MediaAccountConfig{}, ErrInvalidMediaAccountConfig
	}
	var raw mediaAccountConfigJSON
	if err := decodeMediaAccountJSON(encoded, &raw); err != nil {
		return MediaAccountConfig{}, err
	}

	adapter, err := decodeMediaAccountString(raw.Adapter, false)
	if err != nil {
		return MediaAccountConfig{}, err
	}
	mode, err := decodeMediaAccountString(raw.NativeAsyncMode, true)
	if err != nil {
		return MediaAccountConfig{}, err
	}
	config := MediaAccountConfig{Adapter: adapter, NativeAsyncMode: NativeAsyncMode(mode)}
	if len(raw.ModelOverrides) == 0 {
		return config, nil
	}
	if isJSONNull(raw.ModelOverrides) || !isJSONObject(raw.ModelOverrides) {
		return MediaAccountConfig{}, ErrInvalidMediaAccountConfig
	}

	var overrides map[string]json.RawMessage
	if err := json.Unmarshal(raw.ModelOverrides, &overrides); err != nil {
		return MediaAccountConfig{}, err
	}
	config.ModelOverrides = make(map[string]MediaAccountModelOverride, len(overrides))
	for model, rawOverride := range overrides {
		override, err := decodeMediaAccountModelOverride(rawOverride)
		if err != nil {
			return MediaAccountConfig{}, err
		}
		config.ModelOverrides[model] = override
	}
	return config, nil
}

func decodeMediaAccountModelOverride(encoded []byte) (MediaAccountModelOverride, error) {
	if !isJSONObject(encoded) {
		return MediaAccountModelOverride{}, ErrInvalidMediaAccountConfig
	}
	var raw mediaAccountModelOverrideJSON
	if err := decodeMediaAccountJSON(encoded, &raw); err != nil {
		return MediaAccountModelOverride{}, err
	}
	upstreamModel, err := decodeMediaAccountString(raw.UpstreamModel, true)
	if err != nil {
		return MediaAccountModelOverride{}, err
	}
	mode, err := decodeMediaAccountString(raw.NativeAsyncMode, true)
	if err != nil {
		return MediaAccountModelOverride{}, err
	}
	return MediaAccountModelOverride{
		UpstreamModel:   upstreamModel,
		NativeAsyncMode: NativeAsyncMode(mode),
	}, nil
}

func decodeMediaAccountString(raw json.RawMessage, optional bool) (string, error) {
	if len(raw) == 0 {
		if optional {
			return "", nil
		}
		return "", ErrInvalidMediaAccountConfig
	}
	if isJSONNull(raw) {
		return "", ErrInvalidMediaAccountConfig
	}
	var value string
	if err := json.Unmarshal(raw, &value); err != nil {
		return "", ErrInvalidMediaAccountConfig
	}
	return value, nil
}

func decodeMediaAccountJSON(encoded []byte, target any) error {
	decoder := json.NewDecoder(bytes.NewReader(encoded))
	decoder.DisallowUnknownFields()
	return decoder.Decode(target)
}

func isJSONObject(encoded []byte) bool {
	trimmed := bytes.TrimSpace(encoded)
	return len(trimmed) > 0 && trimmed[0] == '{'
}

func isJSONNull(encoded []byte) bool {
	return bytes.Equal(bytes.TrimSpace(encoded), []byte("null"))
}

func normalizeMediaAccountConfigInExtra(extra map[string]any) error {
	config, configured, err := mediaAccountConfigFromExtra(extra)
	if err != nil || !configured {
		return err
	}
	extra[mediaAccountConfigExtraKey] = mediaAccountConfigMap(config)
	return nil
}

func mediaAccountConfigMap(config MediaAccountConfig) map[string]any {
	result := map[string]any{
		"adapter":           config.Adapter,
		"native_async_mode": string(config.NativeAsyncMode),
	}
	if len(config.ModelOverrides) == 0 {
		return result
	}

	overrides := make(map[string]any, len(config.ModelOverrides))
	for model, override := range config.ModelOverrides {
		value := make(map[string]any, 2)
		if override.UpstreamModel != "" {
			value["upstream_model"] = override.UpstreamModel
		}
		if override.NativeAsyncMode != "" {
			value["native_async_mode"] = string(override.NativeAsyncMode)
		}
		overrides[model] = value
	}
	result["model_overrides"] = overrides
	return result
}
