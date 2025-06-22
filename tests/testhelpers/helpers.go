package testhelpers

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"github.com/sumandas0/entropic/config"
	"github.com/sumandas0/entropic/internal/cache"
	"github.com/sumandas0/entropic/internal/core"
	"github.com/sumandas0/entropic/internal/lock"
	"github.com/sumandas0/entropic/internal/models"
	postgresstore "github.com/sumandas0/entropic/internal/store/postgres"
	"github.com/sumandas0/entropic/internal/store/testutils"
	"github.com/sumandas0/entropic/internal/store/typesense"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/modules/redis"
	"github.com/testcontainers/testcontainers-go/wait"
)

type TestContainers struct {
	PostgresContainer  *postgres.PostgresContainer
	RedisContainer     *redis.RedisContainer
	TypesenseContainer testcontainers.Container
}

type TestEnvironment struct {
	Containers   *TestContainers
	Config       *config.Config
	PrimaryStore *postgresstore.PostgresStore
	IndexStore   *typesense.TypesenseStore
	CacheManager *cache.CacheAwareManager
	LockManager  *lock.LockManager
	Engine       *core.Engine
}

func SetupTestContainers(t *testing.T, ctx context.Context) *TestContainers {
	postgresContainer, err := postgres.Run(ctx,
		"pgvector/pgvector:pg17",
		postgres.WithDatabase("entropic_test"),
		postgres.WithUsername("test"),
		postgres.WithPassword("test"),
		testcontainers.WithWaitStrategy(
			wait.ForLog("database system is ready to accept connections").
				WithOccurrence(2).
				WithStartupTimeout(60*time.Second),
		),
	)
	require.NoError(t, err)

	redisContainer, err := redis.Run(ctx, "redis:7-alpine")
	require.NoError(t, err)

	typesenseReq := testcontainers.ContainerRequest{
		Image:        "typesense/typesense:26.0",
		ExposedPorts: []string{"8108/tcp"},
		Env: map[string]string{
			"TYPESENSE_DATA_DIR": "/data",
			"TYPESENSE_API_KEY":  "test-key",
			"TYPESENSE_ENABLE_CORS": "true",
		},
		Cmd: []string{
			"--data-dir=/data",
			"--api-key=test-key",
			"--enable-cors",
		},
		Tmpfs: map[string]string{
			"/data": "rw",
		},
		WaitingFor: wait.ForHTTP("/health").WithPort("8108/tcp").WithStartupTimeout(60 * time.Second),
	}

	typesenseContainer, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: typesenseReq,
		Started:          true,
		ProviderType:     testcontainers.ProviderDocker,
	})
	require.NoError(t, err)

	return &TestContainers{
		PostgresContainer:  postgresContainer,
		RedisContainer:     redisContainer,
		TypesenseContainer: typesenseContainer,
	}
}

func SetupTestEnvironment(t *testing.T, ctx context.Context) *TestEnvironment {
	containers := SetupTestContainers(t, ctx)

	postgresConnStr, err := containers.PostgresContainer.ConnectionString(ctx, "sslmode=disable")
	require.NoError(t, err)

	_, err = containers.RedisContainer.ConnectionString(ctx)
	require.NoError(t, err)

	typesenseHost, err := containers.TypesenseContainer.Host(ctx)
	require.NoError(t, err)

	typesensePort, err := containers.TypesenseContainer.MappedPort(ctx, "8108")
	require.NoError(t, err)

	typesenseURL := fmt.Sprintf("http://%s:%s", typesenseHost, typesensePort.Port())

	cfg := &config.Config{
		Database: config.DatabaseConfig{
			Host:     "localhost",
			Port:     5432,
			Database: "entropic_test",
			Username: "test",
			Password: "test",
			SSLMode:  "disable",
		},
		Search: config.SearchConfig{
			URL:    typesenseURL,
			APIKey: "test-key",
		},
		Lock: config.LockConfig{
			Type: "redis",
			Redis: config.RedisConfig{
				Host: "localhost",
				Port: 6379,
			},
		},
		Cache: config.CacheConfig{
			TTL:             5 * time.Minute,
			CleanupInterval: time.Minute,
		},
	}

	primaryStore, err := postgresstore.NewPostgresStore(postgresConnStr)
	require.NoError(t, err)

	migrator := postgresstore.NewMigrator(primaryStore.GetPool())
	err = migrator.Run(ctx)
	require.NoError(t, err)

	indexStore, err := typesense.NewTypesenseStore(typesenseURL, "test-key")
	require.NoError(t, err)

	cacheManager := cache.NewCacheAwareManager(primaryStore, cfg.Cache.TTL)

	distributedLock := lock.NewInMemoryDistributedLock()
	lockManager := lock.NewLockManager(distributedLock)

	engine, err := core.NewEngine(primaryStore, indexStore, cacheManager, lockManager)
	require.NoError(t, err)

	return &TestEnvironment{
		Containers:   containers,
		Config:       cfg,
		PrimaryStore: primaryStore,
		IndexStore:   indexStore,
		CacheManager: cacheManager,
		LockManager:  lockManager,
		Engine:       engine,
	}
}

func (tc *TestContainers) Cleanup(ctx context.Context) error {
	var errors []error

	if tc.PostgresContainer != nil {
		if err := tc.PostgresContainer.Terminate(ctx); err != nil {
			errors = append(errors, err)
		}
	}

	if tc.RedisContainer != nil {
		if err := tc.RedisContainer.Terminate(ctx); err != nil {
			errors = append(errors, err)
		}
	}

	if tc.TypesenseContainer != nil {
		if err := tc.TypesenseContainer.Terminate(ctx); err != nil {
			errors = append(errors, err)
		}
	}

	if len(errors) > 0 {
		return fmt.Errorf("cleanup failed with %d errors: %v", len(errors), errors[0])
	}

	return nil
}

func (te *TestEnvironment) Cleanup(ctx context.Context) error {
	var errors []error

	if te.Engine != nil {
		if err := te.Engine.Close(); err != nil {
			errors = append(errors, err)
		}
	}

	if err := te.Containers.Cleanup(ctx); err != nil {
		errors = append(errors, err)
	}

	if len(errors) > 0 {
		return fmt.Errorf("test environment cleanup failed: %v", errors[0])
	}

	return nil
}

func CreateTestEntitySchema(entityType string) *models.EntitySchema {
	properties := models.PropertySchema{
		"name": models.PropertyDefinition{
			Type:     "string",
			Required: true,
		},
		"description": models.PropertyDefinition{
			Type:     "string",
			Required: false,
		},
		"tags": models.PropertyDefinition{
			Type:        "array",
			ElementType: "string",
			Required:    false,
		},
		"metadata": models.PropertyDefinition{
			Type:     "object",
			Required: false,
		},
		"embedding": models.PropertyDefinition{
			Type:      "vector",
			VectorDim: 384,
			Required:  false,
		},
	}

	return models.NewEntitySchema(entityType, properties)
}

func CreateTestRelationshipSchema(relationshipType, fromType, toType string) *models.RelationshipSchema {
	properties := models.PropertySchema{
		"weight": models.PropertyDefinition{
			Type:     "number",
			Required: false,
		},
		"metadata": models.PropertyDefinition{
			Type:     "object",
			Required: false,
		},
	}

	schema := models.NewRelationshipSchema(relationshipType, fromType, toType, models.ManyToMany)
	schema.Properties = properties
	schema.DenormalizationConfig = models.DenormalizationConfig{
		DenormalizeToFrom:   []string{"name"},
		DenormalizeFromTo:   []string{"description"},
		UpdateOnChange:      true,
		IncludeRelationData: false,
	}

	return schema
}

func CreateTestEntity(entityType, name string) *models.Entity {
	properties := map[string]interface{}{
		"name":        name,
		"description": fmt.Sprintf("Test %s entity", name),
		"tags":        []string{"test", "example"},
		"metadata": map[string]interface{}{
			"created_by": "test",
			"test_flag":  true,
		},
	}

	urn := fmt.Sprintf("test:%s:%s", entityType, uuid.New().String())
	return models.NewEntity(entityType, urn, properties)
}

func CreateTestEntityWithEmbedding(entityType, name string, embedding []float32) *models.Entity {
	entity := CreateTestEntity(entityType, name)
	entity.Properties["embedding"] = embedding
	return entity
}

func CreateTestRelation(relationType string, from, to *models.Entity) *models.Relation {
	properties := map[string]interface{}{
		"weight": 1.0,
		"metadata": map[string]interface{}{
			"created_by": "test",
		},
	}

	return models.NewRelation(
		relationType,
		from.ID,
		from.EntityType,
		to.ID,
		to.EntityType,
		properties,
	)
}

func GenerateTestEmbedding(dim int) []float32 {
	embedding := make([]float32, dim)
	for i := 0; i < dim; i++ {
		embedding[i] = float32(i) / float32(dim)
	}
	return embedding
}

func AssertEntityEqual(t *testing.T, expected, actual *models.Entity) {
	require.Equal(t, expected.ID, actual.ID)
	require.Equal(t, expected.EntityType, actual.EntityType)
	require.Equal(t, expected.URN, actual.URN)
	require.Equal(t, expected.Properties, actual.Properties)
	require.Equal(t, expected.Version, actual.Version)
}

func AssertRelationEqual(t *testing.T, expected, actual *models.Relation) {
	require.Equal(t, expected.ID, actual.ID)
	require.Equal(t, expected.RelationType, actual.RelationType)
	require.Equal(t, expected.FromEntityID, actual.FromEntityID)
	require.Equal(t, expected.FromEntityType, actual.FromEntityType)
	require.Equal(t, expected.ToEntityID, actual.ToEntityID)
	require.Equal(t, expected.ToEntityType, actual.ToEntityType)
	require.Equal(t, expected.Properties, actual.Properties)
}

func WaitForIndexing(t *testing.T, ctx context.Context, indexStore *typesense.TypesenseStore, timeout time.Duration) {

	time.Sleep(100 * time.Millisecond)
}

func RandomString(length int) string {
	return fmt.Sprintf("test_%s", uuid.New().String()[:length])
}

func SetupPostgresWithDockertest(t *testing.T) (string, func()) {
	container, err := testutils.SetupTestPostgres()
	require.NoError(t, err)

	cleanup := func() {
		if err := container.Cleanup(); err != nil {
			t.Logf("Failed to cleanup postgres container: %v", err)
		}
	}

	return container.URL, cleanup
}

func SetupTypesenseWithDockertest(t *testing.T) (string, string, func()) {
	container, err := testutils.SetupTestTypesense()
	require.NoError(t, err)

	cleanup := func() {
		if err := container.Cleanup(); err != nil {
			t.Logf("Failed to cleanup typesense container: %v", err)
		}
	}

	return container.URL, container.APIKey, cleanup
}
