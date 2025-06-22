package postgres

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/sumandas0/entropic/internal/models"
	"github.com/sumandas0/entropic/internal/store"
	"github.com/sumandas0/entropic/pkg/utils"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

type PostgresTx struct {
	tx pgx.Tx
}

func (t *PostgresTx) CreateEntity(ctx context.Context, entity *models.Entity) error {
	propertiesJSON, err := json.Marshal(entity.Properties)
	if err != nil {
		return fmt.Errorf("failed to marshal properties: %w", err)
	}

	query := `
		INSERT INTO entities (id, entity_type, urn, properties, created_at, updated_at, version)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
	`

	_, err = t.tx.Exec(ctx, query,
		entity.ID,
		entity.EntityType,
		entity.URN,
		propertiesJSON,
		entity.CreatedAt,
		entity.UpdatedAt,
		entity.Version,
	)

	if err != nil {
		if isUniqueViolation(err) {
			return utils.NewAppError(utils.CodeAlreadyExists, "entity with URN already exists", err).
				WithDetail("urn", entity.URN)
		}
		return fmt.Errorf("failed to create entity in transaction: %w", err)
	}

	return nil
}

func (t *PostgresTx) UpdateEntity(ctx context.Context, entity *models.Entity) error {
	propertiesJSON, err := json.Marshal(entity.Properties)
	if err != nil {
		return fmt.Errorf("failed to marshal properties: %w", err)
	}

	query := `
		UPDATE entities
		SET properties = $1, updated_at = $2, version = version + 1
		WHERE entity_type = $3 AND id = $4 AND version = $5 AND deleted_at IS NULL
	`

	result, err := t.tx.Exec(ctx, query,
		propertiesJSON,
		time.Now(),
		entity.EntityType,
		entity.ID,
		entity.Version,
	)

	if err != nil {
		return fmt.Errorf("failed to update entity in transaction: %w", err)
	}

	if result.RowsAffected() == 0 {
		return utils.NewAppError(utils.CodeConcurrentModification,
			"entity was modified by another process or does not exist", nil).
			WithDetail("entity_type", entity.EntityType).
			WithDetail("id", entity.ID.String())
	}

	return nil
}

func (t *PostgresTx) DeleteEntity(ctx context.Context, entityType string, id uuid.UUID) error {
	query := `
		UPDATE entities
		SET deleted_at = $1
		WHERE entity_type = $2 AND id = $3 AND deleted_at IS NULL
	`

	result, err := t.tx.Exec(ctx, query, time.Now(), entityType, id)
	if err != nil {
		return fmt.Errorf("failed to delete entity in transaction: %w", err)
	}

	if result.RowsAffected() == 0 {
		return utils.NewAppError(utils.CodeNotFound, "entity not found", nil).
			WithDetail("entity_type", entityType).
			WithDetail("id", id.String())
	}

	return nil
}

func (t *PostgresTx) CreateRelation(ctx context.Context, relation *models.Relation) error {
	propertiesJSON, err := json.Marshal(relation.Properties)
	if err != nil {
		return fmt.Errorf("failed to marshal properties: %w", err)
	}

	query := `
		INSERT INTO relations (
			id, relation_type, from_entity_id, from_entity_type,
			to_entity_id, to_entity_type, properties, created_at, updated_at
		)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
	`

	_, err = t.tx.Exec(ctx, query,
		relation.ID,
		relation.RelationType,
		relation.FromEntityID,
		relation.FromEntityType,
		relation.ToEntityID,
		relation.ToEntityType,
		propertiesJSON,
		relation.CreatedAt,
		relation.UpdatedAt,
	)

	if err != nil {
		return fmt.Errorf("failed to create relation in transaction: %w", err)
	}

	return nil
}

func (t *PostgresTx) DeleteRelation(ctx context.Context, id uuid.UUID) error {
	query := `
		UPDATE relations
		SET deleted_at = $1
		WHERE id = $2 AND deleted_at IS NULL
	`

	result, err := t.tx.Exec(ctx, query, time.Now(), id)
	if err != nil {
		return fmt.Errorf("failed to delete relation in transaction: %w", err)
	}

	if result.RowsAffected() == 0 {
		return utils.NewAppError(utils.CodeNotFound, "relation not found", nil).
			WithDetail("id", id.String())
	}

	return nil
}

func (t *PostgresTx) Commit() error {
	err := t.tx.Commit(context.Background())
	if err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}
	return nil
}

func (t *PostgresTx) Rollback() error {
	err := t.tx.Rollback(context.Background())
	if err != nil {
		
		if err == pgx.ErrTxClosed {
			return nil
		}
		return fmt.Errorf("failed to rollback transaction: %w", err)
	}
	return nil
}

var _ store.Transaction = (*PostgresTx)(nil)