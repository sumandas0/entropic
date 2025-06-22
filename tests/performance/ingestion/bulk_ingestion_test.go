package ingestion

import (
	"context"
	"fmt"
	"runtime"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/sumandas0/entropic/internal/models"
	"github.com/sumandas0/entropic/tests/performance/utils"
	"github.com/sumandas0/entropic/tests/testhelpers"
)

// TestBulkIngestionScaling tests performance with increasing batch sizes
func TestBulkIngestionScaling(t *testing.T) {
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

	batchSizes := []int{100, 1000, 10000, 100000}
	
	generator := utils.NewDataGenerator(time.Now().UnixNano())
	
	for _, batchSize := range batchSizes {
		t.Run(fmt.Sprintf("BatchSize_%d", batchSize), func(t *testing.T) {
			collector := utils.NewMetricsCollector()
			
			// Pre-generate entities
			template := utils.EntityTemplate{
				Type:            "product",
				PropertySizeKB:  1,
				ComplexityLevel: "simple",
			}
			entities := generator.GenerateBatch(template, batchSize)
			
			// Measure bulk ingestion
			collector.Start()
			
			startTime := time.Now()
			successCount := int64(0)
			errorCount := int64(0)
			
			// Process in parallel with optimal worker count
			workerCount := runtime.NumCPU() * 2
			if workerCount > batchSize/10 {
				workerCount = batchSize / 10
				if workerCount < 1 {
					workerCount = 1
				}
			}
			
			entitiesPerWorker := batchSize / workerCount
			var wg sync.WaitGroup
			
			for w := 0; w < workerCount; w++ {
				wg.Add(1)
				startIdx := w * entitiesPerWorker
				endIdx := startIdx + entitiesPerWorker
				if w == workerCount-1 {
					endIdx = batchSize
				}
				
				go func(start, end int) {
					defer wg.Done()
					for i := start; i < end; i++ {
						timer := collector.StartOperation()
						err := env.Engine.CreateEntity(ctx, entities[i])
						timer.End(err)
						
						if err != nil {
							atomic.AddInt64(&errorCount, 1)
						} else {
							atomic.AddInt64(&successCount, 1)
						}
					}
				}(startIdx, endIdx)
			}
			
			wg.Wait()
			totalDuration := time.Since(startTime)
			collector.Stop()
			
			// Calculate and record metrics
			throughput := float64(batchSize) / totalDuration.Seconds()
			collector.SetCustomMetric("batch_size", batchSize)
			collector.SetCustomMetric("worker_count", workerCount)
			collector.SetCustomMetric("total_duration_ms", totalDuration.Milliseconds())
			collector.SetCustomMetric("overall_throughput", throughput)
			collector.SetCustomMetric("success_count", successCount)
			collector.SetCustomMetric("error_count", errorCount)
			
			// Save report
			reportFile := fmt.Sprintf("reports/bulk_scaling_%d_%d.json", 
				batchSize, time.Now().Unix())
			collector.SaveReport(fmt.Sprintf("Bulk Ingestion %d entities", batchSize), reportFile)
			collector.PrintSummary(fmt.Sprintf("Bulk Ingestion %d entities", batchSize))
			
			// Verify performance
			require.Equal(t, int64(batchSize), successCount+errorCount, 
				"Not all entities were processed")
			require.Less(t, float64(errorCount), float64(batchSize)*0.05, 
				"Error rate exceeds 5%%")
		})
	}
}

// TestStreamingIngestion tests continuous streaming ingestion
func TestStreamingIngestion(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping performance test in short mode")
	}

	ctx := context.Background()
	env := testhelpers.SetupTestEnvironment(t, ctx)
	defer env.Cleanup(ctx)

	// Create schema
	schema := testhelpers.CreateTestEntitySchema("event")
	err := env.Engine.CreateEntitySchema(ctx, schema)
	require.NoError(t, err)

	tests := []struct {
		name            string
		streamDuration  time.Duration
		streamsCount    int
		eventsPerSecond int
		template        utils.EntityTemplate
	}{
		{
			name:            "Single stream - 1000 events/sec",
			streamDuration:  30 * time.Second,
			streamsCount:    1,
			eventsPerSecond: 1000,
			template: utils.EntityTemplate{
				Type:            "event",
				PropertySizeKB:  1,
				ComplexityLevel: "simple",
			},
		},
		{
			name:            "10 streams - 100 events/sec each",
			streamDuration:  30 * time.Second,
			streamsCount:    10,
			eventsPerSecond: 100,
			template: utils.EntityTemplate{
				Type:            "event",
				PropertySizeKB:  2,
				ComplexityLevel: "simple",
			},
		},
		{
			name:            "100 streams - 10 events/sec each",
			streamDuration:  20 * time.Second,
			streamsCount:    100,
			eventsPerSecond: 10,
			template: utils.EntityTemplate{
				Type:            "event",
				PropertySizeKB:  1,
				ComplexityLevel: "simple",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			generator := utils.NewDataGenerator(time.Now().UnixNano())
			collector := utils.NewMetricsCollector()
			
			collector.Start()
			
			ctx, cancel := context.WithTimeout(ctx, tt.streamDuration)
			defer cancel()
			
			var wg sync.WaitGroup
			var totalEvents int64
			
			// Start multiple streams
			for s := 0; s < tt.streamsCount; s++ {
				wg.Add(1)
				go func(streamID int) {
					defer wg.Done()
					
					ticker := time.NewTicker(time.Second / time.Duration(tt.eventsPerSecond))
					defer ticker.Stop()
					
					for {
						select {
						case <-ctx.Done():
							return
						case <-ticker.C:
							entity := generator.GenerateEntity(tt.template)
							// Add stream ID to properties
							entity.Properties["stream_id"] = streamID
							entity.Properties["timestamp"] = time.Now().UnixNano()
							
							timer := collector.StartOperation()
							err := env.Engine.CreateEntity(ctx, entity)
							timer.End(err)
							
							atomic.AddInt64(&totalEvents, 1)
						}
					}
				}(s)
			}
			
			wg.Wait()
			collector.Stop()
			
			// Record streaming metrics
			actualDuration := collector.GetReport(tt.name).Duration
			actualThroughput := float64(totalEvents) / actualDuration.Seconds()
			expectedThroughput := float64(tt.streamsCount * tt.eventsPerSecond)
			
			collector.SetCustomMetric("streams_count", tt.streamsCount)
			collector.SetCustomMetric("events_per_second_per_stream", tt.eventsPerSecond)
			collector.SetCustomMetric("total_events", totalEvents)
			collector.SetCustomMetric("expected_throughput", expectedThroughput)
			collector.SetCustomMetric("actual_throughput", actualThroughput)
			collector.SetCustomMetric("throughput_efficiency", actualThroughput/expectedThroughput*100)
			
			// Save report
			reportFile := fmt.Sprintf("reports/streaming_%s_%d.json", 
				tt.name, time.Now().Unix())
			collector.SaveReport(tt.name, reportFile)
			collector.PrintSummary(tt.name)
			
			// Verify streaming performance
			report := collector.GetReport(tt.name)
			require.GreaterOrEqual(t, actualThroughput, expectedThroughput*0.8,
				"Streaming throughput below 80%% of expected")
			require.GreaterOrEqual(t, report.Metrics.SuccessRate, 95.0,
				"Success rate below 95%%")
		})
	}
}

// TestParallelBulkOperations tests multiple bulk operations in parallel
func TestParallelBulkOperations(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping performance test in short mode")
	}

	ctx := context.Background()
	env := testhelpers.SetupTestEnvironment(t, ctx)
	defer env.Cleanup(ctx)

	// Create multiple schemas for different entity types
	entityTypes := []string{"user", "product", "order"}
	for _, entityType := range entityTypes {
		schema := testhelpers.CreateTestEntitySchema(entityType)
		err := env.Engine.CreateEntitySchema(ctx, schema)
		require.NoError(t, err)
	}

	tests := []struct {
		name              string
		bulkOperations    int
		entitiesPerBulk   int
		parallelExecution bool
	}{
		{
			name:              "Sequential bulk operations",
			bulkOperations:    10,
			entitiesPerBulk:   1000,
			parallelExecution: false,
		},
		{
			name:              "Parallel bulk operations",
			bulkOperations:    10,
			entitiesPerBulk:   1000,
			parallelExecution: true,
		},
		{
			name:              "High parallel bulk load",
			bulkOperations:    50,
			entitiesPerBulk:   500,
			parallelExecution: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			generator := utils.NewDataGenerator(time.Now().UnixNano())
			collector := utils.NewMetricsCollector()
			
			collector.Start()
			startTime := time.Now()
			
			var wg sync.WaitGroup
			var totalSuccess int64
			var totalErrors int64
			
			processBulk := func(bulkID int) {
				defer wg.Done()
				
				// Rotate through entity types
				entityType := entityTypes[bulkID%len(entityTypes)]
				template := utils.EntityTemplate{
					Type:            entityType,
					PropertySizeKB:  2,
					ComplexityLevel: "medium",
				}
				
				// Generate bulk entities
				entities := generator.GenerateBatch(template, tt.entitiesPerBulk)
				
				// Process bulk
				bulkStart := time.Now()
				var bulkSuccess, bulkErrors int64
				
				for _, entity := range entities {
					timer := collector.StartOperation()
					err := env.Engine.CreateEntity(ctx, entity)
					timer.End(err)
					
					if err != nil {
						bulkErrors++
					} else {
						bulkSuccess++
					}
				}
				
				bulkDuration := time.Since(bulkStart)
				
				// Record bulk metrics
				atomic.AddInt64(&totalSuccess, bulkSuccess)
				atomic.AddInt64(&totalErrors, bulkErrors)
				
				collector.RecordBatchOperation(
					int64(tt.entitiesPerBulk),
					float64(bulkDuration.Milliseconds()),
					bulkErrors,
				)
			}
			
			if tt.parallelExecution {
				// Process all bulks in parallel
				for i := 0; i < tt.bulkOperations; i++ {
					wg.Add(1)
					go processBulk(i)
				}
			} else {
				// Process bulks sequentially
				for i := 0; i < tt.bulkOperations; i++ {
					wg.Add(1)
					processBulk(i)
				}
			}
			
			wg.Wait()
			totalDuration := time.Since(startTime)
			collector.Stop()
			
			// Calculate aggregate metrics
			totalEntities := tt.bulkOperations * tt.entitiesPerBulk
			overallThroughput := float64(totalEntities) / totalDuration.Seconds()
			
			collector.SetCustomMetric("bulk_operations", tt.bulkOperations)
			collector.SetCustomMetric("entities_per_bulk", tt.entitiesPerBulk)
			collector.SetCustomMetric("parallel_execution", tt.parallelExecution)
			collector.SetCustomMetric("total_entities", totalEntities)
			collector.SetCustomMetric("overall_throughput", overallThroughput)
			collector.SetCustomMetric("success_count", totalSuccess)
			collector.SetCustomMetric("error_count", totalErrors)
			
			// Save report
			reportFile := fmt.Sprintf("reports/parallel_bulk_%s_%d.json",
				tt.name, time.Now().Unix())
			collector.SaveReport(tt.name, reportFile)
			collector.PrintSummary(tt.name)
		})
	}
}

// TestMemoryEfficientBulkLoading tests memory usage during large bulk loads
func TestMemoryEfficientBulkLoading(t *testing.T) {
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

	tests := []struct {
		name           string
		totalEntities  int
		entitySizeKB   int
		chunkSize      int
		workers        int
	}{
		{
			name:          "Large entities - chunked processing",
			totalEntities: 10000,
			entitySizeKB:  100,
			chunkSize:     100,
			workers:       4,
		},
		{
			name:          "Very large dataset - streaming",
			totalEntities: 50000,
			entitySizeKB:  10,
			chunkSize:     500,
			workers:       8,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			generator := utils.NewDataGenerator(time.Now().UnixNano())
			collector := utils.NewMetricsCollector()
			
			// Track memory usage
			var m runtime.MemStats
			runtime.ReadMemStats(&m)
			startMemory := m.Alloc / 1024 / 1024 // MB
			
			collector.Start()
			
			// Channel for streaming entities
			entityChan := make(chan *models.Entity, tt.workers*10)
			
			// Producer goroutine
			go func() {
				defer close(entityChan)
				
				template := utils.EntityTemplate{
					Type:            "large_entity",
					PropertySizeKB:  tt.entitySizeKB,
					ComplexityLevel: "complex",
				}
				
				for i := 0; i < tt.totalEntities; i++ {
					entity := generator.GenerateEntity(template)
					entityChan <- entity
					
					// Yield occasionally to prevent memory buildup
					if i%100 == 0 {
						runtime.Gosched()
					}
				}
			}()
			
			// Consumer workers
			var wg sync.WaitGroup
			var processedCount int64
			var errorCount int64
			
			for w := 0; w < tt.workers; w++ {
				wg.Add(1)
				go func() {
					defer wg.Done()
					
					for entity := range entityChan {
						timer := collector.StartOperation()
						err := env.Engine.CreateEntity(ctx, entity)
						timer.End(err)
						
						if err != nil {
							atomic.AddInt64(&errorCount, 1)
						} else {
							atomic.AddInt64(&processedCount, 1)
						}
						
						// Force GC occasionally to measure real memory usage
						if atomic.LoadInt64(&processedCount)%1000 == 0 {
							runtime.GC()
						}
					}
				}()
			}
			
			wg.Wait()
			collector.Stop()
			
			// Final memory measurement
			runtime.GC()
			runtime.ReadMemStats(&m)
			endMemory := m.Alloc / 1024 / 1024 // MB
			peakMemory := m.TotalAlloc / 1024 / 1024 // MB
			
			// Record memory metrics
			collector.SetCustomMetric("start_memory_mb", startMemory)
			collector.SetCustomMetric("end_memory_mb", endMemory)
			collector.SetCustomMetric("peak_memory_mb", peakMemory)
			collector.SetCustomMetric("memory_per_entity_kb", 
				float64(peakMemory-startMemory)*1024/float64(tt.totalEntities))
			collector.SetCustomMetric("total_entities", tt.totalEntities)
			collector.SetCustomMetric("entity_size_kb", tt.entitySizeKB)
			collector.SetCustomMetric("chunk_size", tt.chunkSize)
			collector.SetCustomMetric("workers", tt.workers)
			collector.SetCustomMetric("processed_count", processedCount)
			collector.SetCustomMetric("error_count", errorCount)
			
			// Save report
			reportFile := fmt.Sprintf("reports/memory_efficient_%s_%d.json",
				tt.name, time.Now().Unix())
			collector.SaveReport(tt.name, reportFile)
			collector.PrintSummary(tt.name)
			
			// Verify memory efficiency
			memoryPerEntity := float64(peakMemory-startMemory) * 1024 / float64(tt.totalEntities)
			t.Logf("Memory per entity: %.2f KB", memoryPerEntity)
			
			// Memory usage should be reasonable compared to entity size
			require.Less(t, memoryPerEntity, float64(tt.entitySizeKB*2),
				"Memory usage per entity exceeds 2x the entity size")
		})
	}
}

// TestRealisticWorkloadIngestion simulates real-world ingestion patterns
func TestRealisticWorkloadIngestion(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping performance test in short mode")
	}

	ctx := context.Background()
	env := testhelpers.SetupTestEnvironment(t, ctx)
	defer env.Cleanup(ctx)

	// Create schemas for e-commerce scenario
	schemas := map[string]*models.EntitySchema{
		"user":     testhelpers.CreateTestEntitySchema("user"),
		"product":  testhelpers.CreateTestEntitySchema("product"),
		"order":    testhelpers.CreateTestEntitySchema("order"),
		"review":   testhelpers.CreateTestEntitySchema("review"),
		"category": testhelpers.CreateTestEntitySchema("category"),
	}
	
	for _, schema := range schemas {
		err := env.Engine.CreateEntitySchema(ctx, schema)
		require.NoError(t, err)
	}

	// Create relationship schemas
	relSchemas := []struct {
		name string
		from string
		to   string
	}{
		{"placed_by", "order", "user"},
		{"contains", "order", "product"},
		{"reviewed_by", "review", "user"},
		{"reviews", "review", "product"},
		{"belongs_to", "product", "category"},
	}
	
	for _, rs := range relSchemas {
		schema := testhelpers.CreateTestRelationshipSchema(rs.name, rs.from, rs.to)
		err := env.Engine.CreateRelationshipSchema(ctx, schema)
		require.NoError(t, err)
	}

	generator := utils.NewDataGenerator(time.Now().UnixNano())
	collector := utils.NewMetricsCollector()
	
	// Generate base data
	entities, relations := utils.GenerateRealisticEcommerce(generator)
	
	collector.Start()
	
	// Phase 1: Initial catalog load (products and categories)
	t.Log("Phase 1: Loading product catalog...")
	for _, entity := range entities {
		if entity.EntityType == "product" || entity.EntityType == "category" {
			timer := collector.StartOperation()
			err := env.Engine.CreateEntity(ctx, entity)
			timer.End(err)
		}
	}
	
	// Phase 2: User registration burst
	t.Log("Phase 2: User registration burst...")
	var wg sync.WaitGroup
	for _, entity := range entities {
		if entity.EntityType == "user" {
			wg.Add(1)
			go func(e *models.Entity) {
				defer wg.Done()
				timer := collector.StartOperation()
				err := env.Engine.CreateEntity(ctx, e)
				timer.End(err)
			}(entity)
		}
	}
	wg.Wait()
	
	// Phase 3: Order processing with relationships
	t.Log("Phase 3: Order processing...")
	for _, entity := range entities {
		if entity.EntityType == "order" {
			timer := collector.StartOperation()
			err := env.Engine.CreateEntity(ctx, entity)
			timer.End(err)
		}
	}
	
	// Create relationships
	for _, relation := range relations {
		timer := collector.StartOperation()
		err := env.Engine.CreateRelation(ctx, relation)
		timer.End(err)
	}
	
	collector.Stop()
	
	// Calculate phase metrics
	report := collector.GetReport("Realistic E-commerce Workload")
	
	collector.SetCustomMetric("total_entities", len(entities))
	collector.SetCustomMetric("total_relations", len(relations))
	collector.SetCustomMetric("entity_types", len(schemas))
	
	// Save report
	reportFile := fmt.Sprintf("reports/realistic_workload_%d.json", time.Now().Unix())
	collector.SaveReport("Realistic E-commerce Workload", reportFile)
	collector.PrintSummary("Realistic E-commerce Workload")
	
	// Verify realistic workload performance
	require.GreaterOrEqual(t, report.Metrics.SuccessRate, 95.0,
		"Success rate below 95%% for realistic workload")
}