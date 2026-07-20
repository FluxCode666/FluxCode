package repository

import (
	"context"
	"encoding/json"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/ent/mediamodelalias"
	"github.com/Wei-Shaw/sub2api/ent/mediamodeldefinition"
	"github.com/Wei-Shaw/sub2api/internal/service"
)

type mediaModelRepository struct {
	client *dbent.Client
}

func NewMediaModelRepository(client *dbent.Client) service.MediaModelDefinitionRepository {
	return &mediaModelRepository{client: client}
}

func (r *mediaModelRepository) ListEnabled(ctx context.Context) ([]service.MediaModelDefinition, error) {
	entities, err := r.client.MediaModelDefinition.Query().
		Where(mediamodeldefinition.EnabledEQ(true)).
		Order(dbent.Asc(mediamodeldefinition.FieldID)).
		All(ctx)
	if err != nil {
		return nil, err
	}
	definitions := make([]service.MediaModelDefinition, 0, len(entities))
	for _, entity := range entities {
		operations := make([]service.MediaOperation, len(entity.Operations))
		for i, operation := range entity.Operations {
			operations[i] = service.MediaOperation(operation)
		}
		definitions = append(definitions, service.MediaModelDefinition{
			ID:               entity.ID,
			ModelID:          entity.ModelID,
			Vendor:           entity.Vendor,
			MediaType:        service.MediaType(entity.MediaType),
			Operations:       operations,
			Constraints:      append(json.RawMessage(nil), entity.Constraints...),
			BillingUnit:      entity.BillingUnit,
			DefaultAdapter:   entity.DefaultAdapter,
			DefaultAsyncMode: service.NativeAsyncMode(entity.DefaultAsyncMode),
			Enabled:          entity.Enabled,
			CreatedAt:        entity.CreatedAt,
			UpdatedAt:        entity.UpdatedAt,
		})
	}
	return definitions, nil
}

// ListAll implements service.MediaModelAliasRepository in addition to the
// model-definition repository. This allows the one-argument registry
// constructor used by the production composition root to discover aliases.
func (r *mediaModelRepository) ListAll(ctx context.Context) ([]service.MediaModelAlias, error) {
	entities, err := r.client.MediaModelAlias.Query().
		Order(dbent.Asc(mediamodelalias.FieldID)).
		All(ctx)
	if err != nil {
		return nil, err
	}
	aliases := make([]service.MediaModelAlias, 0, len(entities))
	for _, entity := range entities {
		aliases = append(aliases, service.MediaModelAlias{
			RequestedModelID:  entity.RequestedModelID,
			ModelDefinitionID: entity.ModelDefinitionID,
		})
	}
	return aliases, nil
}
