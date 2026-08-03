package openai_compat

import "testing"

func TestResolveResponsesSupport(t *testing.T) {
	tests := []struct {
		name  string
		extra map[string]any
		want  AccountResponsesSupport
	}{
		{"nil extra", nil, ResponsesSupportUnknown},
		{"empty extra", map[string]any{}, ResponsesSupportUnknown},
		{"key missing", map[string]any{"other": "value"}, ResponsesSupportUnknown},
		{"value true", map[string]any{ExtraKeyResponsesSupported: true}, ResponsesSupportYes},
		{"value false", map[string]any{ExtraKeyResponsesSupported: false}, ResponsesSupportNo},
		{"force chat does not change probed capability", map[string]any{
			ExtraKeyResponsesMode:      string(ResponsesSupportModeForceChatCompletions),
			ExtraKeyResponsesSupported: true,
		}, ResponsesSupportYes},
		{"auto does not change probed capability", map[string]any{
			ExtraKeyResponsesMode:      string(ResponsesSupportModeAuto),
			ExtraKeyResponsesSupported: false,
		}, ResponsesSupportNo},
		{"value wrong type string", map[string]any{ExtraKeyResponsesSupported: "true"}, ResponsesSupportUnknown},
		{"mode wrong type falls back to probe", map[string]any{
			ExtraKeyResponsesMode:      1,
			ExtraKeyResponsesSupported: true,
		}, ResponsesSupportYes},
		{"value wrong type number", map[string]any{ExtraKeyResponsesSupported: 1}, ResponsesSupportUnknown},
		{"value nil", map[string]any{ExtraKeyResponsesSupported: nil}, ResponsesSupportUnknown},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := ResolveResponsesSupport(tc.extra)
			if got != tc.want {
				t.Errorf("ResolveResponsesSupport(%v) = %v, want %v", tc.extra, got, tc.want)
			}
		})
	}
}

func TestShouldUseResponsesAPI(t *testing.T) {
	tests := []struct {
		name  string
		extra map[string]any
		want  bool
	}{
		// 关键不变量：除非显式打开 Chat 开关，否则始终使用 Responses。
		{"unknown defaults to true (preserve old behavior)", nil, true},
		{"unknown empty defaults to true", map[string]any{}, true},
		{"unknown wrong type defaults to true", map[string]any{ExtraKeyResponsesSupported: "yes"}, true},

		// 探测结果只描述能力，不改变协议路由。
		{"explicitly supported", map[string]any{ExtraKeyResponsesSupported: true}, true},
		{"explicitly unsupported still uses responses", map[string]any{ExtraKeyResponsesSupported: false}, true},
		{"force chat overrides supported", map[string]any{
			ExtraKeyResponsesMode:      string(ResponsesSupportModeForceChatCompletions),
			ExtraKeyResponsesSupported: true,
		}, false},
		{"force chat also overrides unsupported", map[string]any{
			ExtraKeyResponsesMode:      string(ResponsesSupportModeForceChatCompletions),
			ExtraKeyResponsesSupported: false,
		}, false},
		{"auto ignores unsupported probe", map[string]any{
			ExtraKeyResponsesMode:      string(ResponsesSupportModeAuto),
			ExtraKeyResponsesSupported: false,
		}, true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := ShouldUseResponsesAPI(tc.extra)
			if got != tc.want {
				t.Errorf("ShouldUseResponsesAPI(%v) = %v, want %v", tc.extra, got, tc.want)
			}
		})
	}
}
