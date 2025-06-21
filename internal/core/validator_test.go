package core

import (
	"context"
	"testing"

	"github.com/entropic/entropic/internal/models"
	"github.com/entropic/entropic/tests/testhelpers"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestValidator_ValidateEntity(t *testing.T) {
	ctx := context.Background()
	env := testhelpers.SetupTestEnvironment(t, ctx)
	defer env.Cleanup(ctx)

	validator := NewValidator(env.PrimaryStore, env.CacheManager)

	// Create test schema
	schema := testhelpers.CreateTestEntitySchema("user")
	err := env.Engine.CreateEntitySchema(ctx, schema)
	require.NoError(t, err)

	tests := []struct {
		name      string
		entity    *models.Entity
		wantError bool
		errorMsg  string
	}{
		{
			name:      "valid entity",
			entity:    testhelpers.CreateTestEntity("user", "test-user"),
			wantError: false,
		},
		{
			name: "missing required property",
			entity: &models.Entity{
				ID:         uuid.New(),
				EntityType: "user",
				URN:        "test:user:missing-name",
				Properties: map[string]interface{}{
					"description": "User without name",
				},
			},
			wantError: true,
			errorMsg:  "required property 'name' is missing",
		},
		{
			name: "invalid property type",
			entity: &models.Entity{
				ID:         uuid.New(),
				EntityType: "user",
				URN:        "test:user:invalid-type",
				Properties: map[string]interface{}{
					"name": 123, // Should be string
				},
			},
			wantError: true,
			errorMsg:  "property 'name' must be of type string",
		},
		{
			name: "invalid array element type",
			entity: &models.Entity{
				ID:         uuid.New(),
				EntityType: "user",
				URN:        "test:user:invalid-array",
				Properties: map[string]interface{}{
					"name": "Test User",
					"tags": []interface{}{123, "string"}, // Should be all strings
				},
			},
			wantError: true,
			errorMsg:  "array element at index 0 in property 'tags' must be of type string",
		},
		{
			name: "invalid vector dimension",
			entity: &models.Entity{
				ID:         uuid.New(),
				EntityType: "user",
				URN:        "test:user:invalid-vector",
				Properties: map[string]interface{}{
					"name":      "Test User",
					"embedding": []float32{1.0, 2.0}, // Should be 384 dimensions
				},
			},
			wantError: true,
			errorMsg:  "vector property 'embedding' must have exactly 384 dimensions",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validator.ValidateEntity(ctx, tt.entity)
			if tt.wantError {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.errorMsg)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestValidator_ValidateRelation(t *testing.T) {
	ctx := context.Background()
	env := testhelpers.SetupTestEnvironment(t, ctx)
	defer env.Cleanup(ctx)

	validator := NewValidator(env.PrimaryStore, env.CacheManager)

	// Create test schemas
	userSchema := testhelpers.CreateTestEntitySchema("user")
	err := env.Engine.CreateEntitySchema(ctx, userSchema)
	require.NoError(t, err)

	orgSchema := testhelpers.CreateTestEntitySchema("organization")
	err = env.Engine.CreateEntitySchema(ctx, orgSchema)
	require.NoError(t, err)

	relationSchema := testhelpers.CreateTestRelationshipSchema("member_of", "user", "organization")
	err = env.Engine.CreateRelationshipSchema(ctx, relationSchema)
	require.NoError(t, err)

	// Create test entities
	user := testhelpers.CreateTestEntity("user", "test-user")
	err = env.Engine.CreateEntity(ctx, user)
	require.NoError(t, err)

	org := testhelpers.CreateTestEntity("organization", "test-org")
	err = env.Engine.CreateEntity(ctx, org)
	require.NoError(t, err)

	tests := []struct {
		name      string
		relation  *models.Relation
		wantError bool
		errorMsg  string
	}{
		{
			name:      "valid relation",
			relation:  testhelpers.CreateTestRelation("member_of", user, org),
			wantError: false,
		},
		{
			name: "invalid relation type",
			relation: &models.Relation{
				ID:             uuid.New(),
				RelationType:   "invalid_relation",
				FromEntityID:   user.ID,
				FromEntityType: user.EntityType,
				ToEntityID:     org.ID,
				ToEntityType:   org.EntityType,
				Properties:     map[string]interface{}{},
			},
			wantError: true,
			errorMsg:  "relationship schema not found",
		},
		{
			name: "mismatched from entity type",
			relation: &models.Relation{
				ID:             uuid.New(),
				RelationType:   "member_of",
				FromEntityID:   user.ID,
				FromEntityType: "organization", // Wrong type
				ToEntityID:     org.ID,
				ToEntityType:   org.EntityType,
				Properties:     map[string]interface{}{},
			},
			wantError: true,
			errorMsg:  "from entity type mismatch",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validator.ValidateRelation(ctx, tt.relation)
			if tt.wantError {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.errorMsg)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestValidator_ValidateURNUniqueness(t *testing.T) {
	ctx := context.Background()
	env := testhelpers.SetupTestEnvironment(t, ctx)
	defer env.Cleanup(ctx)

	validator := NewValidator(env.PrimaryStore, env.CacheManager)

	// Create test schema
	schema := testhelpers.CreateTestEntitySchema("user")
	err := env.Engine.CreateEntitySchema(ctx, schema)
	require.NoError(t, err)

	// Create first entity
	entity1 := testhelpers.CreateTestEntity("user", "test-user")
	entity1.URN = "test:user:unique-urn"
	err = env.Engine.CreateEntity(ctx, entity1)
	require.NoError(t, err)

	// Test duplicate URN
	entity2 := testhelpers.CreateTestEntity("user", "another-user")
	entity2.URN = "test:user:unique-urn" // Same URN

	err = validator.ValidateURNUniqueness(ctx, entity2)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "URN already exists")

	// Test unique URN
	entity3 := testhelpers.CreateTestEntity("user", "third-user")
	entity3.URN = "test:user:different-urn"

	err = validator.ValidateURNUniqueness(ctx, entity3)
	assert.NoError(t, err)
}

func TestValidator_ValidateEntityUpdate(t *testing.T) {
	ctx := context.Background()
	env := testhelpers.SetupTestEnvironment(t, ctx)
	defer env.Cleanup(ctx)

	validator := NewValidator(env.PrimaryStore, env.CacheManager)

	// Create test schema
	schema := testhelpers.CreateTestEntitySchema("user")
	err := env.Engine.CreateEntitySchema(ctx, schema)
	require.NoError(t, err)

	// Create original entity
	original := testhelpers.CreateTestEntity("user", "test-user")
	err = env.Engine.CreateEntity(ctx, original)
	require.NoError(t, err)

	tests := []struct {
		name      string
		updated   *models.Entity
		wantError bool
		errorMsg  string
	}{
		{
			name: "valid update",
			updated: &models.Entity{
				ID:         original.ID,
				EntityType: original.EntityType,
				URN:        original.URN,
				Properties: map[string]interface{}{
					"name":        "Updated Name",
					"description": "Updated description",
				},
				Version: original.Version,
			},
			wantError: false,
		},
		{
			name: "cannot change entity type",
			updated: &models.Entity{
				ID:         original.ID,
				EntityType: "organization", // Different type
				URN:        original.URN,
				Properties: original.Properties,
				Version:    original.Version,
			},
			wantError: true,
			errorMsg:  "entity type cannot be changed",
		},
		{
			name: "cannot change URN",
			updated: &models.Entity{
				ID:         original.ID,
				EntityType: original.EntityType,
				URN:        "test:user:different-urn", // Different URN
				Properties: original.Properties,
				Version:    original.Version,
			},
			wantError: true,
			errorMsg:  "URN cannot be changed",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validator.ValidateEntityUpdate(ctx, original, tt.updated)
			if tt.wantError {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.errorMsg)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestValidator_ValidateSchema(t *testing.T) {
	validator := &Validator{}

	tests := []struct {
		name      string
		schema    *models.EntitySchema
		wantError bool
		errorMsg  string
	}{
		{
			name:      "valid schema",
			schema:    testhelpers.CreateTestEntitySchema("user"),
			wantError: false,
		},
		{
			name: "invalid property type",
			schema: &models.EntitySchema{
				EntityType: "test",
				Properties: models.PropertySchema{
					"invalid": models.PropertyDefinition{
						Type:     "invalid_type",
						Required: true,
					},
				},
			},
			wantError: true,
			errorMsg:  "invalid property type",
		},
		{
			name: "vector without dimension",
			schema: &models.EntitySchema{
				EntityType: "test",
				Properties: models.PropertySchema{
					"embedding": models.PropertyDefinition{
						Type:     "vector",
						Required: false,
						// Missing VectorDim
					},
				},
			},
			wantError: true,
			errorMsg:  "vector property must specify dimension",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validator.ValidateSchema(tt.schema)
			if tt.wantError {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.errorMsg)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func BenchmarkValidator_ValidateEntity(b *testing.B) {
	ctx := context.Background()
	env := testhelpers.SetupTestEnvironment(&testing.T{}, ctx)
	defer env.Cleanup(ctx)

	validator := NewValidator(env.PrimaryStore, env.CacheManager)

	// Create test schema
	schema := testhelpers.CreateTestEntitySchema("user")
	env.Engine.CreateEntitySchema(ctx, schema)

	entity := testhelpers.CreateTestEntity("user", "benchmark-user")

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		validator.ValidateEntity(ctx, entity)
	}
}