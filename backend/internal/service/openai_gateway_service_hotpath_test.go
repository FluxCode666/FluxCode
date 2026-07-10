package service

import (
	"encoding/json"
	"net/http/httptest"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/pkg/apicompat"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestExtractOpenAIRequestMetaFromBody(t *testing.T) {
	tests := []struct {
		name          string
		body          []byte
		wantModel     string
		wantStream    bool
		wantPromptKey string
	}{
		{
			name:          "完整字段",
			body:          []byte(`{"model":"gpt-5","stream":true,"prompt_cache_key":" ses-1 "}`),
			wantModel:     "gpt-5",
			wantStream:    true,
			wantPromptKey: "ses-1",
		},
		{
			name:          "缺失可选字段",
			body:          []byte(`{"model":"gpt-4"}`),
			wantModel:     "gpt-4",
			wantStream:    false,
			wantPromptKey: "",
		},
		{
			name:          "空请求体",
			body:          nil,
			wantModel:     "",
			wantStream:    false,
			wantPromptKey: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			model, stream, promptKey := extractOpenAIRequestMetaFromBody(tt.body)
			require.Equal(t, tt.wantModel, model)
			require.Equal(t, tt.wantStream, stream)
			require.Equal(t, tt.wantPromptKey, promptKey)
		})
	}
}

func TestExtractOpenAIReasoningEffortFromBody(t *testing.T) {
	tests := []struct {
		name      string
		body      []byte
		model     string
		wantNil   bool
		wantValue string
	}{
		{
			name:      "优先读取 reasoning.effort",
			body:      []byte(`{"reasoning":{"effort":"medium"}}`),
			model:     "gpt-5-high",
			wantNil:   false,
			wantValue: "medium",
		},
		{
			name:      "兼容 reasoning_effort",
			body:      []byte(`{"reasoning_effort":"x-high"}`),
			model:     "",
			wantNil:   false,
			wantValue: "xhigh",
		},
		{
			name:    "minimal 归一化为空",
			body:    []byte(`{"reasoning":{"effort":"minimal"}}`),
			model:   "gpt-5-high",
			wantNil: true,
		},
		{
			name:      "缺失字段时从模型后缀推导",
			body:      []byte(`{"input":"hi"}`),
			model:     "gpt-5-high",
			wantNil:   false,
			wantValue: "high",
		},
		{
			name:    "未知后缀不返回",
			body:    []byte(`{"input":"hi"}`),
			model:   "gpt-5-unknown",
			wantNil: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := extractOpenAIReasoningEffortFromBody(tt.body, tt.model)
			if tt.wantNil {
				require.Nil(t, got)
				return
			}
			require.NotNil(t, got)
			require.Equal(t, tt.wantValue, *got)
		})
	}
}

func TestExtractOpenAIReasoningEffortFromBody_GPT56Max(t *testing.T) {
	got := extractOpenAIReasoningEffortFromBody([]byte(`{"reasoning":{"effort":"max"}}`), "gpt-5.6")
	require.NotNil(t, got)
	require.Equal(t, "max", *got)
}

func TestExtractOpenAIReasoningEffortFromBody_MaxRejectedForNonGPT56(t *testing.T) {
	require.Nil(t, extractOpenAIReasoningEffortFromBody([]byte(`{"reasoning":{"effort":"max"}}`), "gpt-5.4"))
}

func TestExtractOpenAIReasoningEffortFromBody_UsesModelCandidates(t *testing.T) {
	got := extractOpenAIReasoningEffortFromBody([]byte(`{"input":"hi"}`), "gpt-5.6-sol", "gpt-5.6-max")
	require.NotNil(t, got)
	require.Equal(t, "max", *got)
}

func TestGetOpenAIRequestBodyMap_UsesContextCache(t *testing.T) {
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)

	cached := map[string]any{"model": "cached-model", "stream": true}
	c.Set(OpenAIParsedRequestBodyKey, cached)

	got, err := getOpenAIRequestBodyMap(c, []byte(`{invalid-json`))
	require.NoError(t, err)
	require.Equal(t, cached, got)
}

func TestGetOpenAIRequestBodyMap_ParseErrorWithoutCache(t *testing.T) {
	_, err := getOpenAIRequestBodyMap(nil, []byte(`{invalid-json`))
	require.Error(t, err)
	require.Contains(t, err.Error(), "parse request")
}

func TestGetOpenAIRequestBodyMap_WriteBackContextCache(t *testing.T) {
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)

	got, err := getOpenAIRequestBodyMap(c, []byte(`{"model":"gpt-5","stream":true}`))
	require.NoError(t, err)
	require.Equal(t, "gpt-5", got["model"])

	cached, ok := c.Get(OpenAIParsedRequestBodyKey)
	require.True(t, ok)
	cachedMap, ok := cached.(map[string]any)
	require.True(t, ok)
	require.Equal(t, got, cachedMap)
}

func TestSanitizeEmptyBase64InputImagesInOpenAIRequestBodyMap(t *testing.T) {
	var reqBody map[string]any
	require.NoError(t, json.Unmarshal([]byte(`{
		"model":"gpt-5.4",
		"input":[
			{"role":"user","content":[
				{"type":"input_text","text":"Describe this"},
				{"type":"input_image","image_url":"data:image/png;base64,   "},
				{"type":"input_image","image_url":"data:image/png;base64,abc123"}
			]},
			{"role":"user","content":[
				{"type":"input_image","image_url":"data:image/png;base64,"}
			]},
			{"type":"input_image","image_url":"data:image/png;base64,"},
			{"type":"input_image","image_url":"data:image/png;base64,top-level-valid"}
		]
	}`), &reqBody))

	require.True(t, sanitizeEmptyBase64InputImagesInOpenAIRequestBodyMap(reqBody))

	normalized, err := json.Marshal(reqBody)
	require.NoError(t, err)
	require.JSONEq(t, `{
		"model":"gpt-5.4",
		"input":[
			{"role":"user","content":[
				{"type":"input_text","text":"Describe this"},
				{"type":"input_image","image_url":"data:image/png;base64,abc123"}
			]},
			{"type":"input_image","image_url":"data:image/png;base64,top-level-valid"}
		]
	}`, string(normalized))
}

func TestSanitizeEmptyBase64InputImagesInOpenAIBody(t *testing.T) {
	body, changed, err := sanitizeEmptyBase64InputImagesInOpenAIBody([]byte(`{
		"model":"gpt-5.4",
		"stream":true,
		"input":[
			{"role":"user","content":[
				{"type":"input_text","text":"Describe this"},
				{"type":"input_image","image_url":"data:image/png;base64,"}
			]}
		]
	}`))
	require.NoError(t, err)
	require.True(t, changed)
	require.JSONEq(t, `{
		"model":"gpt-5.4",
		"stream":true,
		"input":[
			{"role":"user","content":[
				{"type":"input_text","text":"Describe this"}
			]}
		]
	}`, string(body))
}

func TestExtractOpenAIUsageFromJSONBytes_CacheWriteFields(t *testing.T) {
	tests := []struct {
		name string
		body []byte
		want int
	}{
		{name: "nested cache write", body: []byte(`{"usage":{"input_tokens":10,"output_tokens":1,"input_tokens_details":{"cache_write_tokens":7}}}`), want: 7},
		{name: "nested cache creation", body: []byte(`{"usage":{"input_tokens":10,"output_tokens":1,"input_tokens_details":{"cache_creation_tokens":8}}}`), want: 8},
		{name: "top level cache write input", body: []byte(`{"usage":{"input_tokens":10,"output_tokens":1,"cache_write_input_tokens":9}}`), want: 9},
		{name: "top level cache creation input", body: []byte(`{"usage":{"input_tokens":10,"output_tokens":1,"cache_creation_input_tokens":6}}`), want: 6},
		{name: "negative clamps to zero", body: []byte(`{"usage":{"input_tokens":10,"output_tokens":1,"input_tokens_details":{"cache_write_tokens":-4}}}`), want: 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := extractOpenAIUsageFromJSONBytes(tt.body)
			require.True(t, ok)
			require.Equal(t, tt.want, got.CacheCreationInputTokens)
		})
	}
}

func TestParseSSEUsageBytes_CacheWriteFields(t *testing.T) {
	svc := &OpenAIGatewayService{}
	usage := &OpenAIUsage{}

	svc.parseSSEUsageBytes([]byte(`{"type":"response.completed","response":{"usage":{"input_tokens":10,"output_tokens":1,"input_tokens_details":{"cached_tokens":3,"cache_write_tokens":4}}}}`), usage)

	require.Equal(t, 10, usage.InputTokens)
	require.Equal(t, 1, usage.OutputTokens)
	require.Equal(t, 3, usage.CacheReadInputTokens)
	require.Equal(t, 4, usage.CacheCreationInputTokens)
}

func TestOpenAIUsageFromResponsesUsage_PreservesPresencePriorityAndClampsNegative(t *testing.T) {
	tests := []struct {
		name string
		body string
	}{
		{
			name: "nested zero wins over positive top level",
			body: `{"input_tokens":-1,"output_tokens":-2,"input_tokens_details":{"cached_tokens":-3,"cache_write_tokens":0},"cache_write_input_tokens":9}`,
		},
		{
			name: "nested negative wins over positive top level",
			body: `{"input_tokens":-1,"output_tokens":-2,"input_tokens_details":{"cached_tokens":-3,"cache_creation_tokens":-4},"cache_creation_input_tokens":9}`,
		},
		{
			name: "prompt detail zero wins over positive top level",
			body: `{"input_tokens":-1,"output_tokens":-2,"prompt_tokens_details":{"cache_write_tokens":0},"cache_write_tokens":9}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var responsesUsage apicompat.ResponsesUsage
			require.NoError(t, json.Unmarshal([]byte(tt.body), &responsesUsage))

			usage := openAIUsageFromResponsesUsage(&responsesUsage)
			require.Zero(t, usage.InputTokens)
			require.Zero(t, usage.OutputTokens)
			require.Zero(t, usage.CacheReadInputTokens)
			require.Zero(t, usage.CacheCreationInputTokens)
		})
	}
}
