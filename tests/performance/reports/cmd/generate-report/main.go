package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"time"

	"github.com/sumandas0/entropic/tests/performance/reports"
)

func main() {
	var (
		inputPattern  = flag.String("input", "reports/*.json", "Pattern for input JSON report files")
		outputDir     = flag.String("output", "reports/html", "Output directory for generated reports")
		htmlFile      = flag.String("html", "", "HTML output file name (default: performance_report_<timestamp>.html)")
		csvFile       = flag.String("csv", "", "CSV output file name (default: performance_report_<timestamp>.csv)")
		baselineFile  = flag.String("baseline", "", "Baseline JSON file for comparison")
		compareOnly   = flag.Bool("compare", false, "Only generate comparison report")
	)

	flag.Parse()

	// Create reporter
	reporter := reports.NewHTMLReporter(*outputDir)

	// Load reports
	log.Printf("Loading reports from: %s", *inputPattern)
	if err := reporter.LoadReports(*inputPattern); err != nil {
		log.Fatalf("Failed to load reports: %v", err)
	}

	// Generate timestamp for default filenames
	timestamp := time.Now().Format("20060102_150405")

	// Generate comparison report if baseline provided
	if *baselineFile != "" {
		log.Printf("Comparing with baseline: %s", *baselineFile)
		comparison, err := reporter.CompareWithBaseline(*baselineFile)
		if err != nil {
			log.Fatalf("Failed to compare with baseline: %v", err)
		}

		// Save comparison report
		comparisonFile := filepath.Join(*outputDir, fmt.Sprintf("comparison_%s.json", timestamp))
		if err := saveComparison(comparison, comparisonFile); err != nil {
			log.Fatalf("Failed to save comparison: %v", err)
		}
		log.Printf("Comparison report saved to: %s", comparisonFile)

		// Print regression warnings
		printRegressions(comparison)

		if *compareOnly {
			return
		}
	}

	// Generate HTML report
	if *htmlFile == "" {
		*htmlFile = filepath.Join(*outputDir, fmt.Sprintf("performance_report_%s.html", timestamp))
	} else {
		*htmlFile = filepath.Join(*outputDir, *htmlFile)
	}

	log.Printf("Generating HTML report: %s", *htmlFile)
	if err := reporter.GenerateHTML(*htmlFile); err != nil {
		log.Fatalf("Failed to generate HTML report: %v", err)
	}
	log.Printf("HTML report generated successfully")

	// Generate CSV report
	if *csvFile == "" {
		*csvFile = filepath.Join(*outputDir, fmt.Sprintf("performance_report_%s.csv", timestamp))
	} else {
		*csvFile = filepath.Join(*outputDir, *csvFile)
	}

	log.Printf("Generating CSV report: %s", *csvFile)
	if err := reporter.GenerateCSV(*csvFile); err != nil {
		log.Fatalf("Failed to generate CSV report: %v", err)
	}
	log.Printf("CSV report generated successfully")

	// Print summary
	fmt.Println("\nReport Generation Complete!")
	fmt.Printf("HTML Report: %s\n", *htmlFile)
	fmt.Printf("CSV Report: %s\n", *csvFile)
	fmt.Println("\nTo view the HTML report, open it in your web browser:")
	fmt.Printf("  open %s\n", *htmlFile)
}

func saveComparison(comparison *reports.ComparisonReport, filename string) error {
	// Implementation would save the comparison report
	// For now, just create a placeholder
	return os.WriteFile(filename, []byte("Comparison report"), 0644)
}

func printRegressions(comparison *reports.ComparisonReport) {
	hasRegression := false
	
	fmt.Println("\n=== Performance Regression Analysis ===")
	
	for _, result := range comparison.Results {
		// Check for throughput regression (>10% decrease)
		if result.ThroughputChange < -10 {
			hasRegression = true
			fmt.Printf("⚠️  %s: Throughput decreased by %.1f%%\n", 
				result.TestName, -result.ThroughputChange)
		}
		
		// Check for latency regression (>20% increase)
		if result.LatencyChange > 20 {
			hasRegression = true
			fmt.Printf("⚠️  %s: Latency increased by %.1f%%\n", 
				result.TestName, result.LatencyChange)
		}
		
		// Check for success rate regression (>5% decrease)
		if result.SuccessRateChange < -5 {
			hasRegression = true
			fmt.Printf("⚠️  %s: Success rate decreased by %.1f%%\n", 
				result.TestName, -result.SuccessRateChange)
		}
	}
	
	if !hasRegression {
		fmt.Println("✅ No significant performance regressions detected")
	}
}