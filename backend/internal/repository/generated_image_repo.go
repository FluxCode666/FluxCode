package repository

import (
	"context"
	"sort"
	"strings"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/ent/account"
	"github.com/Wei-Shaw/sub2api/ent/apikey"
	"github.com/Wei-Shaw/sub2api/ent/generatedimage"
	"github.com/Wei-Shaw/sub2api/ent/group"
	"github.com/Wei-Shaw/sub2api/ent/predicate"
	"github.com/Wei-Shaw/sub2api/ent/user"
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

func (r *generatedImageRepository) List(ctx context.Context, params service.GeneratedImageListParams) ([]service.GeneratedImage, *pagination.PaginationResult, error) {
	page := params.Page
	if page < 1 {
		page = 1
	}
	pageSize := params.Limit()

	predicates, noMatch, err := r.generatedImageListPredicates(ctx, params)
	if err != nil {
		return nil, nil, err
	}
	emptyPage := pagination.PaginationParams{Page: page, PageSize: pageSize}
	if noMatch {
		return []service.GeneratedImage{}, paginationResultFromTotal(0, emptyPage), nil
	}

	total, err := r.client.GeneratedImage.Query().Where(predicates...).Count(ctx)
	if err != nil {
		return nil, nil, err
	}
	if total == 0 {
		return []service.GeneratedImage{}, paginationResultFromTotal(0, emptyPage), nil
	}

	items, err := r.client.GeneratedImage.Query().
		Where(predicates...).
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
	if err := r.enrichGeneratedImageList(ctx, out); err != nil {
		return nil, nil, err
	}
	return out, paginationResultFromTotal(int64(total), emptyPage), nil
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

func (r *generatedImageRepository) generatedImageListPredicates(ctx context.Context, params service.GeneratedImageListParams) ([]predicate.GeneratedImage, bool, error) {
	predicates := make([]predicate.GeneratedImage, 0, 4)
	if params.StartAt != nil {
		predicates = append(predicates, generatedimage.CreatedAtGTE(*params.StartAt))
	}
	if params.EndAt != nil {
		predicates = append(predicates, generatedimage.CreatedAtLT(*params.EndAt))
	}

	if email := strings.TrimSpace(params.UserEmail); email != "" {
		userIDs, err := r.client.User.Query().
			Where(user.EmailContainsFold(email)).
			IDs(ctx)
		if err != nil {
			return nil, false, err
		}
		if len(userIDs) == 0 {
			return nil, true, nil
		}
		predicates = append(predicates, generatedimage.UserIDIn(userIDs...))
	}

	if params.GroupID > 0 {
		accountIDs, err := r.client.Account.Query().
			Where(account.HasGroupsWith(group.IDEQ(params.GroupID))).
			IDs(ctx)
		if err != nil {
			return nil, false, err
		}
		if len(accountIDs) == 0 {
			return nil, true, nil
		}
		predicates = append(predicates, generatedimage.AccountIDIn(accountIDs...))
	}

	return predicates, false, nil
}

func (r *generatedImageRepository) enrichGeneratedImageList(ctx context.Context, images []service.GeneratedImage) error {
	if len(images) == 0 {
		return nil
	}

	userIDs := make([]int64, 0, len(images))
	apiKeyIDs := make([]int64, 0, len(images))
	accountIDs := make([]int64, 0, len(images))
	seenUsers := map[int64]struct{}{}
	seenAPIKeys := map[int64]struct{}{}
	seenAccounts := map[int64]struct{}{}
	for _, image := range images {
		if image.UserID > 0 {
			if _, ok := seenUsers[image.UserID]; !ok {
				seenUsers[image.UserID] = struct{}{}
				userIDs = append(userIDs, image.UserID)
			}
		}
		if image.APIKeyID > 0 {
			if _, ok := seenAPIKeys[image.APIKeyID]; !ok {
				seenAPIKeys[image.APIKeyID] = struct{}{}
				apiKeyIDs = append(apiKeyIDs, image.APIKeyID)
			}
		}
		if image.AccountID > 0 {
			if _, ok := seenAccounts[image.AccountID]; !ok {
				seenAccounts[image.AccountID] = struct{}{}
				accountIDs = append(accountIDs, image.AccountID)
			}
		}
	}

	userEmails := map[int64]string{}
	if len(userIDs) > 0 {
		users, err := r.client.User.Query().Where(user.IDIn(userIDs...)).All(ctx)
		if err != nil {
			return err
		}
		for _, item := range users {
			userEmails[item.ID] = item.Email
		}
	}

	apiKeyNames := map[int64]string{}
	if len(apiKeyIDs) > 0 {
		apiKeys, err := r.client.APIKey.Query().Where(apikey.IDIn(apiKeyIDs...)).All(ctx)
		if err != nil {
			return err
		}
		for _, item := range apiKeys {
			apiKeyNames[item.ID] = item.Name
		}
	}

	accountNames := map[int64]string{}
	accountGroups := map[int64][]string{}
	if len(accountIDs) > 0 {
		accounts, err := r.client.Account.Query().
			Where(account.IDIn(accountIDs...)).
			WithGroups().
			All(ctx)
		if err != nil {
			return err
		}
		for _, item := range accounts {
			accountNames[item.ID] = item.Name
			names := make([]string, 0, len(item.Edges.Groups))
			for _, group := range item.Edges.Groups {
				if name := strings.TrimSpace(group.Name); name != "" {
					names = append(names, name)
				}
			}
			sort.Strings(names)
			accountGroups[item.ID] = names
		}
	}

	for i := range images {
		images[i].UserEmail = userEmails[images[i].UserID]
		images[i].APIKeyName = apiKeyNames[images[i].APIKeyID]
		images[i].AccountName = accountNames[images[i].AccountID]
		images[i].AccountGroups = append([]string(nil), accountGroups[images[i].AccountID]...)
	}

	return nil
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
