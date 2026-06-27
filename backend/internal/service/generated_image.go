package service

import (
	"context"
	"encoding/base64"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/ctxkey"
	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
)

const (
	GeneratedImageProviderOpenAI = "openai"

	GeneratedImageSourceB64JSON     = "b64_json"
	GeneratedImageSourceUpstreamURL = "upstream_url"
	GeneratedImageSourceChatGPTWeb  = "chatgpt_web"
)

type GeneratedImage struct {
	ID             int64
	Provider       string
	UserID         int64
	APIKeyID       int64
	AccountID      int64
	RequestID      string
	Model          string
	Prompt         string
	RevisedPrompt  string
	ResponseFormat string
	Source         string
	ContentType    string
	ImageData      []byte
	SizeBytes      int
	CreatedAt      time.Time
}

type GeneratedImageStore interface {
	Create(ctx context.Context, image *GeneratedImage) (*GeneratedImage, error)
	List(ctx context.Context, params pagination.PaginationParams) ([]GeneratedImage, *pagination.PaginationResult, error)
	GetContent(ctx context.Context, id int64) ([]byte, string, error)
}

type GeneratedImageRecordContext struct {
	UserID         int64
	APIKeyID       int64
	AccountID      int64
	RequestID      string
	Model          string
	Prompt         string
	ResponseFormat string
}

type GeneratedImageRecordInput struct {
	Meta          GeneratedImageRecordContext
	Value         string
	ImageData     []byte
	ContentType   string
	OutputFormat  string
	Source        string
	RevisedPrompt string
}

func (s *OpenAIGatewayService) SetGeneratedImageStore(store GeneratedImageStore) {
	if s == nil {
		return
	}
	s.generatedImageStore = store
}

func (s *OpenAIGatewayService) recordGeneratedImage(ctx context.Context, input GeneratedImageRecordInput) error {
	if s == nil || s.generatedImageStore == nil {
		return nil
	}
	imageBytes := append([]byte(nil), input.ImageData...)
	contentType := strings.TrimSpace(input.ContentType)
	if len(imageBytes) == 0 {
		value := strings.TrimSpace(input.Value)
		if value == "" {
			return nil
		}
		if normalized := normalizeOpenAIImageBase64(value); normalized != "" {
			decoded, err := base64.StdEncoding.DecodeString(normalized)
			if err != nil {
				return err
			}
			imageBytes = decoded
			if detected := dataURLContentType(value); detected != "" {
				contentType = detected
			}
		} else {
			downloaded, downloadedContentType, err := s.downloadOpenAIImagesExternalURL(ctx, nil, value)
			if err != nil {
				return fmt.Errorf("download generated image for archive: %w", err)
			}
			imageBytes = downloaded
			if strings.TrimSpace(downloadedContentType) != "" {
				contentType = strings.TrimSpace(downloadedContentType)
			}
		}
	}
	if len(imageBytes) == 0 {
		return nil
	}
	if contentType == "" {
		contentType = openAIImageOutputMIMEType(input.OutputFormat)
	}
	if contentType == "" {
		contentType = http.DetectContentType(imageBytes)
	}

	record := &GeneratedImage{
		Provider:       GeneratedImageProviderOpenAI,
		UserID:         input.Meta.UserID,
		APIKeyID:       input.Meta.APIKeyID,
		AccountID:      input.Meta.AccountID,
		RequestID:      strings.TrimSpace(input.Meta.RequestID),
		Model:          strings.TrimSpace(input.Meta.Model),
		Prompt:         strings.TrimSpace(input.Meta.Prompt),
		RevisedPrompt:  strings.TrimSpace(input.RevisedPrompt),
		ResponseFormat: strings.TrimSpace(input.Meta.ResponseFormat),
		Source:         strings.TrimSpace(input.Source),
		ContentType:    contentType,
		ImageData:      imageBytes,
		SizeBytes:      len(imageBytes),
	}
	if record.ResponseFormat == "" {
		record.ResponseFormat = "b64_json"
	}
	if record.Source == "" {
		record.Source = GeneratedImageSourceB64JSON
	}
	_, err := s.generatedImageStore.Create(ctx, record)
	return err
}

func (s *OpenAIGatewayService) recordGeneratedImageBestEffort(ctx context.Context, input GeneratedImageRecordInput) {
	if err := s.recordGeneratedImage(ctx, input); err != nil {
		logger.LegacyPrintfContext(ctx, "service.openai_gateway", "[OpenAI] generated image archive failed: %v", err)
	}
}

func normalizeGeneratedImageRecordContext(
	ctx context.Context,
	meta *GeneratedImageRecordContext,
	account *Account,
	parsed *OpenAIImagesRequest,
	model string,
	requestID string,
) GeneratedImageRecordContext {
	out := GeneratedImageRecordContext{}
	if meta != nil {
		out = *meta
	}
	if out.AccountID == 0 && account != nil {
		out.AccountID = account.ID
	}
	if strings.TrimSpace(out.Model) == "" {
		out.Model = strings.TrimSpace(model)
	}
	if parsed != nil {
		if strings.TrimSpace(out.Model) == "" {
			out.Model = strings.TrimSpace(parsed.Model)
		}
		if strings.TrimSpace(out.Prompt) == "" {
			out.Prompt = strings.TrimSpace(parsed.Prompt)
		}
		if strings.TrimSpace(out.ResponseFormat) == "" {
			out.ResponseFormat = strings.TrimSpace(parsed.ResponseFormat)
		}
	}
	if strings.TrimSpace(out.ResponseFormat) == "" {
		out.ResponseFormat = "b64_json"
	}
	if strings.TrimSpace(out.RequestID) == "" {
		out.RequestID = strings.TrimSpace(requestID)
	}
	if strings.TrimSpace(out.RequestID) == "" && ctx != nil {
		if value, _ := ctx.Value(ctxkey.RequestID).(string); strings.TrimSpace(value) != "" {
			out.RequestID = strings.TrimSpace(value)
		}
	}
	return out
}

func firstGeneratedImageRecordContext(values []*GeneratedImageRecordContext) *GeneratedImageRecordContext {
	if len(values) == 0 {
		return nil
	}
	return values[0]
}
