# Performance Testing Makefile for Entropic

# Variables
REPORTS_DIR := reports
PROFILE_DIR := $(REPORTS_DIR)/profiles
HTML_DIR := $(REPORTS_DIR)/html
BASELINE_FILE := $(REPORTS_DIR)/baseline.json

# Go 1.24+ tool commands
GO := go
PPROF := $(GO) tool pprof
VEGETA := $(GO) run github.com/tsenart/vegeta/v12

# Test timeout
TEST_TIMEOUT := 30m

# Colors for output
GREEN := \033[0;32m
YELLOW := \033[0;33m
RED := \033[0;31m
NC := \033[0m # No Color

.PHONY: all clean setup tools ingestion bulk bottleneck profile report baseline compare help

# Default target
all: setup ingestion report

# Help target
help:
	@echo "$(GREEN)Entropic Performance Testing$(NC)"
	@echo ""
	@echo "Available targets:"
	@echo "  $(YELLOW)setup$(NC)       - Create necessary directories"
	@echo "  $(YELLOW)tools$(NC)       - Install Go 1.24+ tool dependencies"
	@echo "  $(YELLOW)ingestion$(NC)   - Run entity ingestion performance tests"
	@echo "  $(YELLOW)bulk$(NC)        - Run bulk ingestion performance tests"
	@echo "  $(YELLOW)bottleneck$(NC)  - Run bottleneck analysis tests"
	@echo "  $(YELLOW)profile$(NC)     - Run tests with CPU and memory profiling"
	@echo "  $(YELLOW)report$(NC)      - Generate HTML and CSV reports"
	@echo "  $(YELLOW)baseline$(NC)    - Save current results as baseline"
	@echo "  $(YELLOW)compare$(NC)     - Compare current results with baseline"
	@echo "  $(YELLOW)load-test$(NC)   - Run load test with Vegeta"
	@echo "  $(YELLOW)all$(NC)         - Run setup, ingestion tests, and generate report"
	@echo "  $(YELLOW)clean$(NC)       - Clean up generated files"

# Setup directories
setup:
	@echo "$(GREEN)Setting up directories...$(NC)"
	@mkdir -p $(REPORTS_DIR)
	@mkdir -p $(PROFILE_DIR)
	@mkdir -p $(HTML_DIR)
	@echo "$(GREEN)Setup complete$(NC)"

# Install tools using Go 1.24+ -tool flag
tools:
	@echo "$(GREEN)Installing performance testing tools...$(NC)"
	@$(GO) get -tool github.com/google/pprof@latest
	@$(GO) get -tool github.com/tsenart/vegeta/v12@latest
	@$(GO) get -tool github.com/codahale/hdrhistogram@latest
	@$(GO) get -tool github.com/wcharczuk/go-chart/v2@latest
	@echo "$(GREEN)Tools installed$(NC)"

# Run entity ingestion tests
ingestion: setup
	@echo "$(GREEN)Running entity ingestion performance tests...$(NC)"
	@$(GO) test -timeout=$(TEST_TIMEOUT) -v ./ingestion -run TestEntityIngestion
	@$(GO) test -timeout=$(TEST_TIMEOUT) -v ./ingestion -run TestConcurrentEntityIngestion
	@$(GO) test -timeout=$(TEST_TIMEOUT) -v ./ingestion -run TestSustainedIngestionLoad
	@$(GO) test -timeout=$(TEST_TIMEOUT) -v ./ingestion -run TestMixedEntityTypeIngestion
	@echo "$(GREEN)Entity ingestion tests complete$(NC)"

# Run bulk ingestion tests
bulk: setup
	@echo "$(GREEN)Running bulk ingestion performance tests...$(NC)"
	@$(GO) test -timeout=$(TEST_TIMEOUT) -v ./ingestion -run TestBulkIngestion
	@$(GO) test -timeout=$(TEST_TIMEOUT) -v ./ingestion -run TestStreamingIngestion
	@$(GO) test -timeout=$(TEST_TIMEOUT) -v ./ingestion -run TestParallelBulkOperations
	@$(GO) test -timeout=$(TEST_TIMEOUT) -v ./ingestion -run TestMemoryEfficientBulkLoading
	@echo "$(GREEN)Bulk ingestion tests complete$(NC)"

# Run bottleneck analysis
bottleneck: setup
	@echo "$(GREEN)Running bottleneck analysis tests...$(NC)"
	@$(GO) test -timeout=$(TEST_TIMEOUT) -v ./ingestion -run TestDatabaseOnlyIngestion
	@$(GO) test -timeout=$(TEST_TIMEOUT) -v ./ingestion -run TestIndexOnlyIngestion
	@$(GO) test -timeout=$(TEST_TIMEOUT) -v ./ingestion -run TestComponentIsolation
	@$(GO) test -timeout=$(TEST_TIMEOUT) -v ./ingestion -run TestConnectionPoolExhaustion
	@echo "$(GREEN)Bottleneck analysis complete$(NC)"

# Run with profiling
profile: setup
	@echo "$(GREEN)Running tests with profiling enabled...$(NC)"
	@$(GO) test -timeout=$(TEST_TIMEOUT) -v ./ingestion -run TestCPUProfiledIngestion
	@$(GO) test -timeout=$(TEST_TIMEOUT) -v ./ingestion -run TestMemoryProfiledIngestion
	@echo "$(GREEN)Profiling complete. Check $(PROFILE_DIR) for .prof files$(NC)"
	@echo "To analyze CPU profile: $(PPROF) -http=:8080 $(REPORTS_DIR)/cpu_profile_*.prof"
	@echo "To analyze memory profile: $(PPROF) -http=:8081 $(REPORTS_DIR)/mem_profile_*.prof"

# Generate reports
report:
	@echo "$(GREEN)Generating performance reports...$(NC)"
	@$(GO) run ./reports/cmd/generate-report \
		-input="$(REPORTS_DIR)/*.json" \
		-output="$(HTML_DIR)"
	@echo "$(GREEN)Reports generated in $(HTML_DIR)$(NC)"
	@ls -la $(HTML_DIR)/*.html 2>/dev/null || echo "$(RED)No HTML reports found$(NC)"

# Save baseline
baseline:
	@echo "$(GREEN)Saving current results as baseline...$(NC)"
	@cat $(REPORTS_DIR)/*.json > $(BASELINE_FILE) 2>/dev/null || echo "$(RED)No reports to baseline$(NC)"
	@echo "$(GREEN)Baseline saved to $(BASELINE_FILE)$(NC)"

# Compare with baseline
compare:
	@echo "$(GREEN)Comparing with baseline...$(NC)"
	@if [ -f $(BASELINE_FILE) ]; then \
		$(GO) run ./reports/cmd/generate-report \
			-input="$(REPORTS_DIR)/*.json" \
			-output="$(HTML_DIR)" \
			-baseline="$(BASELINE_FILE)"; \
	else \
		echo "$(RED)No baseline found. Run 'make baseline' first$(NC)"; \
	fi

# Load test with Vegeta
load-test: setup
	@echo "$(GREEN)Running load test with Vegeta...$(NC)"
	@echo "POST http://localhost:8080/entities/user" | \
		$(VEGETA) attack -duration=30s -rate=100 | \
		$(VEGETA) report | tee $(REPORTS_DIR)/vegeta_report.txt
	@echo "$(GREEN)Load test complete$(NC)"

# Run all performance tests
test-all: ingestion bulk bottleneck profile
	@echo "$(GREEN)All performance tests complete$(NC)"

# Benchmark specific tests
bench-entity:
	@$(GO) test -bench=BenchmarkEntityCreation -benchmem -benchtime=10s ./ingestion

bench-concurrent:
	@$(GO) test -bench=BenchmarkConcurrentEntityIngestion -benchmem -benchtime=10s ./ingestion

bench-bulk:
	@$(GO) test -bench=BenchmarkBulkIngestion -benchmem -benchtime=10s ./ingestion

# Clean up
clean:
	@echo "$(YELLOW)Cleaning up generated files...$(NC)"
	@rm -rf $(REPORTS_DIR)/*.json
	@rm -rf $(PROFILE_DIR)/*.prof
	@rm -rf $(HTML_DIR)/*.html
	@rm -rf $(HTML_DIR)/*.csv
	@echo "$(GREEN)Cleanup complete$(NC)"

# Quick performance check
quick-check: setup
	@echo "$(GREEN)Running quick performance check...$(NC)"
	@$(GO) test -timeout=5m -v ./ingestion -run TestEntityIngestionPerformance -short
	@$(MAKE) report

# CI/CD target
ci: setup tools test-all report compare
	@echo "$(GREEN)CI performance tests complete$(NC)"
	@if [ -f $(HTML_DIR)/comparison_*.json ]; then \
		echo "$(YELLOW)Check comparison report for regressions$(NC)"; \
	fi

# Docker-based testing
docker-test:
	@echo "$(GREEN)Running performance tests in Docker...$(NC)"
	@docker-compose -f ../../docker-compose.test.yml up -d
	@sleep 10 # Wait for services
	@$(MAKE) test-all
	@docker-compose -f ../../docker-compose.test.yml down

# Generate flame graph from CPU profile
flamegraph:
	@echo "$(GREEN)Generating flame graph...$(NC)"
	@latest_prof=$$(ls -t $(REPORTS_DIR)/cpu_profile_*.prof 2>/dev/null | head -1); \
	if [ -n "$$latest_prof" ]; then \
		$(PPROF) -http=:8080 $$latest_prof; \
	else \
		echo "$(RED)No CPU profile found. Run 'make profile' first$(NC)"; \
	fi