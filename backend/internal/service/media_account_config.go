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
	Version  int                          `json:"version,omitempty"`
	Provider string                       `json:"provider,omitempty"`
	Models   map[string]MediaModelBinding `json:"models,omitempty"`
	// Legacy fields are retained for read compatibility only.
	Adapter         string                               `json:"adapter,omitempty"`
	NativeAsyncMode NativeAsyncMode                      `json:"native_async_mode,omitempty"`
	ModelOverrides  map[string]MediaAccountModelOverride `json:"model_overrides,omitempty"`
}

type MediaModelBinding struct {
	Enabled         bool                `json:"enabled"`
	UpstreamModel   string              `json:"upstream_model_id"`
	NativeAsyncMode NativeAsyncMode     `json:"async_mode"`
	RequestMapping  MediaRequestMapping `json:"request_mapping"`
}

// MediaAccountModelBinding names the account-scoped form explicitly while
// retaining MediaModelBinding for source compatibility with the foundation
// implementation.
type MediaAccountModelBinding = MediaModelBinding

func (binding MediaModelBinding) MarshalJSON() ([]byte, error) {
	type wireBinding struct {
		Enabled        bool                `json:"enabled"`
		UpstreamModel  string              `json:"upstream_model_id"`
		AsyncMode      string              `json:"async_mode"`
		RequestMapping MediaRequestMapping `json:"request_mapping"`
	}
	return json.Marshal(wireBinding{
		Enabled:        binding.Enabled,
		UpstreamModel:  binding.UpstreamModel,
		AsyncMode:      mediaBindingAsyncMode(binding.NativeAsyncMode),
		RequestMapping: binding.RequestMapping,
	})
}

type MediaAccountModelOverride struct {
	UpstreamModel   string          `json:"upstream_model,omitempty"`
	NativeAsyncMode NativeAsyncMode `json:"native_async_mode,omitempty"`
}

type ResolvedMediaAccountModel struct {
	Provider        string
	Adapter         string
	UpstreamModel   string
	NativeAsyncMode NativeAsyncMode
}

type mediaAccountConfigJSON struct {
	Version         json.RawMessage `json:"version"`
	Provider        json.RawMessage `json:"provider"`
	Models          json.RawMessage `json:"models"`
	Adapter         json.RawMessage `json:"adapter"`
	NativeAsyncMode json.RawMessage `json:"native_async_mode"`
	ModelOverrides  json.RawMessage `json:"model_overrides"`
}

type mediaModelBindingJSON struct {
	Enabled         json.RawMessage `json:"enabled"`
	UpstreamModel   json.RawMessage `json:"upstream_model_id"`
	NativeAsyncMode json.RawMessage `json:"async_mode"`
	RequestMapping  json.RawMessage `json:"request_mapping"`
}

type mediaAccountModelOverrideJSON struct {
	UpstreamModel   json.RawMessage `json:"upstream_model"`
	NativeAsyncMode json.RawMessage `json:"native_async_mode"`
}

func NormalizeMediaAccountConfig(config MediaAccountConfig) (MediaAccountConfig, error) {
	if config.Version > 0 {
		if config.Version != 1 {
			return MediaAccountConfig{}, fmt.Errorf("%w: unsupported version", ErrInvalidMediaAccountConfig)
		}
		config.Provider = strings.TrimSpace(config.Provider)
		if config.Provider == "" {
			return MediaAccountConfig{}, fmt.Errorf("%w: provider is empty", ErrInvalidMediaAccountConfig)
		}
		normalized := make(map[string]MediaModelBinding, len(config.Models))
		for model, binding := range config.Models {
			model = strings.TrimSpace(model)
			if model == "" || normalized[model].UpstreamModel != "" {
				return MediaAccountConfig{}, fmt.Errorf("%w: invalid or duplicate model", ErrInvalidMediaAccountConfig)
			}
			binding.UpstreamModel = strings.TrimSpace(binding.UpstreamModel)
			if binding.UpstreamModel == "" {
				return MediaAccountConfig{}, fmt.Errorf("%w: upstream model is empty", ErrInvalidMediaAccountConfig)
			}
			mode, err := normalizeMediaAccountNativeAsyncMode(binding.NativeAsyncMode, true)
			if err != nil {
				return MediaAccountConfig{}, err
			}
			binding.NativeAsyncMode = mode
			normalized[model] = binding
		}
		config.Models = normalized
		return config, nil
	}
	// Legacy configuration is normalized but remains marked as legacy in memory;
	// persistence converts it to version 1 in mediaAccountConfigMap.
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

// New account bindings intentionally expose only whether the provider uses a
// native asynchronous protocol. The legacy optional/required distinction stays
// available only while reading old adapter configurations.
func normalizeMediaBindingAsyncMode(mode string) (NativeAsyncMode, error) {
	switch strings.ToLower(strings.TrimSpace(mode)) {
	case "native":
		return NativeAsyncRequired, nil
	case "unsupported":
		return NativeAsyncUnsupported, nil
	default:
		return "", ErrInvalidNativeAsyncMode
	}
}

func mediaBindingAsyncMode(mode NativeAsyncMode) string {
	if mode == NativeAsyncUnsupported {
		return "unsupported"
	}
	return "native"
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
		if errors.Is(err, ErrInvalidNativeAsyncMode) {
			return MediaAccountConfig{}, true, fmt.Errorf("%w: %w", ErrInvalidMediaAccountConfig, err)
		}
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

	if len(raw.Version) > 0 {
		var version int
		if err := json.Unmarshal(raw.Version, &version); err != nil || version != 1 {
			return MediaAccountConfig{}, ErrInvalidMediaAccountConfig
		}
		provider, err := decodeMediaAccountString(raw.Provider, false)
		if err != nil {
			return MediaAccountConfig{}, err
		}
		if len(raw.Models) == 0 || isJSONNull(raw.Models) || !isJSONObject(raw.Models) {
			return MediaAccountConfig{}, ErrInvalidMediaAccountConfig
		}
		var models map[string]json.RawMessage
		if err := json.Unmarshal(raw.Models, &models); err != nil {
			return MediaAccountConfig{}, ErrInvalidMediaAccountConfig
		}
		config := MediaAccountConfig{Version: 1, Provider: provider, Models: make(map[string]MediaModelBinding, len(models))}
		for model, encodedBinding := range models {
			if !isJSONObject(encodedBinding) {
				return MediaAccountConfig{}, ErrInvalidMediaAccountConfig
			}
			var rawBinding mediaModelBindingJSON
			if err := decodeMediaAccountJSON(encodedBinding, &rawBinding); err != nil {
				return MediaAccountConfig{}, err
			}
			upstream, err := decodeMediaAccountString(rawBinding.UpstreamModel, false)
			if err != nil {
				return MediaAccountConfig{}, err
			}
			mode, err := decodeMediaAccountString(rawBinding.NativeAsyncMode, false)
			if err != nil {
				return MediaAccountConfig{}, err
			}
			if len(rawBinding.Enabled) == 0 {
				return MediaAccountConfig{}, ErrInvalidMediaAccountConfig
			}
			var enabled bool
			if err := json.Unmarshal(rawBinding.Enabled, &enabled); err != nil {
				return MediaAccountConfig{}, ErrInvalidMediaAccountConfig
			}
			if len(rawBinding.RequestMapping) > 0 && (isJSONNull(rawBinding.RequestMapping) || !isJSONObject(rawBinding.RequestMapping)) {
				return MediaAccountConfig{}, ErrInvalidMediaAccountConfig
			}
			var mapping MediaRequestMapping
			if len(rawBinding.RequestMapping) > 0 {
				if err := json.Unmarshal(rawBinding.RequestMapping, &mapping); err != nil {
					return MediaAccountConfig{}, ErrInvalidMediaAccountConfig
				}
			}
			nativeMode, err := normalizeMediaBindingAsyncMode(mode)
			if err != nil {
				return MediaAccountConfig{}, err
			}
			config.Models[model] = MediaModelBinding{Enabled: enabled, UpstreamModel: upstream, NativeAsyncMode: nativeMode, RequestMapping: mapping}
		}
		return config, nil
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
	if config.Version == 1 || len(config.Models) > 0 {
		models := make(map[string]any, len(config.Models))
		for model, binding := range config.Models {
			models[model] = map[string]any{"enabled": binding.Enabled, "upstream_model_id": binding.UpstreamModel, "async_mode": mediaBindingAsyncMode(binding.NativeAsyncMode), "request_mapping": binding.RequestMapping}
		}
		return map[string]any{"version": 1, "provider": config.Provider, "models": models}
	}
	models := make(map[string]any, len(config.ModelOverrides))
	for model, override := range config.ModelOverrides {
		upstream := override.UpstreamModel
		if upstream == "" {
			upstream = model
		}
		mode := override.NativeAsyncMode
		if mode == "" {
			mode = config.NativeAsyncMode
		}
		models[model] = map[string]any{
			"enabled":           true,
			"upstream_model_id": upstream,
			"async_mode":        mediaBindingAsyncMode(mode),
			"request_mapping":   MediaRequestMapping{},
		}
	}
	return map[string]any{"version": 1, "provider": config.Adapter, "models": models}
}
