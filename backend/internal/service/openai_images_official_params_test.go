package service

import (
	"bytes"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestParseOpenAIImagesRequestClassifiesOfficialAndArbitrarySizes(t *testing.T) {
	gin.SetMode(gin.TestMode)

	tests := []struct {
		size string
		want string
	}{
		{size: "1024x1024", want: "1K"},
		{size: "1536x1024", want: "2K"},
		{size: "2048x1152", want: "2K"},
		{size: "2048x2048", want: "2K"},
		{size: "3840x2160", want: "4K"},
		{size: "2160x3840", want: "4K"},
		{size: "512x512", want: "1K"},
		{size: "1280x768", want: "2K"},
		{size: "2560x1440", want: "4K"},
		{size: "auto", want: "2K"},
		{size: "invalid", want: "2K"},
	}

	svc := &OpenAIGatewayService{}
	for _, tt := range tests {
		t.Run(tt.size, func(t *testing.T) {
			body := []byte(fmt.Sprintf(`{"model":"gpt-image-2","prompt":"draw a cat","size":%q}`, tt.size))
			req := httptest.NewRequest(http.MethodPost, "/v1/images/generations", bytes.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			rec := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(rec)
			c.Request = req

			parsed, err := svc.ParseOpenAIImagesRequest(c, body)

			require.NoError(t, err)
			require.Equal(t, tt.size, parsed.Size)
			require.Equal(t, tt.want, parsed.SizeTier)
		})
	}
}

func TestBuildOpenAIImagesResponsesRequestUsesOfficialToolParameters(t *testing.T) {
	outputCompression := 80
	partialImages := 2
	parsed := &OpenAIImagesRequest{
		Endpoint:          openAIImagesEditsEndpoint,
		Model:             "gpt-image-2",
		Prompt:            "preserve the face and replace the background",
		N:                 3,
		Size:              "1536x1024",
		InputFidelity:     "high",
		OutputCompression: &outputCompression,
		PartialImages:     &partialImages,
		InputImageURLs:    []string{"data:image/png;base64,aGVsbG8="},
	}

	body, err := buildOpenAIImagesResponsesRequest(parsed, "gpt-image-2")

	require.NoError(t, err)
	tool := gjson.GetBytes(body, "tools.0")
	require.Equal(t, "edit", tool.Get("action").String())
	require.Equal(t, "high", tool.Get("input_fidelity").String())
	require.Equal(t, int64(80), tool.Get("output_compression").Int())
	require.Equal(t, int64(2), tool.Get("partial_images").Int())
	require.False(t, tool.Get("n").Exists())
}
