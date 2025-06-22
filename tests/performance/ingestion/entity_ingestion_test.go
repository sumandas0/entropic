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

// TestEntityIngestionPerformance measures single-threaded ingestion performance
func TestEntityIngestionPerformance(t *testing.T) {
	// Allow running a quick version in short mode
	shortMode := testing.Short()

	ctx := context.Background()
	env := testhelpers.SetupTestEnvironment(t, ctx)
	defer env.Cleanup(ctx)

	// Create schema
	schema := testhelpers.CreateTestEntitySchema("product")
	err := env.Engine.CreateEntitySchema(ctx, schema)
	require.NoError(t, err)

	tests := []struct {
		name           string
		entityCount    int
		template       utils.EntityTemplate
		expectedTPS    float64 // minimum expected transactions per second
	}{
		{
			name:        "Small entities (1KB)",
			entityCount: func() int {
				if shortMode {
					return 100  // Quick test for validation
				}
				return 10000
			}(),
			template: utils.EntityTemplate{
				Type:            "product",
				PropertySizeKB:  1,
				ComplexityLevel: "simple",
			},
			expectedTPS: func() float64 {
				if shortMode {
					return 10  // Lower expectation for quick test
				}
				return 500
			}(),
		},
		{
			name:        "Medium entities (10KB)",
			entityCount: func() int {
				if shortMode {
					return 50
				}
				return 5000
			}(),
			template: utils.EntityTemplate{
				Type:            "product",
				PropertySizeKB:  10,
				ComplexityLevel: "medium",
			},
			expectedTPS: func() float64 {
				if shortMode {
					return 5
				}
				return 200
			}(),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			generator := utils.NewDataGenerator(time.Now().UnixNano())
			collector := utils.NewMetricsCollector()

			// Pre-generate entities to avoid generation overhead
			entities := generator.GenerateBatch(tt.template, tt.entityCount)

			// Start metrics collection
			collector.Start()

			// Run ingestion
			for i, entity := range entities {
				timer := collector.StartOperation()
				err := env.Engine.CreateEntity(ctx, entity)
				timer.End(err)

				if err != nil {
					t.Logf("Failed to create entity %d: %v", i, err)
				}
			}

			// Stop collection and generate report
			collector.Stop()
			
			// Save detailed report
			reportFile := fmt.Sprintf("reports/entity_ingestion_%s_%d.json", 
				tt.name, time.Now().Unix())
			collector.SaveReport(tt.name, reportFile)
			
			// Print summary
			collector.PrintSummary(tt.name)

			// Verify performance meets expectations
			report := collector.GetReport(tt.name)
			require.GreaterOrEqual(t, report.Metrics.Throughput.Average, tt.expectedTPS,
				"Throughput below expected: got %.2f, want >= %.2f ops/sec",
				report.Metrics.Throughput.Average, tt.expectedTPS)
			require.GreaterOrEqual(t, report.Metrics.SuccessRate, 95.0,
				"Success rate too low: %.2f%%", report.Metrics.SuccessRate)
		})
	}
}

// TestConcurrentEntityIngestion measures multi-threaded ingestion performance
func TestConcurrentEntityIngestion(t *testing.T) {
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
		name         string
		workers      int
		totalEntities int
		template     utils.EntityTemplate
	}{
		{
			name:          "5 workers - small entities",
			workers:       5,
			totalEntities: 10000,
			template: utils.EntityTemplate{
				Type:            "product",
				PropertySizeKB:  1,
				ComplexityLevel: "simple",
			},
		},
		{
			name:          "10 workers - medium entities",
			workers:       10,
			totalEntities: 10000,
			template: utils.EntityTemplate{
				Type:            "product",
				PropertySizeKB:  10,
				ComplexityLevel: "medium",
			},
		},
		{
			name:          "20 workers - mixed sizes",
			workers:       20,
			totalEntities: 20000,
			template: utils.EntityTemplate{
				Type:            "product",
				PropertySizeKB:  5,
				ComplexityLevel: "medium",
			},
		},
		{
			name:          "CPU count workers",
			workers:       runtime.NumCPU(),
			totalEntities: 10000,
			template: utils.EntityTemplate{
				Type:            "product",
				PropertySizeKB:  2,
				ComplexityLevel: "simple",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			generator := utils.NewDataGenerator(time.Now().UnixNano())
			collector := utils.NewMetricsCollector()

			// Pre-generate entities
			entities := generator.GenerateBatch(tt.template, tt.totalEntities)
			entitiesPerWorker := tt.totalEntities / tt.workers

			// Start metrics collection
			collector.Start()

			// Run concurrent ingestion
			var wg sync.WaitGroup
			var successCount int64
			var errorCount int64

			for w := 0; w < tt.workers; w++ {
				wg.Add(1)
				startIdx := w * entitiesPerWorker
				endIdx := startIdx + entitiesPerWorker
				if w == tt.workers-1 {
					endIdx = tt.totalEntities // Handle remainder
				}

				go func(workerID int, start, end int) {
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
				}(w, startIdx, endIdx)
			}

			wg.Wait()
			collector.Stop()

			// Custom metrics
			collector.SetCustomMetric("workers", tt.workers)
			collector.SetCustomMetric("success_count", atomic.LoadInt64(&successCount))
			collector.SetCustomMetric("error_count", atomic.LoadInt64(&errorCount))

			// Save and display report
			reportFile := fmt.Sprintf("reports/concurrent_ingestion_%s_%d.json",
				tt.name, time.Now().Unix())
			collector.SaveReport(tt.name, reportFile)
			collector.PrintSummary(tt.name)

			// Verify results
			report := collector.GetReport(tt.name)
			require.GreaterOrEqual(t, report.Metrics.SuccessRate, 90.0,
				"Success rate too low: %.2f%%", report.Metrics.SuccessRate)
		})
	}
}

// TestBatchEntityIngestion measures batch ingestion performance
func TestBatchEntityIngestion(t *testing.T) {
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
		name       string
		batchSize  int
		batchCount int
		template   utils.EntityTemplate
	}{
		{
			name:       "Small batches (10)",
			batchSize:  10,
			batchCount: 1000,
			template: utils.EntityTemplate{
				Type:            "product",
				PropertySizeKB:  1,
				ComplexityLevel: "simple",
			},
		},
		{
			name:       "Medium batches (100)",
			batchSize:  100,
			batchCount: 100,
			template: utils.EntityTemplate{
				Type:            "product",
				PropertySizeKB:  1,
				ComplexityLevel: "simple",
			},
		},
		{
			name:       "Large batches (1000)",
			batchSize:  1000,
			batchCount: 10,
			template: utils.EntityTemplate{
				Type:            "product",
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

			for batch := 0; batch < tt.batchCount; batch++ {
				// Generate batch
				entities := generator.GenerateBatch(tt.template, tt.batchSize)

				// Time the entire batch
				batchStart := time.Now()
				var batchErrors int64

				// Process batch concurrently
				var wg sync.WaitGroup
				for _, entity := range entities {
					wg.Add(1)
					go func(e *models.Entity) {
						defer wg.Done()
						if err := env.Engine.CreateEntity(ctx, e); err != nil {
							atomic.AddInt64(&batchErrors, 1)
						}
					}(entity)
				}
				wg.Wait()

				// Record batch metrics
				batchDuration := time.Since(batchStart)
				collector.RecordBatchOperation(
					int64(tt.batchSize),
					float64(batchDuration.Milliseconds()),
					batchErrors,
				)
			}

			collector.Stop()

			// Add custom metrics
			collector.SetCustomMetric("batch_size", tt.batchSize)
			collector.SetCustomMetric("batch_count", tt.batchCount)
			collector.SetCustomMetric("total_entities", tt.batchSize*tt.batchCount)

			// Save and display report
			reportFile := fmt.Sprintf("reports/batch_ingestion_%s_%d.json",
				tt.name, time.Now().Unix())
			collector.SaveReport(tt.name, reportFile)
			collector.PrintSummary(tt.name)
		})
	}
}

// TestSustainedIngestionLoad tests sustained load over time
func TestSustainedIngestionLoad(t *testing.T) {
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
		name     string
		duration time.Duration
		pattern  utils.LoadPattern
		template utils.EntityTemplate
	}{
		{
			name:     "Steady load - 100 ops/sec for 5 minutes",
			duration: 5 * time.Minute,
			pattern: utils.LoadPattern{
				Type:     "steady",
				BaseRate: 100,
			},
			template: utils.EntityTemplate{
				Type:            "event",
				PropertySizeKB:  2,
				ComplexityLevel: "simple",
			},
		},
		{
			name:     "Burst load - 50 base with 5x bursts",
			duration: 3 * time.Minute,
			pattern: utils.LoadPattern{
				Type:        "burst",
				BaseRate:    50,
				BurstFactor: 5.0,
			},
			template: utils.EntityTemplate{
				Type:            "event",
				PropertySizeKB:  2,
				ComplexityLevel: "simple",
			},
		},
		{
			name:     "Ramp load - 0 to 200 ops/sec over 2 minutes",
			duration: 3 * time.Minute,
			pattern: utils.LoadPattern{
				Type:       "ramp",
				BaseRate:   200,
				RampUpTime: 2 * time.Minute,
			},
			template: utils.EntityTemplate{
				Type:            "event",
				PropertySizeKB:  2,
				ComplexityLevel: "simple",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			generator := utils.NewDataGenerator(time.Now().UnixNano())
			collector := utils.NewMetricsCollector()
			loadGen := utils.NewLoadGenerator(tt.pattern)

			collector.Start()

			// Operation to be executed
			operation := func() error {
				entity := generator.GenerateEntity(tt.template)
				timer := collector.StartOperation()
				err := env.Engine.CreateEntity(ctx, entity)
				timer.End(err)
				return err
			}

			// Start load generation
			loadGen.Start(operation)

			// Run for specified duration
			time.Sleep(tt.duration)

			// Stop load and collection
			loadGen.Stop()
			collector.Stop()

			// Add pattern info to custom metrics
			collector.SetCustomMetric("load_pattern", tt.pattern.Type)
			collector.SetCustomMetric("base_rate", tt.pattern.BaseRate)
			collector.SetCustomMetric("test_duration", tt.duration.String())

			// Save and display report
			reportFile := fmt.Sprintf("reports/sustained_load_%s_%d.json",
				tt.name, time.Now().Unix())
			collector.SaveReport(tt.name, reportFile)
			collector.PrintSummary(tt.name)

			// Verify sustained performance
			report := collector.GetReport(tt.name)
			require.GreaterOrEqual(t, report.Metrics.SuccessRate, 90.0,
				"Success rate dropped below 90%% during sustained load")
		})
	}
}

// TestMixedEntityTypeIngestion tests ingestion with multiple entity types
func TestMixedEntityTypeIngestion(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping performance test in short mode")
	}

	ctx := context.Background()
	env := testhelpers.SetupTestEnvironment(t, ctx)
	defer env.Cleanup(ctx)

	// Create multiple schemas
	entityTypes := []string{"user", "product", "order", "review", "category"}
	for _, entityType := range entityTypes {
		schema := testhelpers.CreateTestEntitySchema(entityType)
		err := env.Engine.CreateEntitySchema(ctx, schema)
		require.NoError(t, err)
	}

	generator := utils.NewDataGenerator(time.Now().UnixNano())
	collector := utils.NewMetricsCollector()

	// Define different templates for each type
	templates := map[string]utils.EntityTemplate{
		"user": {
			Type:            "user",
			PropertySizeKB:  2,
			ComplexityLevel: "medium",
		},
		"product": {
			Type:            "product",
			PropertySizeKB:  10,
			ComplexityLevel: "complex",
			IncludeVector:   true,
			VectorDim:       384,
		},
		"order": {
			Type:            "order",
			PropertySizeKB:  5,
			ComplexityLevel: "medium",
		},
		"review": {
			Type:            "review",
			PropertySizeKB:  1,
			ComplexityLevel: "simple",
		},
		"category": {
			Type:            "category",
			PropertySizeKB:  1,
			ComplexityLevel: "simple",
		},
	}

	totalOperations := 10000
	collector.Start()

	// Concurrent ingestion of mixed types
	var wg sync.WaitGroup
	opsPerType := totalOperations / len(entityTypes)

	for _, entityType := range entityTypes {
		wg.Add(1)
		go func(eType string) {
			defer wg.Done()
			template := templates[eType]

			for i := 0; i < opsPerType; i++ {
				entity := generator.GenerateEntity(template)
				timer := collector.StartOperation()
				err := env.Engine.CreateEntity(ctx, entity)
				timer.End(err)
			}
		}(entityType)
	}

	wg.Wait()
	collector.Stop()

	// Add type distribution to metrics
	collector.SetCustomMetric("entity_types", entityTypes)
	collector.SetCustomMetric("operations_per_type", opsPerType)

	// Save and display report
	reportFile := fmt.Sprintf("reports/mixed_type_ingestion_%d.json", time.Now().Unix())
	collector.SaveReport("Mixed Entity Type Ingestion", reportFile)
	collector.PrintSummary("Mixed Entity Type Ingestion")
}