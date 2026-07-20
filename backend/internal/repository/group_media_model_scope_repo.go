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
	exists, err := r.client.Group.Query().Where(group.IDEQ(groupID)).Exist(ctx)
	if err != nil {
		return fmt.Errorf("check group: %w", err)
	}
	if !exists {
		return fmt.Errorf("group %d does not exist", groupID)
	}
	unique := make(map[string]struct{}, len(modelIDs))
	for _, modelID := range modelIDs {
		modelID = strings.ToLower(strings.TrimSpace(modelID))
		if modelID == "" {
			return fmt.Errorf("media model id is empty")
		}
		unique[modelID] = struct{}{}
	}
	definitions := make([]*dbent.MediaModelDefinition, 0, len(unique))
	for modelID := range unique {
		definition, err := r.client.MediaModelDefinition.Query().Where(mediamodeldefinition.ModelIDEQ(modelID), mediamodeldefinition.EnabledEQ(true)).Only(ctx)
		if err != nil {
			return fmt.Errorf("resolve media model %q: %w", modelID, err)
		}
		definitions = append(definitions, definition)
	}
	tx, err := r.client.Tx(ctx)
	if err != nil {
		return err
	}
	if _, err = tx.GroupMediaModelScope.Delete().Where(groupmediamodelscope.GroupIDEQ(groupID)).Exec(ctx); err != nil {
		_ = tx.Rollback()
		return err
	}
	for _, definition := range definitions {
		if err = tx.GroupMediaModelScope.Create().SetGroupID(groupID).SetModelDefinitionID(definition.ID).Exec(ctx); err != nil {
			_ = tx.Rollback()
			return err
		}
	}
	return tx.Commit()
}
