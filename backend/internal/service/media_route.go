package service

import (
	"bytes"
	"encoding/json"
)

// MediaRequestMapping is a route-level request mapping snapshot. Mapping rules
// are added by the declarative request-mapping stage.
type MediaRequestMapping struct {
	raw json.RawMessage
}

// UnmarshalJSON preserves the declaration until Task 4 validates and applies
// individual mapping rules. This keeps account configuration round-trippable.
func (m *MediaRequestMapping) UnmarshalJSON(data []byte) error {
	if !json.Valid(data) {
		return &json.SyntaxError{}
	}
	m.raw = append(m.raw[:0], bytes.TrimSpace(data)...)
	return nil
}

func (m MediaRequestMapping) MarshalJSON() ([]byte, error) {
	if len(m.raw) == 0 {
		return []byte("{}"), nil
	}
	return append([]byte(nil), m.raw...), nil
}

type MediaRouteRequest struct {
	GroupID        int64
	RequestedModel string
	Operation      MediaOperation
	Capability     MediaType
	SessionHash    string
	ClientAsync    bool
}

type MediaRouteTarget struct {
	AccountID       int64
	PublicModelID   string
	UpstreamModelID string
	Vendor          string
	Adapter         string
	NativeAsyncMode NativeAsyncMode
	RequestMapping  MediaRequestMapping
}
