//go:build tools
// +build tools

// Package performance contains tool dependencies for performance testing
// These tools are managed using Go 1.24+ -tool flag feature
package performance

import (
	// Performance profiling and visualization
	_ "github.com/google/pprof"
	
	// HTTP load testing tool
	_ "github.com/tsenart/vegeta/v12"
	
	// Benchmarking and statistics
	_ "github.com/HdrHistogram/hdrhistogram-go"
	
	// Chart generation for reports
	_ "github.com/wcharczuk/go-chart/v2"
)