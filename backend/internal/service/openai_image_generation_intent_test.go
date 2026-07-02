package service

import "testing"

func TestGroupAllowsImageGeneration(t *testing.T) {
	if !GroupAllowsImageGeneration(nil) {
		t.Fatalf("nil group must preserve existing ungrouped-key allow behavior")
	}
	if GroupAllowsImageGeneration(&Group{AllowImageGeneration: false}) {
		t.Fatalf("group with AllowImageGeneration=false must deny image generation")
	}
	if !GroupAllowsImageGeneration(&Group{AllowImageGeneration: true}) {
		t.Fatalf("group with AllowImageGeneration=true must allow image generation")
	}
}

func TestIsImageGenerationIntent(t *testing.T) {
	cases := []struct {
		name           string
		endpoint       string
		requestedModel string
		body           []byte
		want           bool
	}{
		{name: "images endpoint", endpoint: "/v1/images/generations", want: true},
		{name: "image requested model", endpoint: "/v1/responses", requestedModel: "gpt-image-2", want: true},
		{name: "body model image", endpoint: "/v1/responses", body: []byte(`{"model":"gpt-image-2"}`), want: true},
		{name: "image tool", endpoint: "/v1/responses", body: []byte(`{"model":"gpt-5.5","tools":[{"type":"image_generation"}]}`), want: true},
		{name: "tool choice string", endpoint: "/v1/responses", body: []byte(`{"tool_choice":"image_generation"}`), want: true},
		{name: "tool choice object", endpoint: "/v1/responses", body: []byte(`{"tool_choice":{"type":"image_generation"}}`), want: true},
		{name: "plain text", endpoint: "/v1/responses", requestedModel: "gpt-5.5", body: []byte(`{"input":"write code"}`), want: false},
		{name: "invalid json", endpoint: "/v1/responses", requestedModel: "gpt-5.5", body: []byte(`{bad`), want: false},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsImageGenerationIntent(tt.endpoint, tt.requestedModel, tt.body); got != tt.want {
				t.Fatalf("IsImageGenerationIntent() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestIsImageGenerationIntentMap(t *testing.T) {
	reqBody := map[string]any{
		"model": "gpt-5.5",
		"tools": []any{map[string]any{"type": "image_generation"}},
	}
	if !IsImageGenerationIntentMap("/v1/responses", "gpt-5.5", reqBody) {
		t.Fatalf("map with image_generation tool must be image intent")
	}
	if IsImageGenerationIntentMap("/v1/responses", "gpt-5.5", map[string]any{"input": "write code"}) {
		t.Fatalf("plain map must not be image intent")
	}
}

func float64PtrForTest(v float64) *float64 {
	return &v
}
