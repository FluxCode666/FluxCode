package service

import (
	"errors"
	"net/http"
	"net/url"
	"path"
	"strings"
)

type EffectiveProviderConnection struct {
	URL      string
	Headers  map[string]string
	AuthType string
}

var forbiddenProviderHeaders = map[string]struct{}{
	"authorization":       {},
	"proxy-authorization": {},
	"host":                {},
	"cookie":              {},
	"set-cookie":          {},
	"content-length":      {},
	"transfer-encoding":   {},
	"connection":          {},
	"keep-alive":          {},
	"proxy-authenticate":  {},
	"te":                  {},
	"trailer":             {},
	"upgrade":             {},
}

func IsAllowedProviderHeader(name string) bool {
	name = strings.ToLower(strings.TrimSpace(name))
	if name == "" {
		return false
	}
	if _, blocked := forbiddenProviderHeaders[name]; blocked {
		return false
	}
	return http.CanonicalHeaderKey(name) != ""
}

func sanitizeProviderHeaders(base, override map[string]string) map[string]string {
	out := make(map[string]string, len(base)+len(override))
	for _, source := range []map[string]string{base, override} {
		for name, value := range source {
			if !IsAllowedProviderHeader(name) {
				continue
			}
			out[http.CanonicalHeaderKey(name)] = strings.TrimSpace(value)
		}
	}
	return out
}

func (e ProviderProtocolEndpoint) EffectiveConfig(base ProviderConnectionConfig) (EffectiveProviderConnection, error) {
	if !e.Protocol.IsValid() {
		return EffectiveProviderConnection{}, errors.New("invalid provider protocol")
	}
	baseURL := strings.TrimSpace(base.BaseURL)
	if strings.TrimSpace(e.BaseURL) != "" {
		baseURL = strings.TrimSpace(e.BaseURL)
	}
	u, err := url.Parse(baseURL)
	if err != nil || u.Scheme == "" || u.Host == "" {
		return EffectiveProviderConnection{}, errors.New("provider base URL must be absolute")
	}
	if u.Scheme != "https" && u.Scheme != "http" {
		return EffectiveProviderConnection{}, errors.New("provider URL must use http or https")
	}
	endpointPath := strings.TrimSpace(e.Path)
	if endpointPath == "" {
		endpointPath = e.Protocol.DefaultPath()
	}
	if !strings.HasPrefix(endpointPath, "/") || strings.HasPrefix(endpointPath, "//") {
		return EffectiveProviderConnection{}, errors.New("provider endpoint path must be absolute and host-relative")
	}
	u.Path = path.Join(strings.TrimSuffix(u.Path, "/"), endpointPath)
	u.RawPath = ""
	u.RawQuery = ""
	u.Fragment = ""
	authType := strings.TrimSpace(base.AuthType)
	if strings.TrimSpace(e.AuthType) != "" {
		authType = strings.TrimSpace(e.AuthType)
	}
	return EffectiveProviderConnection{
		URL:      u.String(),
		Headers:  sanitizeProviderHeaders(base.Headers, e.Headers),
		AuthType: authType,
	}, nil
}
