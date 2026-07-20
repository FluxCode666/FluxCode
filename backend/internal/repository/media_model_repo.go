package repository

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/ent/mediamodelalias"
	"github.com/Wei-Shaw/sub2api/ent/mediamodeldefinition"
	"github.com/Wei-Shaw/sub2api/ent/setting"
	"github.com/Wei-Shaw/sub2api/internal/service"
)

const mediaModelNamespaceVersionSettingKey = "internal.media_model_registry_version"

type mediaModelRepository struct {
	client *dbent.Client
}

func NewMediaModelRepository(client *dbent.Client) *mediaModelRepository {
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
		Where(mediamodelalias.HasModelDefinitionWith(mediamodeldefinition.EnabledEQ(true))).
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

func (r *mediaModelRepository) ListAdmin(ctx context.Context) ([]service.MediaModelAdminRecord, error) {
	definitions, err := r.client.MediaModelDefinition.Query().
		Order(dbent.Asc(mediamodeldefinition.FieldID)).
		All(ctx)
	if err != nil {
		return nil, err
	}
	aliases, err := r.client.MediaModelAlias.Query().
		Order(dbent.Asc(mediamodelalias.FieldRequestedModelID)).
		All(ctx)
	if err != nil {
		return nil, err
	}
	aliasesByDefinition := make(map[int64][]string, len(definitions))
	for _, alias := range aliases {
		aliasesByDefinition[alias.ModelDefinitionID] = append(aliasesByDefinition[alias.ModelDefinitionID], alias.RequestedModelID)
	}
	records := make([]service.MediaModelAdminRecord, 0, len(definitions))
	for _, definition := range definitions {
		records = append(records, mediaModelAdminRecordFromEntity(definition, aliasesByDefinition[definition.ID]))
	}
	return records, nil
}

func (r *mediaModelRepository) GetAdminByID(ctx context.Context, id int64) (*service.MediaModelAdminRecord, error) {
	definition, err := r.client.MediaModelDefinition.Get(ctx, id)
	if err != nil {
		if dbent.IsNotFound(err) {
			return nil, service.ErrMediaModelDefinitionNotFound
		}
		return nil, err
	}
	aliases, err := r.listAliasesForDefinition(ctx, id)
	if err != nil {
		return nil, err
	}
	record := mediaModelAdminRecordFromEntity(definition, aliases)
	return &record, nil
}

func (r *mediaModelRepository) CreateAdmin(ctx context.Context, record service.MediaModelAdminRecord) (*service.MediaModelAdminRecord, error) {
	tx, err := r.client.Tx(ctx)
	if err != nil {
		return nil, err
	}
	rollback := func(cause error) (*service.MediaModelAdminRecord, error) {
		if rollbackErr := tx.Rollback(); rollbackErr != nil {
			return nil, fmt.Errorf("%w (rollback: %v)", cause, rollbackErr)
		}
		return nil, cause
	}
	if err := lockMediaModelNamespace(ctx, tx); err != nil {
		return rollback(err)
	}
	if err := ensureMediaModelNamesAvailable(ctx, tx, 0, record.Definition.ModelID, record.Aliases); err != nil {
		return rollback(err)
	}
	definition, err := createMediaModelDefinition(ctx, tx, record.Definition)
	if err != nil {
		return rollback(translateMediaModelWriteError(err))
	}
	if err := replaceMediaModelAliases(ctx, tx, definition.ID, record.Aliases); err != nil {
		return rollback(translateMediaModelWriteError(err))
	}
	created := mediaModelAdminRecordFromEntity(definition, record.Aliases)
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return &created, nil
}

func (r *mediaModelRepository) UpdateAdmin(ctx context.Context, id int64, record service.MediaModelAdminRecord) (*service.MediaModelAdminRecord, error) {
	tx, err := r.client.Tx(ctx)
	if err != nil {
		return nil, err
	}
	rollback := func(cause error) (*service.MediaModelAdminRecord, error) {
		if rollbackErr := tx.Rollback(); rollbackErr != nil {
			return nil, fmt.Errorf("%w (rollback: %v)", cause, rollbackErr)
		}
		return nil, cause
	}
	if err := lockMediaModelNamespace(ctx, tx); err != nil {
		return rollback(err)
	}
	existing, err := tx.MediaModelDefinition.Query().Where(mediamodeldefinition.IDEQ(id)).Only(ctx)
	if err != nil {
		if dbent.IsNotFound(err) {
			return rollback(service.ErrMediaModelDefinitionNotFound)
		}
		return rollback(err)
	}
	if strings.ToLower(strings.TrimSpace(existing.ModelID)) != strings.ToLower(strings.TrimSpace(record.Definition.ModelID)) {
		return rollback(service.ErrMediaModelIDImmutable)
	}
	if err := ensureMediaModelNamesAvailable(ctx, tx, id, record.Definition.ModelID, record.Aliases); err != nil {
		return rollback(err)
	}
	definition, err := tx.MediaModelDefinition.UpdateOneID(id).
		SetVendor(record.Definition.Vendor).
		SetMediaType(string(record.Definition.MediaType)).
		SetOperations(mediaOperationsToStrings(record.Definition.Operations)).
		SetConstraints(append(json.RawMessage(nil), record.Definition.Constraints...)).
		SetBillingUnit(record.Definition.BillingUnit).
		SetDefaultAdapter(record.Definition.DefaultAdapter).
		SetDefaultAsyncMode(string(record.Definition.DefaultAsyncMode)).
		SetEnabled(record.Definition.Enabled).
		Save(ctx)
	if err != nil {
		return rollback(translateMediaModelWriteError(err))
	}
	if _, err := tx.MediaModelAlias.Delete().Where(mediamodelalias.ModelDefinitionIDEQ(id)).Exec(ctx); err != nil {
		return rollback(err)
	}
	if err := replaceMediaModelAliases(ctx, tx, id, record.Aliases); err != nil {
		return rollback(translateMediaModelWriteError(err))
	}
	updated := mediaModelAdminRecordFromEntity(definition, record.Aliases)
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return &updated, nil
}

func (r *mediaModelRepository) DeleteAdmin(ctx context.Context, id int64) error {
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
	if err := lockMediaModelNamespace(ctx, tx); err != nil {
		return rollback(err)
	}
	err = tx.MediaModelDefinition.DeleteOneID(id).Exec(ctx)
	if dbent.IsNotFound(err) {
		return rollback(service.ErrMediaModelDefinitionNotFound)
	}
	if err != nil {
		return rollback(err)
	}
	return tx.Commit()
}

// lockMediaModelNamespace serializes definition and alias mutations through a
// shared database row. The two names live in separate tables, so their own
// unique constraints cannot prevent concurrent cross-table collisions.
func lockMediaModelNamespace(ctx context.Context, tx *dbent.Tx) error {
	now := time.Now().UTC()
	version := strconv.FormatInt(now.UnixNano(), 10)
	return tx.Setting.Create().
		SetKey(mediaModelNamespaceVersionSettingKey).
		SetValue(version).
		SetUpdatedAt(now).
		OnConflictColumns(setting.FieldKey).
		UpdateNewValues().
		Exec(ctx)
}

func (r *mediaModelRepository) listAliasesForDefinition(ctx context.Context, definitionID int64) ([]string, error) {
	entities, err := r.client.MediaModelAlias.Query().
		Where(mediamodelalias.ModelDefinitionIDEQ(definitionID)).
		Order(dbent.Asc(mediamodelalias.FieldRequestedModelID)).
		All(ctx)
	if err != nil {
		return nil, err
	}
	aliases := make([]string, 0, len(entities))
	for _, entity := range entities {
		aliases = append(aliases, entity.RequestedModelID)
	}
	return aliases, nil
}

func ensureMediaModelNamesAvailable(
	ctx context.Context,
	tx *dbent.Tx,
	excludeDefinitionID int64,
	modelID string,
	aliases []string,
) error {
	modelID = strings.ToLower(strings.TrimSpace(modelID))
	definitionQuery := tx.MediaModelDefinition.Query()
	aliasQuery := tx.MediaModelAlias.Query()
	if excludeDefinitionID > 0 {
		definitionQuery = definitionQuery.Where(mediamodeldefinition.IDNEQ(excludeDefinitionID))
		aliasQuery = aliasQuery.Where(mediamodelalias.ModelDefinitionIDNEQ(excludeDefinitionID))
	}
	desiredAliases := make(map[string]struct{}, len(aliases))
	for _, alias := range aliases {
		desiredAliases[strings.ToLower(strings.TrimSpace(alias))] = struct{}{}
	}
	definitions, err := definitionQuery.All(ctx)
	if err != nil {
		return err
	}
	for _, definition := range definitions {
		existingModelID := strings.ToLower(strings.TrimSpace(definition.ModelID))
		if existingModelID == modelID {
			return service.ErrMediaModelIDConflict
		}
		if _, conflict := desiredAliases[existingModelID]; conflict {
			return service.ErrMediaModelAliasConflict
		}
	}
	existingAliases, err := aliasQuery.All(ctx)
	if err != nil {
		return err
	}
	for _, alias := range existingAliases {
		existingAlias := strings.ToLower(strings.TrimSpace(alias.RequestedModelID))
		if existingAlias == modelID {
			return service.ErrMediaModelIDConflict
		}
		if _, conflict := desiredAliases[existingAlias]; conflict {
			return service.ErrMediaModelAliasConflict
		}
	}
	return nil
}

func createMediaModelDefinition(ctx context.Context, tx *dbent.Tx, definition service.MediaModelDefinition) (*dbent.MediaModelDefinition, error) {
	return tx.MediaModelDefinition.Create().
		SetModelID(definition.ModelID).
		SetVendor(definition.Vendor).
		SetMediaType(string(definition.MediaType)).
		SetOperations(mediaOperationsToStrings(definition.Operations)).
		SetConstraints(append(json.RawMessage(nil), definition.Constraints...)).
		SetBillingUnit(definition.BillingUnit).
		SetDefaultAdapter(definition.DefaultAdapter).
		SetDefaultAsyncMode(string(definition.DefaultAsyncMode)).
		SetEnabled(definition.Enabled).
		Save(ctx)
}

func replaceMediaModelAliases(ctx context.Context, tx *dbent.Tx, definitionID int64, aliases []string) error {
	for _, alias := range aliases {
		if _, err := tx.MediaModelAlias.Create().
			SetRequestedModelID(alias).
			SetModelDefinitionID(definitionID).
			Save(ctx); err != nil {
			return err
		}
	}
	return nil
}

func mediaModelAdminRecordFromEntity(entity *dbent.MediaModelDefinition, aliases []string) service.MediaModelAdminRecord {
	operations := make([]service.MediaOperation, len(entity.Operations))
	for index, operation := range entity.Operations {
		operations[index] = service.MediaOperation(operation)
	}
	aliases = append([]string(nil), aliases...)
	if aliases == nil {
		aliases = []string{}
	}
	sort.Strings(aliases)
	return service.MediaModelAdminRecord{
		Definition: service.MediaModelDefinition{
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
		},
		Aliases: aliases,
	}
}

func mediaOperationsToStrings(operations []service.MediaOperation) []string {
	values := make([]string, len(operations))
	for index, operation := range operations {
		values[index] = string(operation)
	}
	return values
}

func translateMediaModelWriteError(err error) error {
	if err == nil {
		return nil
	}
	if dbent.IsConstraintError(err) {
		message := strings.ToLower(err.Error())
		if strings.Contains(message, "requested_model_id") || strings.Contains(message, "media_model_alias") {
			return service.ErrMediaModelAliasConflict
		}
		return service.ErrMediaModelIDConflict
	}
	return err
}
