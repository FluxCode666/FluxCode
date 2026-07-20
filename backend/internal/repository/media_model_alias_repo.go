package repository

import (
	"context"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/ent/mediamodelalias"
	"github.com/Wei-Shaw/sub2api/internal/service"
)

type mediaModelAliasRepository struct {
	client *dbent.Client
}

func NewMediaModelAliasRepository(client *dbent.Client) service.MediaModelAliasRepository {
	return &mediaModelAliasRepository{client: client}
}

func (r *mediaModelAliasRepository) ListAll(ctx context.Context) ([]service.MediaModelAlias, error) {
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
