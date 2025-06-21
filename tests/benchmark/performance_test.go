package benchmark

import (
	"context"
	"fmt"
	"math/rand"
	"sync"
	"testing"
	"time"

	"github.com/sumandas0/entropic/internal/models"
	"github.com/sumandas0/entropic/tests/testhelpers"
)

func BenchmarkEntityCreation(b *testing.B) {
	ctx := context.Background()
	env := testhelpers.SetupTestEnvironment(&testing.T{}, ctx)
	defer env.Cleanup(ctx)

	// Setup schema
	schema := testhelpers.CreateTestEntitySchema("user")
	env.Engine.CreateEntitySchema(ctx, schema)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		entity := testhelpers.CreateTestEntity("user", fmt.Sprintf("user-%d", i))
		entity.URN = fmt.Sprintf("benchmark:user:%d", i)
		env.Engine.CreateEntity(ctx, entity)
	}
}

func BenchmarkEntityCreationConcurrent(b *testing.B) {
	ctx := context.Background()
	env := testhelpers.SetupTestEnvironment(&testing.T{}, ctx)
	defer env.Cleanup(ctx)

	// Setup schema
	schema := testhelpers.CreateTestEntitySchema("user")
	env.Engine.CreateEntitySchema(ctx, schema)

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			entity := testhelpers.CreateTestEntity("user", fmt.Sprintf("user-%d", i))
			entity.URN = fmt.Sprintf("benchmark:user:concurrent:%d-%d", time.Now().UnixNano(), i)
			env.Engine.CreateEntity(ctx, entity)
			i++
		}
	})
}

func BenchmarkEntityRetrieval(b *testing.B) {
	ctx := context.Background()
	env := testhelpers.SetupTestEnvironment(&testing.T{}, ctx)
	defer env.Cleanup(ctx)

	// Setup schema and entities
	schema := testhelpers.CreateTestEntitySchema("user")
	env.Engine.CreateEntitySchema(ctx, schema)

	// Create test entities
	entities := make([]*models.Entity, 100)
	for i := 0; i < 100; i++ {
		entity := testhelpers.CreateTestEntity("user", fmt.Sprintf("user-%d", i))
		entity.URN = fmt.Sprintf("benchmark:user:retrieve:%d", i)
		env.Engine.CreateEntity(ctx, entity)
		entities[i] = entity
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		// Randomly select an entity to retrieve
		entity := entities[rand.Intn(len(entities))]
		env.Engine.GetEntity(ctx, entity.EntityType, entity.ID)
	}
}

func BenchmarkEntityUpdate(b *testing.B) {
	ctx := context.Background()
	env := testhelpers.SetupTestEnvironment(&testing.T{}, ctx)
	defer env.Cleanup(ctx)

	// Setup schema and entity
	schema := testhelpers.CreateTestEntitySchema("user")
	env.Engine.CreateEntitySchema(ctx, schema)

	entity := testhelpers.CreateTestEntity("user", "benchmark-user")
	env.Engine.CreateEntity(ctx, entity)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		// Get current entity
		current, _ := env.Engine.GetEntity(ctx, entity.EntityType, entity.ID)

		// Update properties
		updated := &models.Entity{
			ID:         current.ID,
			EntityType: current.EntityType,
			URN:        current.URN,
			Properties: map[string]interface{}{
				"name":        fmt.Sprintf("updated-user-%d", i),
				"description": fmt.Sprintf("Updated description %d", i),
				"metadata": map[string]interface{}{
					"update_count": i,
					"timestamp":    time.Now().Unix(),
				},
			},
			Version: current.Version,
		}

		env.Engine.UpdateEntity(ctx, updated)
	}
}

func BenchmarkTextSearch(b *testing.B) {
	ctx := context.Background()
	env := testhelpers.SetupTestEnvironment(&testing.T{}, ctx)
	defer env.Cleanup(ctx)

	// Setup schema and entities
	schema := testhelpers.CreateTestEntitySchema("document")
	env.Engine.CreateEntitySchema(ctx, schema)

	// Create test entities
	searchTerms := []string{"technology", "science", "business", "health", "education"}
	for i := 0; i < 100; i++ {
		term := searchTerms[i%len(searchTerms)]
		entity := testhelpers.CreateTestEntity("document", fmt.Sprintf("%s-doc-%d", term, i))
		entity.URN = fmt.Sprintf("benchmark:document:%s:%d", term, i)
		entity.Properties["description"] = fmt.Sprintf("This is a %s document about %s topics", term, term)
		env.Engine.CreateEntity(ctx, entity)
	}

	// Wait for indexing
	time.Sleep(2 * time.Second)

	query := &models.SearchQuery{
		EntityTypes: []string{"document"},
		Query:       "technology",
		Limit:       10,
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		env.Engine.Search(ctx, query)
	}
}

func BenchmarkVectorSearch(b *testing.B) {
	ctx := context.Background()
	env := testhelpers.SetupTestEnvironment(&testing.T{}, ctx)
	defer env.Cleanup(ctx)

	// Setup schema and entities
	schema := testhelpers.CreateTestEntitySchema("document")
	env.Engine.CreateEntitySchema(ctx, schema)

	// Create test entities with embeddings
	for i := 0; i < 50; i++ {
		embedding := testhelpers.GenerateTestEmbedding(384)
		entity := testhelpers.CreateTestEntityWithEmbedding("document", fmt.Sprintf("doc-%d", i), embedding)
		entity.URN = fmt.Sprintf("benchmark:document:vector:%d", i)
		env.Engine.CreateEntity(ctx, entity)
	}

	// Wait for indexing
	time.Sleep(3 * time.Second)

	queryEmbedding := testhelpers.GenerateTestEmbedding(384)
	vectorQuery := &models.VectorQuery{
		EntityTypes: []string{"document"},
		Vector:      queryEmbedding,
		VectorField: "embedding",
		TopK:        5,
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		env.Engine.VectorSearch(ctx, vectorQuery)
	}
}

func BenchmarkRelationCreation(b *testing.B) {
	ctx := context.Background()
	env := testhelpers.SetupTestEnvironment(&testing.T{}, ctx)
	defer env.Cleanup(ctx)

	// Setup schemas
	userSchema := testhelpers.CreateTestEntitySchema("user")
	env.Engine.CreateEntitySchema(ctx, userSchema)

	orgSchema := testhelpers.CreateTestEntitySchema("organization")
	env.Engine.CreateEntitySchema(ctx, orgSchema)

	relationSchema := testhelpers.CreateTestRelationshipSchema("member_of", "user", "organization")
	env.Engine.CreateRelationshipSchema(ctx, relationSchema)

	// Create entities
	users := make([]*models.Entity, 10)
	orgs := make([]*models.Entity, 5)

	for i := 0; i < 10; i++ {
		user := testhelpers.CreateTestEntity("user", fmt.Sprintf("user-%d", i))
		user.URN = fmt.Sprintf("benchmark:user:relation:%d", i)
		env.Engine.CreateEntity(ctx, user)
		users[i] = user
	}

	for i := 0; i < 5; i++ {
		org := testhelpers.CreateTestEntity("organization", fmt.Sprintf("org-%d", i))
		org.URN = fmt.Sprintf("benchmark:org:relation:%d", i)
		env.Engine.CreateEntity(ctx, org)
		orgs[i] = org
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		user := users[rand.Intn(len(users))]
		org := orgs[rand.Intn(len(orgs))]

		relation := testhelpers.CreateTestRelation("member_of", user, org)
		env.Engine.CreateRelation(ctx, relation)
	}
}

func BenchmarkCacheHitRate(b *testing.B) {
	ctx := context.Background()
	env := testhelpers.SetupTestEnvironment(&testing.T{}, ctx)
	defer env.Cleanup(ctx)

	// Setup schemas
	schemas := make([]string, 10)
	for i := 0; i < 10; i++ {
		entityType := fmt.Sprintf("type-%d", i)
		schema := testhelpers.CreateTestEntitySchema(entityType)
		env.Engine.CreateEntitySchema(ctx, schema)
		schemas[i] = entityType
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		// Randomly access schemas to test cache performance
		entityType := schemas[rand.Intn(len(schemas))]
		env.CacheManager.GetEntitySchema(ctx, entityType)
	}
}

func BenchmarkLockContention(b *testing.B) {
	ctx := context.Background()
	env := testhelpers.SetupTestEnvironment(&testing.T{}, ctx)
	defer env.Cleanup(ctx)

	numResources := 10
	resources := make([]string, numResources)
	for i := 0; i < numResources; i++ {
		resources[i] = fmt.Sprintf("resource-%d", i)
	}

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			resource := resources[rand.Intn(len(resources))]

			err := env.LockManager.Lock(ctx, resource, 100*time.Millisecond)
			if err == nil {
				// Simulate some work
				time.Sleep(1 * time.Millisecond)
				env.LockManager.Unlock(ctx, resource)
			}
		}
	})
}

func BenchmarkTwoPhaseCommit(b *testing.B) {
	ctx := context.Background()
	env := testhelpers.SetupTestEnvironment(&testing.T{}, ctx)
	defer env.Cleanup(ctx)

	// Setup schema
	schema := testhelpers.CreateTestEntitySchema("user")
	env.Engine.CreateEntitySchema(ctx, schema)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		entity := testhelpers.CreateTestEntity("user", fmt.Sprintf("user-%d", i))
		entity.URN = fmt.Sprintf("benchmark:2pc:user:%d", i)

		// This tests the full two-phase commit flow
		env.Engine.CreateEntity(ctx, entity)
	}
}

func BenchmarkHighVolumeOperations(b *testing.B) {
	ctx := context.Background()
	env := testhelpers.SetupTestEnvironment(&testing.T{}, ctx)
	defer env.Cleanup(ctx)

	// Setup schema
	schema := testhelpers.CreateTestEntitySchema("event")
	env.Engine.CreateEntitySchema(ctx, schema)

	// Test high-volume concurrent operations
	b.ResetTimer()

	var wg sync.WaitGroup
	numWorkers := 10
	operationsPerWorker := b.N / numWorkers

	for w := 0; w < numWorkers; w++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()

			for i := 0; i < operationsPerWorker; i++ {
				entity := testhelpers.CreateTestEntity("event", fmt.Sprintf("event-%d-%d", workerID, i))
				entity.URN = fmt.Sprintf("benchmark:volume:event:%d:%d", workerID, i)
				entity.Properties["timestamp"] = time.Now().Unix()
				entity.Properties["worker_id"] = workerID
				entity.Properties["sequence"] = i

				env.Engine.CreateEntity(ctx, entity)
			}
		}(w)
	}

	wg.Wait()
}

func BenchmarkMemoryUsage(b *testing.B) {
	ctx := context.Background()
	env := testhelpers.SetupTestEnvironment(&testing.T{}, ctx)
	defer env.Cleanup(ctx)

	// Setup schema
	schema := testhelpers.CreateTestEntitySchema("large_entity")
	env.Engine.CreateEntitySchema(ctx, schema)

	// Create entities with large property sets
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		entity := testhelpers.CreateTestEntity("large_entity", fmt.Sprintf("large-%d", i))
		entity.URN = fmt.Sprintf("benchmark:memory:large:%d", i)

		// Add large properties to test memory usage
		largeData := make(map[string]interface{})
		for j := 0; j < 100; j++ {
			largeData[fmt.Sprintf("field_%d", j)] = fmt.Sprintf("value_%d_%s", j, testhelpers.RandomString(50))
		}
		entity.Properties["large_data"] = largeData

		env.Engine.CreateEntity(ctx, entity)
	}
}

// LoadTest simulates realistic load patterns
func BenchmarkRealisticLoad(b *testing.B) {
	ctx := context.Background()
	env := testhelpers.SetupTestEnvironment(&testing.T{}, ctx)
	defer env.Cleanup(ctx)

	// Setup multiple schemas
	userSchema := testhelpers.CreateTestEntitySchema("user")
	env.Engine.CreateEntitySchema(ctx, userSchema)

	productSchema := testhelpers.CreateTestEntitySchema("product")
	env.Engine.CreateEntitySchema(ctx, productSchema)

	orderSchema := testhelpers.CreateTestEntitySchema("order")
	env.Engine.CreateEntitySchema(ctx, orderSchema)

	// Pre-create some entities for read operations
	for i := 0; i < 50; i++ {
		user := testhelpers.CreateTestEntity("user", fmt.Sprintf("user-%d", i))
		user.URN = fmt.Sprintf("load:user:%d", i)
		env.Engine.CreateEntity(ctx, user)

		product := testhelpers.CreateTestEntity("product", fmt.Sprintf("product-%d", i))
		product.URN = fmt.Sprintf("load:product:%d", i)
		env.Engine.CreateEntity(ctx, product)
	}

	// Wait for indexing
	time.Sleep(2 * time.Second)

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		operationCount := 0
		for pb.Next() {
			operation := operationCount % 10

			switch operation {
			case 0, 1, 2: // 30% reads
				entityTypes := []string{"user", "product"}
				query := &models.SearchQuery{
					EntityTypes: entityTypes,
					Query:       fmt.Sprintf("user-%d", rand.Intn(50)),
					Limit:       5,
				}
				env.Engine.Search(ctx, query)

			case 3, 4: // 20% writes
				entity := testhelpers.CreateTestEntity("order", fmt.Sprintf("order-%d", operationCount))
				entity.URN = fmt.Sprintf("load:order:%d:%d", time.Now().UnixNano(), operationCount)
				env.Engine.CreateEntity(ctx, entity)

			case 5: // 10% updates
				// Update existing entity (simplified)
				entity := testhelpers.CreateTestEntity("user", fmt.Sprintf("updated-user-%d", operationCount))
				entity.URN = fmt.Sprintf("load:user:update:%d", operationCount)
				env.Engine.CreateEntity(ctx, entity)

			case 6, 7: // 20% cache operations
				env.CacheManager.GetEntitySchema(ctx, "user")
				env.CacheManager.GetEntitySchema(ctx, "product")

			case 8, 9: // 20% vector operations (if supported)
				embedding := testhelpers.GenerateTestEmbedding(384)
				entity := testhelpers.CreateTestEntityWithEmbedding("product", fmt.Sprintf("vector-product-%d", operationCount), embedding)
				entity.URN = fmt.Sprintf("load:product:vector:%d", operationCount)
				env.Engine.CreateEntity(ctx, entity)
			}

			operationCount++
		}
	})
}

// Helper to measure throughput
func measureThroughput(b *testing.B, operation func()) {
	start := time.Now()
	for i := 0; i < b.N; i++ {
		operation()
	}
	duration := time.Since(start)

	opsPerSecond := float64(b.N) / duration.Seconds()
	b.ReportMetric(opsPerSecond, "ops/sec")
}
