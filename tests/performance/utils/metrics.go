package utils

import (
	"encoding/json"
	"fmt"
	"math"
	"os"
	"runtime"
	"sort"
	"sync"
	"sync/atomic"
	"time"

	"github.com/HdrHistogram/hdrhistogram-go"
)

// MetricsCollector collects performance metrics during test execution
type MetricsCollector struct {
	mu sync.RWMutex

	// Counters
	totalOperations   int64
	successOperations int64
	failedOperations  int64

	// Timing
	startTime time.Time
	endTime   time.Time

	// Latency tracking
	latencyHist *hdrhistogram.Histogram

	// Throughput tracking
	throughputWindows []ThroughputWindow

	// Resource tracking
	resourceSnapshots []ResourceSnapshot

	// Error tracking
	errors map[string]int64

	// Custom metrics
	customMetrics map[string]interface{}
}

// ThroughputWindow represents operations count in a time window
type ThroughputWindow struct {
	Timestamp  time.Time
	Operations int64
	Duration   time.Duration
}

// ResourceSnapshot captures system resources at a point in time
type ResourceSnapshot struct {
	Timestamp     time.Time
	CPUPercent    float64
	MemoryMB      uint64
	Goroutines    int
	GCPauseTimeMs float64
	Connections   int
}

// PerformanceReport contains the final performance test results
type PerformanceReport struct {
	TestName    string                 `json:"test_name"`
	StartTime   time.Time              `json:"start_time"`
	EndTime     time.Time              `json:"end_time"`
	Duration    time.Duration          `json:"duration"`
	Metrics     PerformanceMetrics     `json:"metrics"`
	Resources   ResourceMetrics        `json:"resources"`
	Errors      map[string]int64       `json:"errors,omitempty"`
	Custom      map[string]interface{} `json:"custom,omitempty"`
}

// PerformanceMetrics contains operation performance metrics
type PerformanceMetrics struct {
	TotalOperations    int64         `json:"total_operations"`
	SuccessOperations  int64         `json:"success_operations"`
	FailedOperations   int64         `json:"failed_operations"`
	SuccessRate        float64       `json:"success_rate"`
	Throughput         Throughput    `json:"throughput"`
	Latency            Latency       `json:"latency"`
}

// Throughput metrics
type Throughput struct {
	Average float64 `json:"average_ops_per_sec"`
	Peak    float64 `json:"peak_ops_per_sec"`
	P95     float64 `json:"p95_ops_per_sec"`
}

// Latency metrics in milliseconds
type Latency struct {
	Min    float64 `json:"min_ms"`
	Max    float64 `json:"max_ms"`
	Mean   float64 `json:"mean_ms"`
	Median float64 `json:"median_ms"`
	P95    float64 `json:"p95_ms"`
	P99    float64 `json:"p99_ms"`
	P999   float64 `json:"p999_ms"`
}

// ResourceMetrics contains resource usage statistics
type ResourceMetrics struct {
	CPU      ResourceStat `json:"cpu_percent"`
	Memory   ResourceStat `json:"memory_mb"`
	GCPause  ResourceStat `json:"gc_pause_ms"`
}

// ResourceStat contains min/max/avg for a resource metric
type ResourceStat struct {
	Min float64 `json:"min"`
	Max float64 `json:"max"`
	Avg float64 `json:"avg"`
}

// NewMetricsCollector creates a new metrics collector
func NewMetricsCollector() *MetricsCollector {
	return &MetricsCollector{
		latencyHist:       hdrhistogram.New(1, 3600000, 3), // 1ms to 1 hour, 3 significant figures
		errors:            make(map[string]int64),
		customMetrics:     make(map[string]interface{}),
		throughputWindows: make([]ThroughputWindow, 0),
		resourceSnapshots: make([]ResourceSnapshot, 0),
	}
}

// Start begins metrics collection
func (mc *MetricsCollector) Start() {
	mc.mu.Lock()
	mc.startTime = time.Now()
	mc.mu.Unlock()

	// Start resource monitoring
	go mc.monitorResources()
	
	// Start throughput tracking
	go mc.trackThroughput()
}

// Stop ends metrics collection
func (mc *MetricsCollector) Stop() {
	mc.mu.Lock()
	mc.endTime = time.Now()
	mc.mu.Unlock()
}

// RecordOperation records a single operation result
func (mc *MetricsCollector) RecordOperation(latencyMs float64, err error) {
	atomic.AddInt64(&mc.totalOperations, 1)
	
	if err != nil {
		atomic.AddInt64(&mc.failedOperations, 1)
		mc.recordError(err)
	} else {
		atomic.AddInt64(&mc.successOperations, 1)
	}

	mc.mu.Lock()
	mc.latencyHist.RecordValue(int64(latencyMs))
	mc.mu.Unlock()
}

// RecordBatchOperation records results for a batch of operations
func (mc *MetricsCollector) RecordBatchOperation(count int64, totalLatencyMs float64, errors int64) {
	atomic.AddInt64(&mc.totalOperations, count)
	atomic.AddInt64(&mc.successOperations, count-errors)
	atomic.AddInt64(&mc.failedOperations, errors)

	avgLatency := totalLatencyMs / float64(count)
	mc.mu.Lock()
	for i := int64(0); i < count; i++ {
		mc.latencyHist.RecordValue(int64(avgLatency))
	}
	mc.mu.Unlock()
}

// SetCustomMetric sets a custom metric value
func (mc *MetricsCollector) SetCustomMetric(key string, value interface{}) {
	mc.mu.Lock()
	mc.customMetrics[key] = value
	mc.mu.Unlock()
}

// GetReport generates the final performance report
func (mc *MetricsCollector) GetReport(testName string) *PerformanceReport {
	mc.mu.RLock()
	defer mc.mu.RUnlock()

	duration := mc.endTime.Sub(mc.startTime)
	totalOps := atomic.LoadInt64(&mc.totalOperations)
	successOps := atomic.LoadInt64(&mc.successOperations)
	failedOps := atomic.LoadInt64(&mc.failedOperations)

	return &PerformanceReport{
		TestName:  testName,
		StartTime: mc.startTime,
		EndTime:   mc.endTime,
		Duration:  duration,
		Metrics: PerformanceMetrics{
			TotalOperations:   totalOps,
			SuccessOperations: successOps,
			FailedOperations:  failedOps,
			SuccessRate:       float64(successOps) / float64(totalOps) * 100,
			Throughput:        mc.calculateThroughput(),
			Latency:           mc.calculateLatency(),
		},
		Resources: mc.calculateResourceMetrics(),
		Errors:    mc.errors,
		Custom:    mc.customMetrics,
	}
}

// SaveReport saves the report to a JSON file
func (mc *MetricsCollector) SaveReport(testName string, filename string) error {
	report := mc.GetReport(testName)
	
	data, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal report: %w", err)
	}

	return os.WriteFile(filename, data, 0644)
}

// PrintSummary prints a summary of the metrics to stdout
func (mc *MetricsCollector) PrintSummary(testName string) {
	report := mc.GetReport(testName)
	
	fmt.Printf("\n=== Performance Test Summary: %s ===\n", testName)
	fmt.Printf("Duration: %v\n", report.Duration)
	fmt.Printf("Total Operations: %d\n", report.Metrics.TotalOperations)
	fmt.Printf("Success Rate: %.2f%%\n", report.Metrics.SuccessRate)
	fmt.Printf("\nThroughput:\n")
	fmt.Printf("  Average: %.2f ops/sec\n", report.Metrics.Throughput.Average)
	fmt.Printf("  Peak: %.2f ops/sec\n", report.Metrics.Throughput.Peak)
	fmt.Printf("\nLatency:\n")
	fmt.Printf("  Min: %.2f ms\n", report.Metrics.Latency.Min)
	fmt.Printf("  Median: %.2f ms\n", report.Metrics.Latency.Median)
	fmt.Printf("  P95: %.2f ms\n", report.Metrics.Latency.P95)
	fmt.Printf("  P99: %.2f ms\n", report.Metrics.Latency.P99)
	fmt.Printf("  Max: %.2f ms\n", report.Metrics.Latency.Max)
	
	if len(report.Errors) > 0 {
		fmt.Printf("\nErrors:\n")
		for errType, count := range report.Errors {
			fmt.Printf("  %s: %d\n", errType, count)
		}
	}
}

func (mc *MetricsCollector) recordError(err error) {
	errType := fmt.Sprintf("%T", err)
	mc.mu.Lock()
	mc.errors[errType]++
	mc.mu.Unlock()
}

func (mc *MetricsCollector) monitorResources() {
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	var m runtime.MemStats
	for range ticker.C {
		mc.mu.RLock()
		if !mc.endTime.IsZero() {
			mc.mu.RUnlock()
			break
		}
		mc.mu.RUnlock()

		runtime.ReadMemStats(&m)
		
		snapshot := ResourceSnapshot{
			Timestamp:     time.Now(),
			MemoryMB:      m.Alloc / 1024 / 1024,
			Goroutines:    runtime.NumGoroutine(),
			GCPauseTimeMs: float64(m.PauseNs[(m.NumGC+255)%256]) / 1e6,
		}

		mc.mu.Lock()
		mc.resourceSnapshots = append(mc.resourceSnapshots, snapshot)
		mc.mu.Unlock()
	}
}

func (mc *MetricsCollector) trackThroughput() {
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	lastCount := int64(0)
	for range ticker.C {
		mc.mu.RLock()
		if !mc.endTime.IsZero() {
			mc.mu.RUnlock()
			break
		}
		mc.mu.RUnlock()

		currentCount := atomic.LoadInt64(&mc.totalOperations)
		window := ThroughputWindow{
			Timestamp:  time.Now(),
			Operations: currentCount - lastCount,
			Duration:   1 * time.Second,
		}
		lastCount = currentCount

		mc.mu.Lock()
		mc.throughputWindows = append(mc.throughputWindows, window)
		mc.mu.Unlock()
	}
}

func (mc *MetricsCollector) calculateThroughput() Throughput {
	// Calculate overall throughput from total operations and duration
	duration := mc.endTime.Sub(mc.startTime)
	totalOps := atomic.LoadInt64(&mc.totalOperations)
	overallThroughput := float64(totalOps) / duration.Seconds()
	
	if len(mc.throughputWindows) == 0 {
		// If no windows recorded (short test), use overall throughput
		return Throughput{
			Average: overallThroughput,
			Peak:    overallThroughput,
			P95:     overallThroughput,
		}
	}

	var total float64
	var values []float64
	
	for _, window := range mc.throughputWindows {
		ops := float64(window.Operations) / window.Duration.Seconds()
		total += ops
		values = append(values, ops)
	}

	sort.Float64s(values)
	
	return Throughput{
		Average: total / float64(len(values)),
		Peak:    values[len(values)-1],
		P95:     percentile(values, 0.95),
	}
}

func (mc *MetricsCollector) calculateLatency() Latency {
	return Latency{
		Min:    float64(mc.latencyHist.Min()),
		Max:    float64(mc.latencyHist.Max()),
		Mean:   mc.latencyHist.Mean(),
		Median: float64(mc.latencyHist.ValueAtQuantile(50)),
		P95:    float64(mc.latencyHist.ValueAtQuantile(95)),
		P99:    float64(mc.latencyHist.ValueAtQuantile(99)),
		P999:   float64(mc.latencyHist.ValueAtQuantile(99.9)),
	}
}

func (mc *MetricsCollector) calculateResourceMetrics() ResourceMetrics {
	if len(mc.resourceSnapshots) == 0 {
		return ResourceMetrics{}
	}

	var memory []float64
	var gcPause []float64
	
	for _, snapshot := range mc.resourceSnapshots {
		memory = append(memory, float64(snapshot.MemoryMB))
		gcPause = append(gcPause, snapshot.GCPauseTimeMs)
	}

	return ResourceMetrics{
		Memory: calculateResourceStat(memory),
		GCPause: calculateResourceStat(gcPause),
	}
}

func calculateResourceStat(values []float64) ResourceStat {
	if len(values) == 0 {
		return ResourceStat{}
	}

	sort.Float64s(values)
	
	var sum float64
	for _, v := range values {
		sum += v
	}

	return ResourceStat{
		Min: values[0],
		Max: values[len(values)-1],
		Avg: sum / float64(len(values)),
	}
}

func percentile(values []float64, p float64) float64 {
	if len(values) == 0 {
		return 0
	}
	
	index := int(math.Ceil(p * float64(len(values)))) - 1
	if index < 0 {
		index = 0
	}
	if index >= len(values) {
		index = len(values) - 1
	}
	
	return values[index]
}

// OperationTimer helps time individual operations
type OperationTimer struct {
	collector *MetricsCollector
	start     time.Time
}

// StartOperation begins timing an operation
func (mc *MetricsCollector) StartOperation() *OperationTimer {
	return &OperationTimer{
		collector: mc,
		start:     time.Now(),
	}
}

// End completes the operation timing and records the result
func (ot *OperationTimer) End(err error) {
	latencyMs := float64(time.Since(ot.start).Microseconds()) / 1000.0
	ot.collector.RecordOperation(latencyMs, err)
}