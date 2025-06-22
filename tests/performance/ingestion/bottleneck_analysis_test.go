package ingestion

import (
	"context"
	"fmt"
	"os"
	"runtime"
	"runtime/pprof"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/sumandas0/entropic/internal/core"
	"github.com/sumandas0/entropic/internal/models"
	"github.com/sumandas0/entropic/tests/performance/utils"
	"github.com/sumandas0/entropic/tests/testhelpers"
)

// TestDatabaseOnlyIngestion isolates database performance
func TestDatabaseOnlyIngestion(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping performance test in short mode")
	}

	ctx := context.Background()
	env := testhelpers.SetupTestEnvironment(t, ctx)
	defer env.Cleanup(ctx)

	// Create schema directly in database
	schema := testhelpers.CreateTestEntitySchema("product")
	err := env.Engine.CreateEntitySchema(ctx, schema)
	require.NoError(t, err)

	tests := []struct {
		name          string
		entityCount   int
		template      utils.EntityTemplate
		concurrent    bool
		workers       int
	}{
		{
			name:        "Database only - sequential",
			entityCount: 5000,
			template: utils.EntityTemplate{
				Type:            "product",
				PropertySizeKB:  2,
				ComplexityLevel: "simple",
			},
			concurrent: false,
			workers:    1,
		},
		{
			name:        "Database only - concurrent",
			entityCount: 5000,
			template: utils.EntityTemplate{
				Type:            "product",
				PropertySizeKB:  2,
				ComplexityLevel: "simple",
			},
			concurrent: true,
			workers:    10,
		},
		{
			name:        "Database only - with vectors",
			entityCount: 2000,
			template: utils.EntityTemplate{
				Type:            "product",
				PropertySizeKB:  2,
				ComplexityLevel: "simple",
				IncludeVector:   true,
				VectorDim:       384,
			},
			concurrent: true,
			workers:    5,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			generator := utils.NewDataGenerator(time.Now().UnixNano())
			collector := utils.NewMetricsCollector()
			
			// Generate entities
			entities := generator.GenerateBatch(tt.template, tt.entityCount)
			
			collector.Start()
			
			if tt.concurrent {
				// Concurrent database operations
				var wg sync.WaitGroup
				entitiesPerWorker := tt.entityCount / tt.workers
				
				for w := 0; w < tt.workers; w++ {
					wg.Add(1)
					startIdx := w * entitiesPerWorker
					endIdx := startIdx + entitiesPerWorker
					if w == tt.workers-1 {
						endIdx = tt.entityCount
					}
					
					go func(start, end int) {
						defer wg.Done()
						
						for i := start; i < end; i++ {
							timer := collector.StartOperation()
							
							// Direct database operation, bypassing business logic
							pgTx, err := env.PrimaryStore.BeginTx(ctx)
							if err != nil {
								timer.End(err)
								continue
							}
							
							err = pgTx.CreateEntity(ctx, entities[i])
							if err != nil {
								pgTx.Rollback()
								timer.End(err)
								continue
							}
							
							err = pgTx.Commit()
							timer.End(err)
						}
					}(startIdx, endIdx)
				}
				
				wg.Wait()
			} else {
				// Sequential database operations
				for _, entity := range entities {
					timer := collector.StartOperation()
					
					pgTx, err := env.PrimaryStore.BeginTx(ctx)
					if err != nil {
						timer.End(err)
						continue
					}
					
					err = pgTx.CreateEntity(ctx, entity)
					if err != nil {
						pgTx.Rollback()
						timer.End(err)
						continue
					}
					
					err = pgTx.Commit()
					timer.End(err)
				}
			}
			
			collector.Stop()
			
			// Add test configuration to metrics
			collector.SetCustomMetric("test_type", "database_only")
			collector.SetCustomMetric("concurrent", tt.concurrent)
			collector.SetCustomMetric("workers", tt.workers)
			collector.SetCustomMetric("entity_count", tt.entityCount)
			
			// Save report
			reportFile := fmt.Sprintf("reports/database_only_%s_%d.json",
				tt.name, time.Now().Unix())
			collector.SaveReport(tt.name, reportFile)
			collector.PrintSummary(tt.name)
		})
	}
}

// TestIndexOnlyIngestion isolates index store performance
func TestIndexOnlyIngestion(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping performance test in short mode")
	}

	ctx := context.Background()
	env := testhelpers.SetupTestEnvironment(t, ctx)
	defer env.Cleanup(ctx)

	// Create collection for indexing
	productSchema := testhelpers.CreateTestEntitySchema("product")
	err := env.IndexStore.CreateCollection(ctx, "product", productSchema)
	if err != nil && !contains(err.Error(), "already exists") {
		require.NoError(t, err)
	}

	tests := []struct {
		name        string
		entityCount int
		template    utils.EntityTemplate
		batchSize   int
	}{
		{
			name:        "Index only - small batch",
			entityCount: 5000,
			template: utils.EntityTemplate{
				Type:            "product",
				PropertySizeKB:  2,
				ComplexityLevel: "simple",
			},
			batchSize: 100,
		},
		{
			name:        "Index only - large batch",
			entityCount: 5000,
			template: utils.EntityTemplate{
				Type:            "product",
				PropertySizeKB:  2,
				ComplexityLevel: "simple",
			},
			batchSize: 1000,
		},
		{
			name:        "Index only - with vectors",
			entityCount: 2000,
			template: utils.EntityTemplate{
				Type:            "product",
				PropertySizeKB:  2,
				ComplexityLevel: "simple",
				IncludeVector:   true,
				VectorDim:       384,
			},
			batchSize: 100,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			generator := utils.NewDataGenerator(time.Now().UnixNano())
			collector := utils.NewMetricsCollector()
			
			// Generate entities
			entities := generator.GenerateBatch(tt.template, tt.entityCount)
			
			collector.Start()
			
			// Process in batches
			for i := 0; i < tt.entityCount; i += tt.batchSize {
				end := i + tt.batchSize
				if end > tt.entityCount {
					end = tt.entityCount
				}
				
				batch := entities[i:end]
				
				timer := collector.StartOperation()
				
				// Direct index operation
				for _, entity := range batch {
					err := env.IndexStore.IndexEntity(ctx, entity)
					if err != nil {
						timer.End(err)
						continue
					}
				}
				
				timer.End(nil)
			}
			
			collector.Stop()
			
			// Add test configuration to metrics
			collector.SetCustomMetric("test_type", "index_only")
			collector.SetCustomMetric("batch_size", tt.batchSize)
			collector.SetCustomMetric("entity_count", tt.entityCount)
			
			// Save report
			reportFile := fmt.Sprintf("reports/index_only_%s_%d.json",
				tt.name, time.Now().Unix())
			collector.SaveReport(tt.name, reportFile)
			collector.PrintSummary(tt.name)
		})
	}
}

// TestComponentIsolation compares different components
func TestComponentIsolation(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping performance test in short mode")
	}

	ctx := context.Background()
	env := testhelpers.SetupTestEnvironment(t, ctx)
	defer env.Cleanup(ctx)

	// Create schema
	schema := testhelpers.CreateTestEntitySchema("product")
	err := env.Engine.CreateEntitySchema(ctx, schema)
	require.NoError(t, err)

	entityCount := 1000
	generator := utils.NewDataGenerator(time.Now().UnixNano())
	template := utils.EntityTemplate{
		Type:            "product",
		PropertySizeKB:  5,
		ComplexityLevel: "medium",
	}
	
	entities := generator.GenerateBatch(template, entityCount)

	tests := []struct {
		name      string
		operation func(*models.Entity) error
	}{
		{
			name: "Full pipeline",
			operation: func(entity *models.Entity) error {
				return env.Engine.CreateEntity(ctx, entity)
			},
		},
		{
			name: "Without indexing",
			operation: func(entity *models.Entity) error {
				// Create a temporary engine without index store
				tempEngine, _ := core.NewEngine(env.PrimaryStore, nil, env.CacheManager, env.LockManager)
				return tempEngine.CreateEntity(ctx, entity)
			},
		},
		{
			name: "Without caching",
			operation: func(entity *models.Entity) error {
				// Direct primary store operation
				tx, err := env.PrimaryStore.BeginTx(ctx)
				if err != nil {
					return err
				}
				
				if err := tx.CreateEntity(ctx, entity); err != nil {
					tx.Rollback()
					return err
				}
				
				return tx.Commit()
			},
		},
		{
			name: "Without validation",
			operation: func(entity *models.Entity) error {
				// Skip validation by going directly to storage
				tx, err := env.PrimaryStore.BeginTx(ctx)
				if err != nil {
					return err
				}
				
				// Direct insert without schema validation
				if err := tx.CreateEntity(ctx, entity); err != nil {
					tx.Rollback()
					return err
				}
				
				if env.IndexStore != nil {
					env.IndexStore.IndexEntity(ctx, entity)
				}
				
				return tx.Commit()
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			collector := utils.NewMetricsCollector()
			collector.Start()
			
			// Process entities
			for i, entity := range entities {
				// Create a copy to avoid conflicts
				entityCopy := *entity
				entityCopy.URN = fmt.Sprintf("%s-%s-%d", entity.URN, tt.name, i)
				
				timer := collector.StartOperation()
				err := tt.operation(&entityCopy)
				timer.End(err)
			}
			
			collector.Stop()
			
			// Add component info
			collector.SetCustomMetric("component_test", tt.name)
			collector.SetCustomMetric("entity_count", entityCount)
			
			// Save report
			reportFile := fmt.Sprintf("reports/component_%s_%d.json",
				tt.name, time.Now().Unix())
			collector.SaveReport(tt.name, reportFile)
			collector.PrintSummary(tt.name)
		})
	}
}

// TestConnectionPoolExhaustion tests behavior under connection pressure
func TestConnectionPoolExhaustion(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping performance test in short mode")
	}

	ctx := context.Background()
	env := testhelpers.SetupTestEnvironment(t, ctx)
	defer env.Cleanup(ctx)

	// Create schema
	schema := testhelpers.CreateTestEntitySchema("product")
	err := env.Engine.CreateEntitySchema(ctx, schema)
	require.NoError(t, err)

	tests := []struct {
		name              string
		concurrentWorkers int
		operationsPerWorker int
		holdConnection    time.Duration
	}{
		{
			name:              "Normal load",
			concurrentWorkers: 10,
			operationsPerWorker: 100,
			holdConnection:    0,
		},
		{
			name:              "High concurrency",
			concurrentWorkers: 100,
			operationsPerWorker: 10,
			holdConnection:    0,
		},
		{
			name:              "Long-held connections",
			concurrentWorkers: 50,
			operationsPerWorker: 20,
			holdConnection:    100 * time.Millisecond,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			generator := utils.NewDataGenerator(time.Now().UnixNano())
			collector := utils.NewMetricsCollector()
			
			template := utils.EntityTemplate{
				Type:            "product",
				PropertySizeKB:  1,
				ComplexityLevel: "simple",
			}
			
			collector.Start()
			
			var wg sync.WaitGroup
			var connectionErrors int64
			var timeoutErrors int64
			
			for w := 0; w < tt.concurrentWorkers; w++ {
				wg.Add(1)
				go func(workerID int) {
					defer wg.Done()
					
					for op := 0; op < tt.operationsPerWorker; op++ {
						entity := generator.GenerateEntity(template)
						entity.URN = fmt.Sprintf("%s-w%d-op%d", entity.URN, workerID, op)
						
						timer := collector.StartOperation()
						
						// Try to create entity with potential connection issues
						err := env.Engine.CreateEntity(ctx, entity)
						
						// Simulate holding connection
						if tt.holdConnection > 0 {
							time.Sleep(tt.holdConnection)
						}
						
						timer.End(err)
						
						// Analyze error types
						if err != nil {
							errStr := err.Error()
							if contains(errStr, "connection") || contains(errStr, "pool") {
								atomic.AddInt64(&connectionErrors, 1)
							} else if contains(errStr, "timeout") || contains(errStr, "context") {
								atomic.AddInt64(&timeoutErrors, 1)
							}
						}
					}
				}(w)
			}
			
			wg.Wait()
			collector.Stop()
			
			// Record connection metrics
			collector.SetCustomMetric("concurrent_workers", tt.concurrentWorkers)
			collector.SetCustomMetric("operations_per_worker", tt.operationsPerWorker)
			collector.SetCustomMetric("hold_connection_ms", tt.holdConnection.Milliseconds())
			collector.SetCustomMetric("connection_errors", connectionErrors)
			collector.SetCustomMetric("timeout_errors", timeoutErrors)
			
			// Save report
			reportFile := fmt.Sprintf("reports/connection_pool_%s_%d.json",
				tt.name, time.Now().Unix())
			collector.SaveReport(tt.name, reportFile)
			collector.PrintSummary(tt.name)
		})
	}
}

// TestCPUProfiledIngestion runs ingestion with CPU profiling
func TestCPUProfiledIngestion(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping performance test in short mode")
	}

	ctx := context.Background()
	env := testhelpers.SetupTestEnvironment(t, ctx)
	defer env.Cleanup(ctx)

	// Create schema
	schema := testhelpers.CreateTestEntitySchema("product")
	err := env.Engine.CreateEntitySchema(ctx, schema)
	require.NoError(t, err)

	generator := utils.NewDataGenerator(time.Now().UnixNano())
	collector := utils.NewMetricsCollector()
	
	// Start CPU profiling
	cpuFile := fmt.Sprintf("reports/cpu_profile_%d.prof", time.Now().Unix())
	f, err := os.Create(cpuFile)
	require.NoError(t, err)
	defer f.Close()
	
	err = pprof.StartCPUProfile(f)
	require.NoError(t, err)
	defer pprof.StopCPUProfile()
	
	// Run intensive workload
	template := utils.EntityTemplate{
		Type:            "product",
		PropertySizeKB:  10,
		ComplexityLevel: "complex",
		IncludeVector:   true,
		VectorDim:       384,
	}
	
	entityCount := 5000
	entities := generator.GenerateBatch(template, entityCount)
	
	collector.Start()
	
	// Process with multiple workers for CPU stress
	workers := runtime.NumCPU()
	var wg sync.WaitGroup
	entitiesPerWorker := entityCount / workers
	
	for w := 0; w < workers; w++ {
		wg.Add(1)
		startIdx := w * entitiesPerWorker
		endIdx := startIdx + entitiesPerWorker
		if w == workers-1 {
			endIdx = entityCount
		}
		
		go func(start, end int) {
			defer wg.Done()
			
			for i := start; i < end; i++ {
				timer := collector.StartOperation()
				err := env.Engine.CreateEntity(ctx, entities[i])
				timer.End(err)
			}
		}(startIdx, endIdx)
	}
	
	wg.Wait()
	collector.Stop()
	
	// Add profiling info
	collector.SetCustomMetric("cpu_profile_file", cpuFile)
	collector.SetCustomMetric("workers", workers)
	collector.SetCustomMetric("entity_count", entityCount)
	
	// Save report
	reportFile := fmt.Sprintf("reports/cpu_profiled_ingestion_%d.json", time.Now().Unix())
	collector.SaveReport("CPU Profiled Ingestion", reportFile)
	collector.PrintSummary("CPU Profiled Ingestion")
	
	t.Logf("CPU profile saved to: %s", cpuFile)
	t.Log("To analyze: go tool pprof -http=:8080 " + cpuFile)
}

// TestMemoryProfiledIngestion runs ingestion with memory profiling
func TestMemoryProfiledIngestion(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping performance test in short mode")
	}

	ctx := context.Background()
	env := testhelpers.SetupTestEnvironment(t, ctx)
	defer env.Cleanup(ctx)

	// Create schema
	schema := testhelpers.CreateTestEntitySchema("large_entity")
	err := env.Engine.CreateEntitySchema(ctx, schema)
	require.NoError(t, err)

	generator := utils.NewDataGenerator(time.Now().UnixNano())
	collector := utils.NewMetricsCollector()
	
	// Run memory-intensive workload
	template := utils.EntityTemplate{
		Type:            "large_entity",
		PropertySizeKB:  100,
		ComplexityLevel: "complex",
		IncludeVector:   true,
		VectorDim:       1024,
	}
	
	entityCount := 1000
	
	collector.Start()
	
	// Track memory allocations
	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	startAlloc := m.Alloc
	
	// Process entities
	for i := 0; i < entityCount; i++ {
		entity := generator.GenerateEntity(template)
		
		timer := collector.StartOperation()
		err := env.Engine.CreateEntity(ctx, entity)
		timer.End(err)
		
		// Force GC periodically to get accurate memory usage
		if i%100 == 0 {
			runtime.GC()
		}
	}
	
	collector.Stop()
	
	// Final memory stats
	runtime.GC()
	runtime.ReadMemStats(&m)
	endAlloc := m.Alloc
	
	// Write memory profile
	memFile := fmt.Sprintf("reports/mem_profile_%d.prof", time.Now().Unix())
	f, err := os.Create(memFile)
	require.NoError(t, err)
	defer f.Close()
	
	runtime.GC()
	err = pprof.WriteHeapProfile(f)
	require.NoError(t, err)
	
	// Calculate memory metrics
	totalAllocated := m.TotalAlloc - startAlloc
	avgPerEntity := totalAllocated / uint64(entityCount)
	
	collector.SetCustomMetric("memory_profile_file", memFile)
	collector.SetCustomMetric("start_alloc_mb", startAlloc/1024/1024)
	collector.SetCustomMetric("end_alloc_mb", endAlloc/1024/1024)
	collector.SetCustomMetric("total_allocated_mb", totalAllocated/1024/1024)
	collector.SetCustomMetric("avg_per_entity_kb", avgPerEntity/1024)
	collector.SetCustomMetric("gc_runs", m.NumGC)
	collector.SetCustomMetric("entity_count", entityCount)
	collector.SetCustomMetric("entity_size_kb", template.PropertySizeKB)
	
	// Save report
	reportFile := fmt.Sprintf("reports/memory_profiled_ingestion_%d.json", time.Now().Unix())
	collector.SaveReport("Memory Profiled Ingestion", reportFile)
	collector.PrintSummary("Memory Profiled Ingestion")
	
	t.Logf("Memory profile saved to: %s", memFile)
	t.Log("To analyze: go tool pprof -http=:8080 " + memFile)
}

// Helper function
func contains(s, substr string) bool {
	return len(s) >= len(substr) && s[:len(substr)] == substr || 
		   len(s) >= len(substr) && s[len(s)-len(substr):] == substr ||
		   len(substr) < len(s) && containsMiddle(s, substr)
}

func containsMiddle(s, substr string) bool {
	for i := 1; i < len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}