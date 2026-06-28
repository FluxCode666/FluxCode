package service

import (
	"context"
	"encoding/base64"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestForwardOpenAIImagesAPIKeyRecordsGeneratedImages(t *testing.T) {
	body := []byte(`{"prompt":"draw a cat","response_format":"b64_json"}`)
	c, rec := newOpenAIImagesForwardTestContext(body)
	upstream := &httpUpstreamRecorder{
		resp: &http.Response{
			StatusCode: http.StatusOK,
			Header: http.Header{
				"Content-Type": []string{"application/json"},
				"X-Request-Id": []string{"req_img_123"},
			},
			Body: io.NopCloser(strings.NewReader(
				`{"created":1710000000,"data":[{"b64_json":"aGVsbG8=","revised_prompt":"cute cat","output_format":"png"}]}`,
			)),
		},
	}
	store := &generatedImageStoreStub{}
	svc := newOpenAIImagesForwardTestService(upstream)
	svc.SetGeneratedImageStore(store)
	account := newOpenAIImagesAPIKeyTestAccount()
	parsed, err := svc.ParseOpenAIImagesRequest(c, body)
	require.NoError(t, err)

	result, err := svc.ForwardImages(context.Background(), c, account, body, parsed, "", &GeneratedImageRecordContext{
		UserID:   12,
		APIKeyID: 34,
	})

	require.NoError(t, err)
	require.NotNil(t, result)
	require.Equal(t, "aGVsbG8=", gjson.GetBytes(rec.Body.Bytes(), "data.0.b64_json").String())
	require.Len(t, store.created, 1)

	record := store.created[0]
	require.Equal(t, GeneratedImageProviderOpenAI, record.Provider)
	require.Equal(t, int64(12), record.UserID)
	require.Equal(t, int64(34), record.APIKeyID)
	require.Equal(t, account.ID, record.AccountID)
	require.Equal(t, "req_img_123", record.RequestID)
	require.Equal(t, "gpt-image-2", record.Model)
	require.Equal(t, "draw a cat", record.Prompt)
	require.Equal(t, "cute cat", record.RevisedPrompt)
	require.Equal(t, "b64_json", record.ResponseFormat)
	require.Equal(t, "b64_json", record.Source)
	require.Equal(t, "image/png", record.ContentType)
	require.Equal(t, []byte("hello"), record.ImageData)
	require.Equal(t, len("hello"), record.SizeBytes)
}

func TestForwardOpenAIImagesAPIKeyUploadsGeneratedImagesWhenB64JSONRequested(t *testing.T) {
	body := []byte(`{"prompt":"draw a cat","response_format":"b64_json"}`)
	c, _ := newOpenAIImagesForwardTestContext(body)
	upstream := &httpUpstreamRecorder{
		resp: &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": []string{"application/json"}},
			Body: io.NopCloser(strings.NewReader(
				`{"created":1710000000,"data":[{"b64_json":"aGVsbG8=","output_format":"png"}]}`,
			)),
		},
	}
	objectStore := &generatedImageObjectStoreStub{url: "https://cdn.example.com/openai/generated.png"}
	svc := newOpenAIImagesForwardTestService(upstream)
	svc.SetGeneratedImageObjectStore(objectStore)
	account := newOpenAIImagesAPIKeyTestAccount()
	parsed, err := svc.ParseOpenAIImagesRequest(c, body)
	require.NoError(t, err)

	result, err := svc.ForwardImages(context.Background(), c, account, body, parsed, "")

	require.NoError(t, err)
	require.NotNil(t, result)
	require.Len(t, objectStore.uploads, 1)
	require.Equal(t, []byte("hello"), objectStore.uploads[0].Data)
	require.Equal(t, "image/png", objectStore.uploads[0].ContentType)
}

func TestForwardOpenAIImagesAPIKeyFallsBackToProxyWhenObjectStoreUploadFails(t *testing.T) {
	body := []byte(`{"prompt":"draw a cat","response_format":"url"}`)
	c, rec := newOpenAIImagesForwardTestContext(body)
	upstream := &httpUpstreamRecorder{
		resp: &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": []string{"application/json"}},
			Body: io.NopCloser(strings.NewReader(
				`{"created":1710000000,"data":[{"b64_json":"aGVsbG8=","output_format":"png"}]}`,
			)),
		},
	}
	cache := newOpenAIImagesFakeImageCache()
	objectStore := &generatedImageObjectStoreStub{err: errors.New("qiniu unavailable")}
	svc := newOpenAIImagesForwardTestService(upstream)
	svc.SetOpenAIImageCache(cache)
	svc.SetGeneratedImageObjectStore(objectStore)
	account := newOpenAIImagesAPIKeyTestAccount()
	parsed, err := svc.ParseOpenAIImagesRequest(c, body)
	require.NoError(t, err)

	result, err := svc.ForwardImages(context.Background(), c, account, body, parsed, "")

	require.NoError(t, err)
	require.NotNil(t, result)
	proxyURL := gjson.GetBytes(rec.Body.Bytes(), "data.0.url").String()
	require.Contains(t, proxyURL, "proxy.example.test/v1/images/proxy/")
	require.False(t, gjson.GetBytes(rec.Body.Bytes(), "data.0.b64_json").Exists())
	require.Len(t, objectStore.uploads, 1)
	require.Len(t, cache.sets, 1)
	require.Equal(t, []byte("hello"), cache.sets[0].data)
	require.Equal(t, "image/png", cache.sets[0].contentType)
}

type generatedImageStoreStub struct {
	created []GeneratedImage
}

func (s *generatedImageStoreStub) Create(ctx context.Context, image *GeneratedImage) (*GeneratedImage, error) {
	copy := *image
	copy.ImageData = append([]byte(nil), image.ImageData...)
	copy.ID = int64(len(s.created) + 1)
	s.created = append(s.created, copy)
	return &copy, nil
}

func (s *generatedImageStoreStub) List(ctx context.Context, params GeneratedImageListParams) ([]GeneratedImage, *pagination.PaginationResult, error) {
	return nil, nil, nil
}

func (s *generatedImageStoreStub) GetContent(ctx context.Context, id int64) ([]byte, string, error) {
	return nil, "", nil
}

func (s *generatedImageStoreStub) DeleteByDateRange(ctx context.Context, startAt, endAt time.Time) (int64, error) {
	return 0, nil
}

type generatedImageObjectStoreStub struct {
	uploads []GeneratedImageObjectUpload
	url     string
	err     error
}

func (s *generatedImageObjectStoreStub) Upload(ctx context.Context, upload GeneratedImageObjectUpload) (*GeneratedImageObject, error) {
	copy := GeneratedImageObjectUpload{
		Data:        append([]byte(nil), upload.Data...),
		ContentType: upload.ContentType,
	}
	s.uploads = append(s.uploads, copy)
	if s.err != nil {
		return nil, s.err
	}
	return &GeneratedImageObject{URL: s.url}, nil
}

func TestGeneratedImageRecordNormalizesDataURL(t *testing.T) {
	store := &generatedImageStoreStub{}
	svc := &OpenAIGatewayService{}
	svc.SetGeneratedImageStore(store)

	err := svc.recordGeneratedImage(context.Background(), GeneratedImageRecordInput{
		Meta: GeneratedImageRecordContext{
			UserID:         1,
			APIKeyID:       2,
			AccountID:      3,
			RequestID:      "req_data_url",
			Model:          "gpt-image-2",
			Prompt:         "draw",
			ResponseFormat: "url",
		},
		Value:         "data:image/webp;base64," + base64.StdEncoding.EncodeToString([]byte("webp-bytes")),
		ContentType:   "image/png",
		Source:        "b64_json",
		RevisedPrompt: "draw refined",
	})

	require.NoError(t, err)
	require.Len(t, store.created, 1)
	require.Equal(t, "image/webp", store.created[0].ContentType)
	require.Equal(t, []byte("webp-bytes"), store.created[0].ImageData)
}
