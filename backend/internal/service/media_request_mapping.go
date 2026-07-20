package service

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
)

var (
	ErrInvalidMediaRequestMapping = errors.New("invalid media request mapping")
	ErrMediaMappingValueMissing   = errors.New("media mapping value is missing")
	ErrMediaMappingEnumMiss       = errors.New("media mapping enum value is not mapped")
	ErrMediaMappingCastFailed     = errors.New("media mapping cast failed")
	ErrMediaMappingTargetConflict = errors.New("media mapping target conflict")
)

// MediaMappingRule is a declarative, data-only request transformation. Script
// expressions are deliberately not part of this format.
type MediaMappingRule struct {
	Source    string            `json:"source,omitempty"`
	Target    string            `json:"target"`
	Operation string            `json:"operation"`
	Value     any               `json:"value,omitempty"`
	Values    map[string]string `json:"values,omitempty"`
	Cast      string            `json:"cast,omitempty"`
}

// MediaRequestMapping is persisted with an account binding and copied into the
// chosen route snapshot. It contains no executable code.
type MediaRequestMapping struct {
	Rules []MediaMappingRule `json:"rules,omitempty"`
}

func (m MediaRequestMapping) MarshalJSON() ([]byte, error) {
	if m.Rules == nil {
		return []byte("{}"), nil
	}
	return json.Marshal(struct {
		Rules []MediaMappingRule `json:"rules"`
	}{Rules: m.Rules})
}

func (m *MediaRequestMapping) UnmarshalJSON(data []byte) error {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	var decoded struct {
		Rules []MediaMappingRule `json:"rules,omitempty"`
	}
	if err := decoder.Decode(&decoded); err != nil {
		return err
	}
	if err := ensureSingleMediaMappingJSONValue(decoder); err != nil {
		return err
	}
	m.Rules = decoded.Rules
	return nil
}

func ensureSingleMediaMappingJSONValue(decoder *json.Decoder) error {
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		if err == nil {
			return errors.New("multiple JSON values")
		}
		return err
	}
	return nil
}

func (m MediaRequestMapping) Validate() error {
	targets := make(map[string]struct{}, len(m.Rules))
	for index := range m.Rules {
		rule := m.Rules[index]
		op := strings.ToLower(strings.TrimSpace(rule.Operation))
		if !isMediaMappingOperation(op) {
			return fmt.Errorf("%w: rule %d has unsupported op %q", ErrInvalidMediaRequestMapping, index, op)
		}
		if err := validateMediaMappingPath(rule.Target); err != nil {
			return fmt.Errorf("%w: rule %d target: %w", ErrInvalidMediaRequestMapping, index, err)
		}
		if _, exists := targets[rule.Target]; exists {
			return fmt.Errorf("%w: %s", ErrMediaMappingTargetConflict, rule.Target)
		}
		targets[rule.Target] = struct{}{}
		switch op {
		case "rename", "copy", "enum", "cast":
			if err := validateMediaMappingPath(rule.Source); err != nil {
				return fmt.Errorf("%w: rule %d source: %w", ErrInvalidMediaRequestMapping, index, err)
			}
		}
		if op == "rename" && rule.Source == rule.Target {
			return fmt.Errorf("%w: rule %d rename source and target match", ErrInvalidMediaRequestMapping, index)
		}
		switch op {
		case "enum":
			if len(rule.Values) == 0 {
				return fmt.Errorf("%w: rule %d enum values are empty", ErrInvalidMediaRequestMapping, index)
			}
		case "cast":
			switch strings.ToLower(strings.TrimSpace(rule.Cast)) {
			case "string", "number", "integer", "boolean":
			default:
				return fmt.Errorf("%w: rule %d cast type %q", ErrInvalidMediaRequestMapping, index, rule.Cast)
			}
		}
	}
	return nil
}

// Apply returns an independent transformed request. rename removes its source;
// all other operations leave the source intact.
func (m MediaRequestMapping) Apply(request map[string]any) (map[string]any, error) {
	if err := m.Validate(); err != nil {
		return nil, err
	}
	result := cloneMediaMappingObject(request)
	for index := range m.Rules {
		rule := m.Rules[index]
		op := strings.ToLower(strings.TrimSpace(rule.Operation))
		var value any
		var found bool
		switch op {
		case "default":
			if _, exists := mediaMappingGet(result, rule.Target); exists {
				continue
			}
			value = rule.Value
		case "rename", "copy", "enum", "cast":
			value, found = mediaMappingGet(result, rule.Source)
			if !found {
				return nil, fmt.Errorf("%w: rule %d source %s", ErrMediaMappingValueMissing, index, rule.Source)
			}
		}
		if op == "enum" {
			key, ok := value.(string)
			if !ok {
				return nil, fmt.Errorf("%w: rule %d source is not a string", ErrMediaMappingEnumMiss, index)
			}
			var okMapped bool
			value, okMapped = rule.Values[key]
			if !okMapped {
				return nil, fmt.Errorf("%w: rule %d value %q", ErrMediaMappingEnumMiss, index, key)
			}
		}
		if op == "cast" {
			castValue, castErr := castMediaMappingValue(value, rule.Cast)
			if castErr != nil {
				return nil, fmt.Errorf("%w: rule %d: %w", ErrMediaMappingCastFailed, index, castErr)
			}
			value = castValue
		}
		if _, exists := mediaMappingGet(result, rule.Target); exists {
			if op == "rename" || op == "copy" || (op != "default" && rule.Source != rule.Target) {
				return nil, fmt.Errorf("%w: rule %d target %s", ErrMediaMappingTargetConflict, index, rule.Target)
			}
		}
		if err := mediaMappingSet(result, rule.Target, value); err != nil {
			return nil, fmt.Errorf("%w: rule %d: %w", ErrInvalidMediaRequestMapping, index, err)
		}
		if op == "rename" {
			mediaMappingDelete(result, rule.Source)
		}
	}
	return result, nil
}

func isMediaMappingOperation(op string) bool {
	switch op {
	case "rename", "copy", "default", "enum", "cast":
		return true
	default:
		return false
	}
}

func validateMediaMappingPath(path string) error {
	if path == "" {
		return errors.New("path is empty")
	}
	for _, segment := range strings.Split(path, ".") {
		if segment == "" || !(segment[0] == '_' || segment[0] >= 'a' && segment[0] <= 'z' || segment[0] >= 'A' && segment[0] <= 'Z') {
			return fmt.Errorf("unsafe path %q", path)
		}
		for _, char := range segment[1:] {
			if char != '_' && !(char >= 'a' && char <= 'z') && !(char >= 'A' && char <= 'Z') && !(char >= '0' && char <= '9') {
				return fmt.Errorf("unsafe path %q", path)
			}
		}
	}
	return nil
}

func cloneMediaMappingObject(input map[string]any) map[string]any {
	output := make(map[string]any, len(input))
	for key, value := range input {
		output[key] = cloneMediaMappingValue(value)
	}
	return output
}

func cloneMediaMappingValue(value any) any {
	switch value := value.(type) {
	case map[string]any:
		return cloneMediaMappingObject(value)
	case []any:
		result := make([]any, len(value))
		for index := range value {
			result[index] = cloneMediaMappingValue(value[index])
		}
		return result
	default:
		return value
	}
}

func mediaMappingGet(input map[string]any, path string) (any, bool) {
	current := input
	segments := strings.Split(path, ".")
	for _, segment := range segments[:len(segments)-1] {
		next, ok := current[segment].(map[string]any)
		if !ok {
			return nil, false
		}
		current = next
	}
	value, ok := current[segments[len(segments)-1]]
	return value, ok
}

func mediaMappingSet(input map[string]any, path string, value any) error {
	current := input
	segments := strings.Split(path, ".")
	for _, segment := range segments[:len(segments)-1] {
		if next, exists := current[segment]; exists {
			object, ok := next.(map[string]any)
			if !ok {
				return errors.New("target parent is not an object")
			}
			current = object
			continue
		}
		next := make(map[string]any)
		current[segment] = next
		current = next
	}
	current[segments[len(segments)-1]] = value
	return nil
}

func mediaMappingDelete(input map[string]any, path string) {
	current := input
	segments := strings.Split(path, ".")
	for _, segment := range segments[:len(segments)-1] {
		next, ok := current[segment].(map[string]any)
		if !ok {
			return
		}
		current = next
	}
	delete(current, segments[len(segments)-1])
}

func castMediaMappingValue(value any, kind string) (any, error) {
	switch strings.ToLower(strings.TrimSpace(kind)) {
	case "string":
		return fmt.Sprint(value), nil
	case "number":
		switch value := value.(type) {
		case float64:
			return value, nil
		case json.Number:
			return value.Float64()
		case string:
			return strconv.ParseFloat(strings.TrimSpace(value), 64)
		default:
			return nil, fmt.Errorf("cannot cast %T to number", value)
		}
	case "integer":
		switch value := value.(type) {
		case float64:
			if value != float64(int64(value)) {
				return nil, errors.New("number is not an integer")
			}
			return int64(value), nil
		case string:
			return strconv.ParseInt(strings.TrimSpace(value), 10, 64)
		default:
			return nil, fmt.Errorf("cannot cast %T to integer", value)
		}
	case "boolean":
		switch value := value.(type) {
		case bool:
			return value, nil
		case string:
			return strconv.ParseBool(strings.TrimSpace(value))
		default:
			return nil, fmt.Errorf("cannot cast %T to boolean", value)
		}
	}
	return nil, errors.New("unsupported cast")
}
