package repository

import (
	"context"
	"strings"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/ent/generatedimage"
	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	"github.com/Wei-Shaw/sub2api/internal/service"

	"entgo.io/ent/dialect/sql"
)

type generatedImageRepository struct {
	client *dbent.Client
}

func NewGeneratedImageRepository(client *dbent.Client) service.GeneratedImageStore {
	return &generatedImageRepository{client: client}
}

func (r *generatedImageRepository) Create(ctx context.Context, image *service.GeneratedImage) (*service.GeneratedImage, error) {
	if image == nil {
		return nil, nil
	}
	create := r.client.GeneratedImage.Create().
		SetProvider(defaultString(image.Provider, service.GeneratedImageProviderOpenAI)).
		SetUserID(image.UserID).
		SetAPIKeyID(image.APIKeyID).
		SetAccountID(image.AccountID).
		SetResponseFormat(defaultString(image.ResponseFormat, "b64_json")).
		SetSource(defaultString(image.Source, service.GeneratedImageSourceB64JSON)).
		SetContentType(defaultString(image.ContentType, "image/png")).
		SetImageData(append([]byte(nil), image.ImageData...)).
		SetSizeBytes(image.SizeBytes)
	if strings.TrimSpace(image.RequestID) != "" {
		create.SetRequestID(strings.TrimSpace(image.RequestID))
	}
	if strings.TrimSpace(image.Model) != "" {
		create.SetModel(strings.TrimSpace(image.Model))
	}
	if strings.TrimSpace(image.Prompt) != "" {
		create.SetPrompt(strings.TrimSpace(image.Prompt))
	}
	if strings.TrimSpace(image.RevisedPrompt) != "" {
		create.SetRevisedPrompt(strings.TrimSpace(image.RevisedPrompt))
	}
	if !image.CreatedAt.IsZero() {
		create.SetCreatedAt(image.CreatedAt)
	}
	created, err := create.Save(ctx)
	if err != nil {
		return nil, err
	}
	return generatedImageFromEnt(created, true), nil
}

func (r *generatedImageRepository) List(ctx context.Context, params pagination.PaginationParams) ([]service.GeneratedImage, *pagination.PaginationResult, error) {
	page := params.Page
	if page < 1 {
		page = 1
	}
	pageSize := params.Limit()

	total, err := r.client.GeneratedImage.Query().Count(ctx)
	if err != nil {
		return nil, nil, err
	}
	if total == 0 {
		return []service.GeneratedImage{}, paginationResultFromTotal(0, pagination.PaginationParams{Page: page, PageSize: pageSize}), nil
	}

	items, err := r.client.GeneratedImage.Query().
		Select(
			generatedimage.FieldID,
			generatedimage.FieldProvider,
			generatedimage.FieldUserID,
			generatedimage.FieldAPIKeyID,
			generatedimage.FieldAccountID,
			generatedimage.FieldRequestID,
			generatedimage.FieldModel,
			generatedimage.FieldPrompt,
			generatedimage.FieldRevisedPrompt,
			generatedimage.FieldResponseFormat,
			generatedimage.FieldSource,
			generatedimage.FieldContentType,
			generatedimage.FieldSizeBytes,
			generatedimage.FieldCreatedAt,
		).
		Order(generatedimage.ByCreatedAt(sql.OrderDesc()), generatedimage.ByID(sql.OrderDesc())).
		Limit(pageSize).
		Offset((page - 1) * pageSize).
		All(ctx)
	if err != nil {
		return nil, nil, err
	}
	out := make([]service.GeneratedImage, 0, len(items))
	for _, item := range items {
		out = append(out, *generatedImageFromEnt(item, false))
	}
	return out, paginationResultFromTotal(int64(total), pagination.PaginationParams{Page: page, PageSize: pageSize}), nil
}

func (r *generatedImageRepository) GetContent(ctx context.Context, id int64) ([]byte, string, error) {
	image, err := r.client.GeneratedImage.Get(ctx, id)
	if err != nil {
		return nil, "", err
	}
	contentType := strings.TrimSpace(image.ContentType)
	if contentType == "" {
		contentType = "application/octet-stream"
	}
	return append([]byte(nil), image.ImageData...), contentType, nil
}

func generatedImageFromEnt(image *dbent.GeneratedImage, includeData bool) *service.GeneratedImage {
	if image == nil {
		return nil
	}
	out := &service.GeneratedImage{
		ID:             image.ID,
		Provider:       image.Provider,
		UserID:         image.UserID,
		APIKeyID:       image.APIKeyID,
		AccountID:      image.AccountID,
		ResponseFormat: image.ResponseFormat,
		Source:         image.Source,
		ContentType:    image.ContentType,
		SizeBytes:      image.SizeBytes,
		CreatedAt:      image.CreatedAt,
	}
	if image.RequestID != nil {
		out.RequestID = *image.RequestID
	}
	if image.Model != nil {
		out.Model = *image.Model
	}
	if image.Prompt != nil {
		out.Prompt = *image.Prompt
	}
	if image.RevisedPrompt != nil {
		out.RevisedPrompt = *image.RevisedPrompt
	}
	if includeData {
		out.ImageData = append([]byte(nil), image.ImageData...)
	}
	return out
}

func defaultString(value string, fallback string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return fallback
	}
	return value
}
