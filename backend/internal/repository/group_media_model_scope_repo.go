package repository

import (
	"context"
	"fmt"
	"sort"
	"strings"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/ent/group"
	"github.com/Wei-Shaw/sub2api/ent/groupmediamodelscope"
	"github.com/Wei-Shaw/sub2api/ent/mediamodeldefinition"
	"github.com/Wei-Shaw/sub2api/internal/service"
)

type groupMediaModelScopeRepository struct{ client *dbent.Client }

func NewGroupMediaModelScopeRepository(client *dbent.Client) service.GroupMediaModelScopeRepository {
	return &groupMediaModelScopeRepository{client: client}
}

func (r *groupMediaModelScopeRepository) ListMediaModelIDs(ctx context.Context, groupID int64) ([]string, error) {
	items, err := r.client.GroupMediaModelScope.Query().
		Where(groupmediamodelscope.GroupIDEQ(groupID)).
		QueryModelDefinition().All(ctx)
	if err != nil {
		return nil, fmt.Errorf("list group media model scopes: %w", err)
	}
	ids := make([]string, 0, len(items))
	for _, item := range items {
		ids = append(ids, strings.ToLower(strings.TrimSpace(item.ModelID)))
	}
	sort.Strings(ids)
	return ids, nil
}

func (r *groupMediaModelScopeRepository) ListEnabledMediaModelIDs(ctx context.Context, groupID int64) ([]string, error) {
	items, err := r.client.GroupMediaModelScope.Query().
		Where(groupmediamodelscope.GroupIDEQ(groupID)).
		QueryModelDefinition().Where(mediamodeldefinition.EnabledEQ(true)).All(ctx)
	if err != nil {
		return nil, fmt.Errorf("list group media model scopes: %w", err)
	}
	ids := make([]string, 0, len(items))
	for _, item := range items {
		ids = append(ids, strings.ToLower(strings.TrimSpace(item.ModelID)))
	}
	sort.Strings(ids)
	return ids, nil
}

func (r *groupMediaModelScopeRepository) ReplaceMediaModelScopes(ctx context.Context, groupID int64, modelIDs []string) error {
	if groupID <= 0 {
		return fmt.Errorf("group id must be positive")
	}
	unique := make(map[string]struct{}, len(modelIDs))
	for _, modelID := range modelIDs {
		modelID = strings.ToLower(strings.TrimSpace(modelID))
		if modelID == "" {
			return fmt.Errorf("media model id is empty")
		}
		unique[modelID] = struct{}{}
	}
	tx, err := r.client.Tx(ctx)
	if err != nil {
		return err
	}
	rollback := func(cause error) error {
		if rollbackErr := tx.Rollback(); rollbackErr != nil {
			return fmt.Errorf("%w (rollback: %v)", cause, rollbackErr)
		}
		return cause
	}
	mediaGroupExists, err := tx.Group.Query().
		Where(group.IDEQ(groupID), group.PlatformEQ(service.PlatformMedia)).
		Exist(ctx)
	if err != nil {
		return rollback(fmt.Errorf("check media group: %w", err))
	}
	if !mediaGroupExists {
		return rollback(service.ErrMediaGroupRequired)
	}
	definitions := make([]*dbent.MediaModelDefinition, 0, len(unique))
	if len(unique) > 0 {
		enabledDefinitions, queryErr := tx.MediaModelDefinition.Query().
			Where(mediamodeldefinition.EnabledEQ(true)).
			All(ctx)
		err = queryErr
		if err != nil {
			return rollback(fmt.Errorf("resolve media models: %w", err))
		}
		resolved := make(map[string]*dbent.MediaModelDefinition, len(enabledDefinitions))
		for _, definition := range enabledDefinitions {
			resolved[strings.ToLower(strings.TrimSpace(definition.ModelID))] = definition
		}
		for modelID := range unique {
			definition, exists := resolved[modelID]
			if !exists {
				return rollback(service.ErrMediaModelScopeModelNotFound)
			}
			definitions = append(definitions, definition)
		}
		if len(definitions) != len(unique) {
			return rollback(service.ErrMediaModelScopeModelNotFound)
		}
		sort.Slice(definitions, func(i, j int) bool { return definitions[i].ModelID < definitions[j].ModelID })
	}
	if _, err = tx.GroupMediaModelScope.Delete().Where(groupmediamodelscope.GroupIDEQ(groupID)).Exec(ctx); err != nil {
		return rollback(err)
	}
	for _, definition := range definitions {
		if err = tx.GroupMediaModelScope.Create().SetGroupID(groupID).SetModelDefinitionID(definition.ID).Exec(ctx); err != nil {
			return rollback(err)
		}
	}
	return tx.Commit()
}
