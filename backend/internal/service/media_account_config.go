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
	var config MediaAccountConfig
	decoder := json.NewDecoder(bytes.NewReader(encoded))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&config); err != nil {
		return MediaAccountConfig{}, true, fmt.Errorf("%w: invalid media_config shape", ErrInvalidMediaAccountConfig)
	}
	config, err = NormalizeMediaAccountConfig(config)
	if err != nil {
		return MediaAccountConfig{}, true, err
	}
	return config, true, nil
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
