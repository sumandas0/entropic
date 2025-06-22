# Entropic Performance Testing Suite

This comprehensive performance testing suite measures entity ingestion speed and identifies bottlenecks in the Entropic storage engine using real database clusters via dockertest.

## Overview

The performance test suite provides:
- **Entity ingestion benchmarks** - Single and multi-threaded performance tests
- **Bulk operation testing** - Large-scale data ingestion scenarios
- **Bottleneck analysis** - Component isolation and profiling
- **Real-world simulations** - E-commerce and IoT workload patterns
- **Automated reporting** - HTML/CSV reports with charts and comparisons

## Prerequisites

- Go 1.24+ (for tool dependency management)
- Docker (for dockertest containers)
- Make (optional, for using Makefile commands)

## Quick Start

```bash
# Install tool dependencies
make tools

# Run all performance tests and generate report
make all

# View the HTML report
open reports/html/performance_report_*.html
```

## Test Categories

### 1. Entity Ingestion Tests (`ingestion/entity_ingestion_test.go`)

Tests basic entity creation performance under various conditions:

- **Single-threaded ingestion** - Baseline performance metrics
- **Concurrent ingestion** - Multi-worker scalability testing
- **Batch operations** - Optimal batch size determination
- **Sustained load** - Long-running stability tests
- **Mixed entity types** - Real-world variety simulation

### 2. Bulk Ingestion Tests (`ingestion/bulk_ingestion_test.go`)

Focuses on high-volume data loading scenarios:

- **Scaling tests** - Performance with 100 to 100K entities
- **Streaming ingestion** - Continuous data flow simulation
- **Parallel bulk operations** - Multiple bulk loads simultaneously
- **Memory-efficient loading** - Large dataset handling
- **Realistic workloads** - E-commerce catalog simulation

### 3. Bottleneck Analysis (`ingestion/bottleneck_analysis_test.go`)

Isolates and measures individual components:

- **Database-only performance** - PostgreSQL throughput limits
- **Index-only performance** - Typesense indexing speed
- **Component isolation** - Pipeline stage analysis
- **Connection pool testing** - Concurrency limits
- **CPU/Memory profiling** - Resource utilization analysis

## Running Tests

### Using Make

```bash
# Run specific test categories
make ingestion      # Entity ingestion tests
make bulk          # Bulk operation tests
make bottleneck    # Component analysis

# Run with profiling
make profile       # Generate CPU and memory profiles

# Generate reports
make report        # Create HTML/CSV reports

# Compare with baseline
make baseline      # Save current results as baseline
make compare       # Compare new results with baseline
```

### Using Go Commands

```bash
# Run all performance tests
go test -timeout=30m ./ingestion/...

# Run specific test
go test -timeout=10m -v ./ingestion -run TestEntityIngestionPerformance

# Run with custom parameters
go test -bench=. -benchmem -benchtime=30s ./ingestion
```

### Test Flags

- `-short` - Skip long-running tests
- `-timeout` - Set test timeout (default: 10m)
- `-v` - Verbose output
- `-bench` - Run benchmarks
- `-benchmem` - Include memory allocation stats
- `-cpuprofile` - Write CPU profile
- `-memprofile` - Write memory profile

## Performance Metrics

### Key Metrics Collected

1. **Throughput Metrics**
   - Operations per second (avg/peak/p95)
   - Total operations completed
   - Success/failure rates

2. **Latency Metrics**
   - Min/Max/Mean/Median
   - Percentiles (P95, P99, P99.9)
   - Latency distribution histogram

3. **Resource Metrics**
   - Memory usage (current/peak)
   - CPU utilization
   - Goroutine count
   - GC pause times

4. **Custom Metrics**
   - Entity size impact
   - Batch size optimization
   - Connection pool utilization
   - Component-specific timings

## Report Generation

### HTML Reports

Generated HTML reports include:
- Executive summary with key metrics
- Interactive throughput and latency charts
- Detailed test results table
- Resource utilization graphs
- Performance trend analysis

```bash
# Generate HTML report
go run ./reports/cmd/generate-report \
  -input="reports/*.json" \
  -output="reports/html"
```

### CSV Export

For further analysis in spreadsheet tools:
```bash
# Export to CSV
go run ./reports/cmd/generate-report \
  -input="reports/*.json" \
  -csv="performance_results.csv"
```

### Baseline Comparison

Track performance over time:
```bash
# Save baseline
cat reports/*.json > reports/baseline.json

# Compare with baseline
go run ./reports/cmd/generate-report \
  -baseline="reports/baseline.json" \
  -compare
```

## Test Configuration

### Entity Templates

Configure test entity characteristics:
```go
template := utils.EntityTemplate{
    Type:            "product",
    PropertySizeKB:  10,        // Size of properties
    ComplexityLevel: "complex",  // simple/medium/complex
    IncludeVector:   true,      // Add vector embeddings
    VectorDim:       384,       // Vector dimension
}
```

### Load Patterns

Simulate different load patterns:
```go
pattern := utils.LoadPattern{
    Type:        "burst",    // steady/burst/ramp/spike
    BaseRate:    100,       // Operations per second
    BurstFactor: 5.0,      // Multiplier for bursts
    Duration:    5*time.Minute,
}
```

## Profiling and Analysis

### CPU Profiling

```bash
# Run with CPU profiling
go test -cpuprofile=cpu.prof -bench=. ./ingestion

# Analyze profile
go tool pprof -http=:8080 cpu.prof
```

### Memory Profiling

```bash
# Run with memory profiling
go test -memprofile=mem.prof -bench=. ./ingestion

# Analyze allocations
go tool pprof -http=:8080 -alloc_space mem.prof
```

### Flame Graphs

Generate flame graphs for visualization:
```bash
# Using the Makefile
make flamegraph

# Or manually
go tool pprof -http=:8080 reports/cpu_profile_*.prof
```

## CI/CD Integration

### GitHub Actions Example

```yaml
- name: Run Performance Tests
  run: |
    cd tests/performance
    make ci
    
- name: Upload Reports
  uses: actions/upload-artifact@v3
  with:
    name: performance-reports
    path: tests/performance/reports/html/
    
- name: Check Regressions
  run: |
    # Fail if performance degrades >10%
    make compare | grep -q "No significant performance regressions" || exit 1
```

### Performance Gates

Set minimum performance thresholds:
```go
require.GreaterOrEqual(t, throughput, 500.0, 
    "Throughput below 500 ops/sec")
require.LessOrEqual(t, p95Latency, 100.0,
    "P95 latency exceeds 100ms")
```

## Troubleshooting

### Common Issues

1. **Docker containers fail to start**
   ```bash
   # Check Docker is running
   docker ps
   
   # Clean up old containers
   docker system prune -f
   ```

2. **Tests timeout**
   ```bash
   # Increase timeout
   go test -timeout=60m ./ingestion/...
   ```

3. **Out of memory**
   ```bash
   # Limit concurrent workers
   export GOMAXPROCS=4
   ```

4. **Port conflicts**
   ```bash
   # Kill processes using test ports
   lsof -ti:5432,8108,6379 | xargs kill -9
   ```

### Debug Output

Enable detailed logging:
```bash
# Set log level
export LOG_LEVEL=debug

# Run with verbose output
go test -v -count=1 ./ingestion/...
```

## Performance Optimization Tips

Based on test results, consider:

1. **Connection Pool Tuning**
   - Adjust `max_connections` in PostgreSQL
   - Configure pool size based on worker count

2. **Batch Size Optimization**
   - Use 100-1000 entities per batch for optimal throughput
   - Smaller batches for lower latency requirements

3. **Index Configuration**
   - Disable unnecessary indexes during bulk loads
   - Use deferred index building for large imports

4. **Resource Allocation**
   - Increase PostgreSQL `shared_buffers`
   - Tune Typesense memory settings
   - Configure appropriate Docker resource limits

## Contributing

When adding new performance tests:

1. Use the existing test patterns and utilities
2. Include meaningful metrics collection
3. Add appropriate test documentation
4. Ensure tests work with dockertest
5. Update this README with new test descriptions

## License

This performance testing suite is part of the Entropic project and follows the same license terms.