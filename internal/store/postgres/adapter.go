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
	"github.com/jackc/pgx/v5/pgxpool"
)

// PostgresStore implements the PrimaryStore interface for PostgreSQL
type PostgresStore struct {
	pool *pgxpool.Pool
}

// NewPostgresStore creates a new PostgreSQL store
func NewPostgresStore(connectionString string) (*PostgresStore, error) {
	config, err := pgxpool.ParseConfig(connectionString)
	if err != nil {
		return nil, fmt.Errorf("failed to parse connection string: %w", err)
	}

	// Configure connection pool
	config.MaxConns = 25
	config.MinConns = 5
	config.MaxConnLifetime = time.Hour
	config.MaxConnIdleTime = time.Minute * 30

	pool, err := pgxpool.NewWithConfig(context.Background(), config)
	if err != nil {
		return nil, fmt.Errorf("failed to create connection pool: %w", err)
	}

	return &PostgresStore{
		pool: pool,
	}, nil
}

// Entity operations

func (s *PostgresStore) CreateEntity(ctx context.Context, entity *models.Entity) error {
	propertiesJSON, err := json.Marshal(entity.Properties)
	if err != nil {
		return fmt.Errorf("failed to marshal properties: %w", err)
	}

	query := `
		INSERT INTO entities (id, entity_type, urn, properties, created_at, updated_at, version)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
	`

	_, err = s.pool.Exec(ctx, query,
		entity.ID,
		entity.EntityType,
		entity.URN,
		propertiesJSON,
		entity.CreatedAt,
		entity.UpdatedAt,
		entity.Version,
	)

	if err != nil {
		// Check for unique constraint violation
		if isUniqueViolation(err) {
			return utils.NewAppError(utils.CodeAlreadyExists, "entity with URN already exists", err).
				WithDetail("urn", entity.URN)
		}
		return fmt.Errorf("failed to create entity: %w", err)
	}

	return nil
}

func (s *PostgresStore) GetEntity(ctx context.Context, entityType string, id uuid.UUID) (*models.Entity, error) {
	query := `
		SELECT id, entity_type, urn, properties, created_at, updated_at, version
		FROM entities
		WHERE entity_type = $1 AND id = $2 AND deleted_at IS NULL
	`

	var entity models.Entity
	var propertiesJSON []byte

	err := s.pool.QueryRow(ctx, query, entityType, id).Scan(
		&entity.ID,
		&entity.EntityType,
		&entity.URN,
		&propertiesJSON,
		&entity.CreatedAt,
		&entity.UpdatedAt,
		&entity.Version,
	)

	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, utils.NewAppError(utils.CodeNotFound, "entity not found", err).
				WithDetail("entity_type", entityType).
				WithDetail("id", id.String())
		}
		return nil, fmt.Errorf("failed to get entity: %w", err)
	}

	if err := json.Unmarshal(propertiesJSON, &entity.Properties); err != nil {
		return nil, fmt.Errorf("failed to unmarshal properties: %w", err)
	}

	return &entity, nil
}

func (s *PostgresStore) UpdateEntity(ctx context.Context, entity *models.Entity) error {
	propertiesJSON, err := json.Marshal(entity.Properties)
	if err != nil {
		return fmt.Errorf("failed to marshal properties: %w", err)
	}

	query := `
		UPDATE entities
		SET properties = $1, updated_at = $2, version = version + 1
		WHERE entity_type = $3 AND id = $4 AND version = $5 AND deleted_at IS NULL
	`

	result, err := s.pool.Exec(ctx, query,
		propertiesJSON,
		time.Now(),
		entity.EntityType,
		entity.ID,
		entity.Version,
	)

	if err != nil {
		return fmt.Errorf("failed to update entity: %w", err)
	}

	if result.RowsAffected() == 0 {
		return utils.NewAppError(utils.CodeConcurrentModification,
			"entity was modified by another process or does not exist", nil).
			WithDetail("entity_type", entity.EntityType).
			WithDetail("id", entity.ID.String())
	}

	return nil
}

func (s *PostgresStore) DeleteEntity(ctx context.Context, entityType string, id uuid.UUID) error {
	query := `
		UPDATE entities
		SET deleted_at = $1
		WHERE entity_type = $2 AND id = $3 AND deleted_at IS NULL
	`

	result, err := s.pool.Exec(ctx, query, time.Now(), entityType, id)
	if err != nil {
		return fmt.Errorf("failed to delete entity: %w", err)
	}

	if result.RowsAffected() == 0 {
		return utils.NewAppError(utils.CodeNotFound, "entity not found", nil).
			WithDetail("entity_type", entityType).
			WithDetail("id", id.String())
	}

	return nil
}

func (s *PostgresStore) CheckURNExists(ctx context.Context, urn string) (bool, error) {
	query := `
		SELECT EXISTS(
			SELECT 1 FROM entities WHERE urn = $1 AND deleted_at IS NULL
		)
	`

	var exists bool
	err := s.pool.QueryRow(ctx, query, urn).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("failed to check URN existence: %w", err)
	}

	return exists, nil
}

func (s *PostgresStore) ListEntities(ctx context.Context, entityType string, limit, offset int) ([]*models.Entity, error) {
	query := `
		SELECT id, entity_type, urn, properties, created_at, updated_at, version
		FROM entities
		WHERE entity_type = $1 AND deleted_at IS NULL
		ORDER BY created_at DESC
		LIMIT $2 OFFSET $3
	`

	rows, err := s.pool.Query(ctx, query, entityType, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("failed to list entities: %w", err)
	}
	defer rows.Close()

	var entities []*models.Entity
	for rows.Next() {
		var entity models.Entity
		var propertiesJSON []byte

		err := rows.Scan(
			&entity.ID,
			&entity.EntityType,
			&entity.URN,
			&propertiesJSON,
			&entity.CreatedAt,
			&entity.UpdatedAt,
			&entity.Version,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan entity: %w", err)
		}

		if err := json.Unmarshal(propertiesJSON, &entity.Properties); err != nil {
			return nil, fmt.Errorf("failed to unmarshal properties: %w", err)
		}

		entities = append(entities, &entity)
	}

	return entities, nil
}

// Relation operations

func (s *PostgresStore) CreateRelation(ctx context.Context, relation *models.Relation) error {
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

	_, err = s.pool.Exec(ctx, query,
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
		return fmt.Errorf("failed to create relation: %w", err)
	}

	return nil
}

func (s *PostgresStore) GetRelation(ctx context.Context, id uuid.UUID) (*models.Relation, error) {
	query := `
		SELECT id, relation_type, from_entity_id, from_entity_type,
			   to_entity_id, to_entity_type, properties, created_at, updated_at
		FROM relations
		WHERE id = $1 AND deleted_at IS NULL
	`

	var relation models.Relation
	var propertiesJSON []byte

	err := s.pool.QueryRow(ctx, query, id).Scan(
		&relation.ID,
		&relation.RelationType,
		&relation.FromEntityID,
		&relation.FromEntityType,
		&relation.ToEntityID,
		&relation.ToEntityType,
		&propertiesJSON,
		&relation.CreatedAt,
		&relation.UpdatedAt,
	)

	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, utils.NewAppError(utils.CodeNotFound, "relation not found", err).
				WithDetail("id", id.String())
		}
		return nil, fmt.Errorf("failed to get relation: %w", err)
	}

	if len(propertiesJSON) > 0 {
		if err := json.Unmarshal(propertiesJSON, &relation.Properties); err != nil {
			return nil, fmt.Errorf("failed to unmarshal properties: %w", err)
		}
	}

	return &relation, nil
}

func (s *PostgresStore) DeleteRelation(ctx context.Context, id uuid.UUID) error {
	query := `
		UPDATE relations
		SET deleted_at = $1
		WHERE id = $2 AND deleted_at IS NULL
	`

	result, err := s.pool.Exec(ctx, query, time.Now(), id)
	if err != nil {
		return fmt.Errorf("failed to delete relation: %w", err)
	}

	if result.RowsAffected() == 0 {
		return utils.NewAppError(utils.CodeNotFound, "relation not found", nil).
			WithDetail("id", id.String())
	}

	return nil
}

func (s *PostgresStore) GetRelationsByEntity(ctx context.Context, entityID uuid.UUID, relationTypes []string) ([]*models.Relation, error) {
	query := `
		SELECT id, relation_type, from_entity_id, from_entity_type,
			   to_entity_id, to_entity_type, properties, created_at, updated_at
		FROM relations
		WHERE (from_entity_id = $1 OR to_entity_id = $1) AND deleted_at IS NULL
	`

	args := []interface{}{entityID}

	if len(relationTypes) > 0 {
		query += " AND relation_type = ANY($2)"
		args = append(args, relationTypes)
	}

	query += " ORDER BY created_at DESC"

	rows, err := s.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to get relations by entity: %w", err)
	}
	defer rows.Close()

	var relations []*models.Relation
	for rows.Next() {
		var relation models.Relation
		var propertiesJSON []byte

		err := rows.Scan(
			&relation.ID,
			&relation.RelationType,
			&relation.FromEntityID,
			&relation.FromEntityType,
			&relation.ToEntityID,
			&relation.ToEntityType,
			&propertiesJSON,
			&relation.CreatedAt,
			&relation.UpdatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan relation: %w", err)
		}

		if len(propertiesJSON) > 0 {
			if err := json.Unmarshal(propertiesJSON, &relation.Properties); err != nil {
				return nil, fmt.Errorf("failed to unmarshal properties: %w", err)
			}
		}

		relations = append(relations, &relation)
	}

	return relations, nil
}

// Schema operations

func (s *PostgresStore) CreateEntitySchema(ctx context.Context, schema *models.EntitySchema) error {
	propertiesJSON, err := json.Marshal(schema.Properties)
	if err != nil {
		return fmt.Errorf("failed to marshal properties: %w", err)
	}

	indexesJSON, err := json.Marshal(schema.Indexes)
	if err != nil {
		return fmt.Errorf("failed to marshal indexes: %w", err)
	}

	query := `
		INSERT INTO entity_schemas (id, entity_type, properties, indexes, created_at, updated_at, version)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
	`

	_, err = s.pool.Exec(ctx, query,
		schema.ID,
		schema.EntityType,
		propertiesJSON,
		indexesJSON,
		schema.CreatedAt,
		schema.UpdatedAt,
		schema.Version,
	)

	if err != nil {
		if isUniqueViolation(err) {
			return utils.NewAppError(utils.CodeAlreadyExists, "entity schema already exists", err).
				WithDetail("entity_type", schema.EntityType)
		}
		return fmt.Errorf("failed to create entity schema: %w", err)
	}

	return nil
}

func (s *PostgresStore) GetEntitySchema(ctx context.Context, entityType string) (*models.EntitySchema, error) {
	query := `
		SELECT id, entity_type, properties, indexes, created_at, updated_at, version
		FROM entity_schemas
		WHERE entity_type = $1 AND deleted_at IS NULL
	`

	var schema models.EntitySchema
	var propertiesJSON, indexesJSON []byte

	err := s.pool.QueryRow(ctx, query, entityType).Scan(
		&schema.ID,
		&schema.EntityType,
		&propertiesJSON,
		&indexesJSON,
		&schema.CreatedAt,
		&schema.UpdatedAt,
		&schema.Version,
	)

	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, utils.NewAppError(utils.CodeNotFound, "entity schema not found", err).
				WithDetail("entity_type", entityType)
		}
		return nil, fmt.Errorf("failed to get entity schema: %w", err)
	}

	if err := json.Unmarshal(propertiesJSON, &schema.Properties); err != nil {
		return nil, fmt.Errorf("failed to unmarshal properties: %w", err)
	}

	if err := json.Unmarshal(indexesJSON, &schema.Indexes); err != nil {
		return nil, fmt.Errorf("failed to unmarshal indexes: %w", err)
	}

	return &schema, nil
}

func (s *PostgresStore) UpdateEntitySchema(ctx context.Context, schema *models.EntitySchema) error {
	propertiesJSON, err := json.Marshal(schema.Properties)
	if err != nil {
		return fmt.Errorf("failed to marshal properties: %w", err)
	}

	indexesJSON, err := json.Marshal(schema.Indexes)
	if err != nil {
		return fmt.Errorf("failed to marshal indexes: %w", err)
	}

	query := `
		UPDATE entity_schemas
		SET properties = $1, indexes = $2, updated_at = $3, version = version + 1
		WHERE entity_type = $4 AND version = $5 AND deleted_at IS NULL
	`

	result, err := s.pool.Exec(ctx, query,
		propertiesJSON,
		indexesJSON,
		time.Now(),
		schema.EntityType,
		schema.Version,
	)

	if err != nil {
		return fmt.Errorf("failed to update entity schema: %w", err)
	}

	if result.RowsAffected() == 0 {
		return utils.NewAppError(utils.CodeConcurrentModification,
			"schema was modified by another process or does not exist", nil).
			WithDetail("entity_type", schema.EntityType)
	}

	return nil
}

func (s *PostgresStore) DeleteEntitySchema(ctx context.Context, entityType string) error {
	query := `
		UPDATE entity_schemas
		SET deleted_at = $1
		WHERE entity_type = $2 AND deleted_at IS NULL
	`

	result, err := s.pool.Exec(ctx, query, time.Now(), entityType)
	if err != nil {
		return fmt.Errorf("failed to delete entity schema: %w", err)
	}

	if result.RowsAffected() == 0 {
		return utils.NewAppError(utils.CodeNotFound, "entity schema not found", nil).
			WithDetail("entity_type", entityType)
	}

	return nil
}

func (s *PostgresStore) ListEntitySchemas(ctx context.Context) ([]*models.EntitySchema, error) {
	query := `
		SELECT id, entity_type, properties, indexes, created_at, updated_at, version
		FROM entity_schemas
		WHERE deleted_at IS NULL
		ORDER BY entity_type
	`

	rows, err := s.pool.Query(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("failed to list entity schemas: %w", err)
	}
	defer rows.Close()

	var schemas []*models.EntitySchema
	for rows.Next() {
		var schema models.EntitySchema
		var propertiesJSON, indexesJSON []byte

		err := rows.Scan(
			&schema.ID,
			&schema.EntityType,
			&propertiesJSON,
			&indexesJSON,
			&schema.CreatedAt,
			&schema.UpdatedAt,
			&schema.Version,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan entity schema: %w", err)
		}

		if err := json.Unmarshal(propertiesJSON, &schema.Properties); err != nil {
			return nil, fmt.Errorf("failed to unmarshal properties: %w", err)
		}

		if err := json.Unmarshal(indexesJSON, &schema.Indexes); err != nil {
			return nil, fmt.Errorf("failed to unmarshal indexes: %w", err)
		}

		schemas = append(schemas, &schema)
	}

	return schemas, nil
}

// Relationship schema operations

func (s *PostgresStore) CreateRelationshipSchema(ctx context.Context, schema *models.RelationshipSchema) error {
	propertiesJSON, err := json.Marshal(schema.Properties)
	if err != nil {
		return fmt.Errorf("failed to marshal properties: %w", err)
	}

	denormConfigJSON, err := json.Marshal(schema.DenormalizationConfig)
	if err != nil {
		return fmt.Errorf("failed to marshal denormalization config: %w", err)
	}

	query := `
		INSERT INTO relationship_schemas (
			id, relationship_type, from_entity_type, to_entity_type,
			properties, cardinality, denormalization_config,
			created_at, updated_at, version
		)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
	`

	_, err = s.pool.Exec(ctx, query,
		schema.ID,
		schema.RelationshipType,
		schema.FromEntityType,
		schema.ToEntityType,
		propertiesJSON,
		schema.Cardinality,
		denormConfigJSON,
		schema.CreatedAt,
		schema.UpdatedAt,
		schema.Version,
	)

	if err != nil {
		if isUniqueViolation(err) {
			return utils.NewAppError(utils.CodeAlreadyExists, "relationship schema already exists", err).
				WithDetail("relationship_type", schema.RelationshipType)
		}
		return fmt.Errorf("failed to create relationship schema: %w", err)
	}

	return nil
}

func (s *PostgresStore) GetRelationshipSchema(ctx context.Context, relationshipType string) (*models.RelationshipSchema, error) {
	query := `
		SELECT id, relationship_type, from_entity_type, to_entity_type,
			   properties, cardinality, denormalization_config,
			   created_at, updated_at, version
		FROM relationship_schemas
		WHERE relationship_type = $1 AND deleted_at IS NULL
	`

	var schema models.RelationshipSchema
	var propertiesJSON, denormConfigJSON []byte

	err := s.pool.QueryRow(ctx, query, relationshipType).Scan(
		&schema.ID,
		&schema.RelationshipType,
		&schema.FromEntityType,
		&schema.ToEntityType,
		&propertiesJSON,
		&schema.Cardinality,
		&denormConfigJSON,
		&schema.CreatedAt,
		&schema.UpdatedAt,
		&schema.Version,
	)

	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, utils.NewAppError(utils.CodeNotFound, "relationship schema not found", err).
				WithDetail("relationship_type", relationshipType)
		}
		return nil, fmt.Errorf("failed to get relationship schema: %w", err)
	}

	if err := json.Unmarshal(propertiesJSON, &schema.Properties); err != nil {
		return nil, fmt.Errorf("failed to unmarshal properties: %w", err)
	}

	if err := json.Unmarshal(denormConfigJSON, &schema.DenormalizationConfig); err != nil {
		return nil, fmt.Errorf("failed to unmarshal denormalization config: %w", err)
	}

	return &schema, nil
}

func (s *PostgresStore) UpdateRelationshipSchema(ctx context.Context, schema *models.RelationshipSchema) error {
	propertiesJSON, err := json.Marshal(schema.Properties)
	if err != nil {
		return fmt.Errorf("failed to marshal properties: %w", err)
	}

	denormConfigJSON, err := json.Marshal(schema.DenormalizationConfig)
	if err != nil {
		return fmt.Errorf("failed to marshal denormalization config: %w", err)
	}

	query := `
		UPDATE relationship_schemas
		SET properties = $1, cardinality = $2, denormalization_config = $3,
			updated_at = $4, version = version + 1
		WHERE relationship_type = $5 AND version = $6 AND deleted_at IS NULL
	`

	result, err := s.pool.Exec(ctx, query,
		propertiesJSON,
		schema.Cardinality,
		denormConfigJSON,
		time.Now(),
		schema.RelationshipType,
		schema.Version,
	)

	if err != nil {
		return fmt.Errorf("failed to update relationship schema: %w", err)
	}

	if result.RowsAffected() == 0 {
		return utils.NewAppError(utils.CodeConcurrentModification,
			"schema was modified by another process or does not exist", nil).
			WithDetail("relationship_type", schema.RelationshipType)
	}

	return nil
}

func (s *PostgresStore) DeleteRelationshipSchema(ctx context.Context, relationshipType string) error {
	query := `
		UPDATE relationship_schemas
		SET deleted_at = $1
		WHERE relationship_type = $2 AND deleted_at IS NULL
	`

	result, err := s.pool.Exec(ctx, query, time.Now(), relationshipType)
	if err != nil {
		return fmt.Errorf("failed to delete relationship schema: %w", err)
	}

	if result.RowsAffected() == 0 {
		return utils.NewAppError(utils.CodeNotFound, "relationship schema not found", nil).
			WithDetail("relationship_type", relationshipType)
	}

	return nil
}

func (s *PostgresStore) ListRelationshipSchemas(ctx context.Context) ([]*models.RelationshipSchema, error) {
	query := `
		SELECT id, relationship_type, from_entity_type, to_entity_type,
			   properties, cardinality, denormalization_config,
			   created_at, updated_at, version
		FROM relationship_schemas
		WHERE deleted_at IS NULL
		ORDER BY relationship_type
	`

	rows, err := s.pool.Query(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("failed to list relationship schemas: %w", err)
	}
	defer rows.Close()

	var schemas []*models.RelationshipSchema
	for rows.Next() {
		var schema models.RelationshipSchema
		var propertiesJSON, denormConfigJSON []byte

		err := rows.Scan(
			&schema.ID,
			&schema.RelationshipType,
			&schema.FromEntityType,
			&schema.ToEntityType,
			&propertiesJSON,
			&schema.Cardinality,
			&denormConfigJSON,
			&schema.CreatedAt,
			&schema.UpdatedAt,
			&schema.Version,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan relationship schema: %w", err)
		}

		if err := json.Unmarshal(propertiesJSON, &schema.Properties); err != nil {
			return nil, fmt.Errorf("failed to unmarshal properties: %w", err)
		}

		if err := json.Unmarshal(denormConfigJSON, &schema.DenormalizationConfig); err != nil {
			return nil, fmt.Errorf("failed to unmarshal denormalization config: %w", err)
		}

		schemas = append(schemas, &schema)
	}

	return schemas, nil
}

// Transaction support

func (s *PostgresStore) BeginTx(ctx context.Context) (store.Transaction, error) {
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{
		IsoLevel: pgx.Serializable,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to begin transaction: %w", err)
	}

	return &PostgresTx{tx: tx}, nil
}

// Health check and cleanup

func (s *PostgresStore) Ping(ctx context.Context) error {
	return s.pool.Ping(ctx)
}

// GetPool returns the connection pool for migrations
func (s *PostgresStore) GetPool() *pgxpool.Pool {
	return s.pool
}

func (s *PostgresStore) Close() error {
	s.pool.Close()
	return nil
}

// Helper functions

func isUniqueViolation(err error) bool {
	if err == nil {
		return false
	}
	// PostgreSQL unique violation error code is 23505
	return err.Error() == "ERROR: duplicate key value violates unique constraint (SQLSTATE 23505)"
}

// Ensure PostgresStore implements PrimaryStore
var _ store.PrimaryStore = (*PostgresStore)(nil)
