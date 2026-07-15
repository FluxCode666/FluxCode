package repository

import (
	"context"
	"encoding/json"

	dbent "github.com/Wei-Shaw/sub2api/ent"
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
			ID:          entity.ID,
			ModelID:     entity.ModelID,
			MediaType:   service.MediaType(entity.MediaType),
			Operations:  operations,
			Constraints: append(json.RawMessage(nil), entity.Constraints...),
			BillingUnit: entity.BillingUnit,
			Enabled:     entity.Enabled,
			CreatedAt:   entity.CreatedAt,
			UpdatedAt:   entity.UpdatedAt,
		})
	}
	return definitions, nil
}
