package chatgptweb

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"math/rand/v2"
	"strings"
	"time"
)

type orderedMap struct {
	keys   []string
	values map[string]any
}

func newOrderedMap() *orderedMap {
	return &orderedMap{values: make(map[string]any)}
}

func (m *orderedMap) Add(key string, value any) {
	if _, exists := m.values[key]; !exists {
		m.keys = append(m.keys, key)
	}
	m.values[key] = value
}

func turnstileToStr(value any) string {
	if value == nil {
		return "undefined"
	}
	switch v := value.(type) {
	case float64:
		return fmt.Sprintf("%v", v)
	case string:
		special := map[string]string{
			"window.Math":                "[object Math]",
			"window.Reflect":             "[object Reflect]",
			"window.performance":         "[object Performance]",
			"window.localStorage":        "[object Storage]",
			"window.Object":              "function Object() { [native code] }",
			"window.Reflect.set":         "function set() { [native code] }",
			"window.performance.now":     "function () { [native code] }",
			"window.Object.create":       "function create() { [native code] }",
			"window.Object.keys":         "function keys() { [native code] }",
			"window.Math.random":         "function random() { [native code] }",
		}
		if s, ok := special[v]; ok {
			return s
		}
		return v
	case []string:
		return strings.Join(v, ",")
	case []any:
		parts := make([]string, 0, len(v))
		for _, item := range v {
			parts = append(parts, fmt.Sprintf("%v", item))
		}
		return strings.Join(parts, ",")
	default:
		return fmt.Sprintf("%v", v)
	}
}

func xorString(text, key string) string {
	if key == "" {
		return text
	}
	keyRunes := []rune(key)
	textRunes := []rune(text)
	result := make([]rune, len(textRunes))
	for i, ch := range textRunes {
		result[i] = ch ^ keyRunes[i%len(keyRunes)]
	}
	return string(result)
}

// SolveTurnstileToken solves a ChatGPT turnstile challenge.
// dx is the base64-encoded, XOR-encrypted token list; p is the XOR key.
// Returns empty string on failure.
func SolveTurnstileToken(dx, p string) string {
	if dx == "" {
		return ""
	}
	decoded, err := base64.StdEncoding.DecodeString(dx)
	if err != nil {
		return ""
	}
	decrypted := xorString(string(decoded), p)

	var tokenList [][]any
	if err := json.Unmarshal([]byte(decrypted), &tokenList); err != nil {
		return ""
	}

	processMap := make(map[float64]any)
	startTime := time.Now()
	var result string

	// Function dispatch table matching Python reference
	fn1 := func(e, t float64) {
		processMap[e] = xorString(turnstileToStr(processMap[e]), turnstileToStr(processMap[t]))
	}
	fn2 := func(e float64, t any) {
		processMap[e] = t
	}
	fn3 := func(e string) {
		result = base64.StdEncoding.EncodeToString([]byte(e))
	}
	fn5 := func(e, t float64) {
		current := processMap[e]
		incoming := processMap[t]
		switch cv := current.(type) {
		case []any:
			processMap[e] = append(cv, incoming)
		default:
			processMap[e] = turnstileToStr(current) + turnstileToStr(incoming)
		}
	}
	fn6 := func(e, t, n float64) {
		tv := turnstileToStr(processMap[t])
		nv := turnstileToStr(processMap[n])
		value := tv + "." + nv
		if value == "window.document.location" {
			processMap[e] = "https://chatgpt.com/"
		} else {
			processMap[e] = value
		}
	}
	fn8 := func(e, t float64) {
		processMap[e] = processMap[t]
	}
	fn14 := func(e, t float64) {
		s, ok := processMap[t].(string)
		if !ok {
			return
		}
		var v any
		if err := json.Unmarshal([]byte(s), &v); err == nil {
			processMap[e] = v
		}
	}
	fn15 := func(e, t float64) {
		b, err := json.Marshal(processMap[t])
		if err == nil {
			processMap[e] = string(b)
		}
	}
	fn18 := func(e float64) {
		s := turnstileToStr(processMap[e])
		decoded, err := base64.StdEncoding.DecodeString(s)
		if err == nil {
			processMap[e] = string(decoded)
		}
	}
	fn19 := func(e float64) {
		s := turnstileToStr(processMap[e])
		processMap[e] = base64.StdEncoding.EncodeToString([]byte(s))
	}
	fn24 := func(e, t, n float64) {
		tv, ok1 := processMap[t].(string)
		nv, ok2 := processMap[n].(string)
		if ok1 && ok2 {
			processMap[e] = tv + "." + nv
		}
	}

	// Initialize process map
	processMap[9] = tokenList
	processMap[10] = "window"
	processMap[16] = p

	for _, token := range tokenList {
		if len(token) == 0 {
			continue
		}
		opcode, ok := toFloat64(token[0])
		if !ok {
			continue
		}
		func() {
			defer func() { recover() }() //nolint:errcheck
			switch opcode {
			case 1:
				if len(token) >= 3 {
					e, _ := toFloat64(token[1])
					t, _ := toFloat64(token[2])
					fn1(e, t)
				}
			case 2:
				if len(token) >= 3 {
					e, _ := toFloat64(token[1])
					fn2(e, token[2])
				}
			case 3:
				if len(token) >= 2 {
					s := turnstileToStr(processMap[mustFloat64(token[1])])
					fn3(s)
				}
			case 5:
				if len(token) >= 3 {
					e, _ := toFloat64(token[1])
					t, _ := toFloat64(token[2])
					fn5(e, t)
				}
			case 6:
				if len(token) >= 4 {
					e, _ := toFloat64(token[1])
					t, _ := toFloat64(token[2])
					n, _ := toFloat64(token[3])
					fn6(e, t, n)
				}
			case 7:
				if len(token) >= 2 {
					e, _ := toFloat64(token[1])
					target := processMap[e]
					targetStr, isStr := target.(string)
					if isStr && targetStr == "window.Reflect.set" && len(token) >= 5 {
						obj, ok := processMap[mustFloat64(token[2])].(*orderedMap)
						if ok {
							keyName := turnstileToStr(processMap[mustFloat64(token[3])])
							val := processMap[mustFloat64(token[4])]
							obj.Add(keyName, val)
						}
					}
				}
			case 8:
				if len(token) >= 3 {
					e, _ := toFloat64(token[1])
					t, _ := toFloat64(token[2])
					fn8(e, t)
				}
			case 14:
				if len(token) >= 3 {
					e, _ := toFloat64(token[1])
					t, _ := toFloat64(token[2])
					fn14(e, t)
				}
			case 15:
				if len(token) >= 3 {
					e, _ := toFloat64(token[1])
					t, _ := toFloat64(token[2])
					fn15(e, t)
				}
			case 17:
				if len(token) >= 3 {
					e, _ := toFloat64(token[1])
					t, _ := toFloat64(token[2])
					target := turnstileToStr(processMap[t])
					switch target {
					case "window.performance.now":
						elapsed := time.Since(startTime)
						processMap[e] = (float64(elapsed.Nanoseconds()) + rand.Float64()) / 1e6
					case "window.Object.create":
						processMap[e] = newOrderedMap()
					case "window.Object.keys":
						if len(token) >= 4 {
							arg := processMap[mustFloat64(token[3])]
							if s, ok := arg.(string); ok && s == "window.localStorage" {
								processMap[e] = []any{
									"STATSIG_LOCAL_STORAGE_INTERNAL_STORE_V4",
									"STATSIG_LOCAL_STORAGE_STABLE_ID",
									"client-correlated-secret",
									"oai/apps/capExpiresAt",
									"oai-did",
									"STATSIG_LOCAL_STORAGE_LOGGING_REQUEST",
									"UiState.isNavigationCollapsed.1",
								}
							}
						}
					case "window.Math.random":
						processMap[e] = rand.Float64()
					}
				}
			case 18:
				if len(token) >= 2 {
					e, _ := toFloat64(token[1])
					fn18(e)
				}
			case 19:
				if len(token) >= 2 {
					e, _ := toFloat64(token[1])
					fn19(e)
				}
			case 20:
				if len(token) >= 4 {
					e, _ := toFloat64(token[1])
					t, _ := toFloat64(token[2])
					if processMap[e] == processMap[t] {
						// call fn at token[3] with remaining args
						n, _ := toFloat64(token[3])
						target := processMap[n]
						if targetStr, ok := target.(string); ok && targetStr == "window.Reflect.set" && len(token) >= 7 {
							obj, ok := processMap[mustFloat64(token[4])].(*orderedMap)
							if ok {
								keyName := turnstileToStr(processMap[mustFloat64(token[5])])
								val := processMap[mustFloat64(token[6])]
								obj.Add(keyName, val)
							}
						}
					}
				}
			case 21:
				// no-op
			case 23:
				if len(token) >= 3 {
					e, _ := toFloat64(token[1])
					if processMap[e] != nil {
						// call fn at token[2]
						_ = token[2]
					}
				}
			case 24:
				if len(token) >= 4 {
					e, _ := toFloat64(token[1])
					t, _ := toFloat64(token[2])
					n, _ := toFloat64(token[3])
					fn24(e, t, n)
				}
			}
		}()
	}

	return result
}

func toFloat64(v any) (float64, bool) {
	switch n := v.(type) {
	case float64:
		return n, true
	case int:
		return float64(n), true
	case int64:
		return float64(n), true
	case json.Number:
		f, err := n.Float64()
		return f, err == nil
	default:
		return 0, false
	}
}

func mustFloat64(v any) float64 {
	f, _ := toFloat64(v)
	return f
}
