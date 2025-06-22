package reports

import (
	"bytes"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"html/template"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/sumandas0/entropic/tests/performance/utils"
	"github.com/wcharczuk/go-chart/v2"
	"github.com/wcharczuk/go-chart/v2/drawing"
)

// HTMLReporter generates HTML reports from performance test results
type HTMLReporter struct {
	outputDir string
	reports   []*utils.PerformanceReport
}

// NewHTMLReporter creates a new HTML reporter
func NewHTMLReporter(outputDir string) *HTMLReporter {
	return &HTMLReporter{
		outputDir: outputDir,
		reports:   make([]*utils.PerformanceReport, 0),
	}
}

// AddReport adds a performance report to the reporter
func (hr *HTMLReporter) AddReport(report *utils.PerformanceReport) {
	hr.reports = append(hr.reports, report)
}

// LoadReports loads reports from JSON files
func (hr *HTMLReporter) LoadReports(pattern string) error {
	files, err := filepath.Glob(pattern)
	if err != nil {
		return fmt.Errorf("failed to find report files: %w", err)
	}

	for _, file := range files {
		data, err := os.ReadFile(file)
		if err != nil {
			return fmt.Errorf("failed to read report %s: %w", file, err)
		}

		var report utils.PerformanceReport
		if err := json.Unmarshal(data, &report); err != nil {
			return fmt.Errorf("failed to parse report %s: %w", file, err)
		}

		hr.reports = append(hr.reports, &report)
	}

	return nil
}

// GenerateHTML creates an HTML report with all loaded reports
func (hr *HTMLReporter) GenerateHTML(outputFile string) error {
	// Sort reports by start time
	sort.Slice(hr.reports, func(i, j int) bool {
		return hr.reports[i].StartTime.Before(hr.reports[j].StartTime)
	})

	// Generate charts
	throughputChart, err := hr.generateThroughputChart()
	if err != nil {
		return fmt.Errorf("failed to generate throughput chart: %w", err)
	}

	latencyChart, err := hr.generateLatencyChart()
	if err != nil {
		return fmt.Errorf("failed to generate latency chart: %w", err)
	}

	// Create HTML from template
	tmpl := template.Must(template.New("report").Parse(htmlTemplate))

	var buf bytes.Buffer
	data := struct {
		Title           string
		GeneratedAt     string
		Reports         []*utils.PerformanceReport
		ThroughputChart template.HTML
		LatencyChart    template.HTML
		Summary         Summary
	}{
		Title:           "Entropic Performance Test Report",
		GeneratedAt:     time.Now().Format("2006-01-02 15:04:05"),
		Reports:         hr.reports,
		ThroughputChart: template.HTML(throughputChart),
		LatencyChart:    template.HTML(latencyChart),
		Summary:         hr.generateSummary(),
	}

	if err := tmpl.Execute(&buf, data); err != nil {
		return fmt.Errorf("failed to execute template: %w", err)
	}

	// Write to file
	if err := os.MkdirAll(filepath.Dir(outputFile), 0755); err != nil {
		return fmt.Errorf("failed to create output directory: %w", err)
	}

	if err := os.WriteFile(outputFile, buf.Bytes(), 0644); err != nil {
		return fmt.Errorf("failed to write HTML file: %w", err)
	}

	return nil
}

// GenerateCSV exports reports to CSV format
func (hr *HTMLReporter) GenerateCSV(outputFile string) error {
	file, err := os.Create(outputFile)
	if err != nil {
		return fmt.Errorf("failed to create CSV file: %w", err)
	}
	defer file.Close()

	writer := csv.NewWriter(file)
	defer writer.Flush()

	// Write header
	header := []string{
		"Test Name",
		"Start Time",
		"Duration (s)",
		"Total Operations",
		"Success Rate (%)",
		"Avg Throughput (ops/s)",
		"Peak Throughput (ops/s)",
		"Min Latency (ms)",
		"Median Latency (ms)",
		"P95 Latency (ms)",
		"P99 Latency (ms)",
		"Max Latency (ms)",
		"Avg Memory (MB)",
		"Max Memory (MB)",
	}

	if err := writer.Write(header); err != nil {
		return fmt.Errorf("failed to write CSV header: %w", err)
	}

	// Write data rows
	for _, report := range hr.reports {
		row := []string{
			report.TestName,
			report.StartTime.Format("2006-01-02 15:04:05"),
			fmt.Sprintf("%.2f", report.Duration.Seconds()),
			fmt.Sprintf("%d", report.Metrics.TotalOperations),
			fmt.Sprintf("%.2f", report.Metrics.SuccessRate),
			fmt.Sprintf("%.2f", report.Metrics.Throughput.Average),
			fmt.Sprintf("%.2f", report.Metrics.Throughput.Peak),
			fmt.Sprintf("%.2f", report.Metrics.Latency.Min),
			fmt.Sprintf("%.2f", report.Metrics.Latency.Median),
			fmt.Sprintf("%.2f", report.Metrics.Latency.P95),
			fmt.Sprintf("%.2f", report.Metrics.Latency.P99),
			fmt.Sprintf("%.2f", report.Metrics.Latency.Max),
			fmt.Sprintf("%.2f", report.Resources.Memory.Avg),
			fmt.Sprintf("%.2f", report.Resources.Memory.Max),
		}

		if err := writer.Write(row); err != nil {
			return fmt.Errorf("failed to write CSV row: %w", err)
		}
	}

	return nil
}

// CompareWithBaseline compares current results with baseline
func (hr *HTMLReporter) CompareWithBaseline(baselineFile string) (*ComparisonReport, error) {
	// Load baseline
	baselineData, err := os.ReadFile(baselineFile)
	if err != nil {
		return nil, fmt.Errorf("failed to read baseline: %w", err)
	}

	var baseline []*utils.PerformanceReport
	if err := json.Unmarshal(baselineData, &baseline); err != nil {
		return nil, fmt.Errorf("failed to parse baseline: %w", err)
	}

	comparison := &ComparisonReport{
		Timestamp: time.Now(),
		Results:   make([]ComparisonResult, 0),
	}

	// Compare each test
	for _, current := range hr.reports {
		// Find matching baseline
		var base *utils.PerformanceReport
		for _, b := range baseline {
			if b.TestName == current.TestName {
				base = b
				break
			}
		}

		if base == nil {
			continue
		}

		result := ComparisonResult{
			TestName: current.TestName,
			Current:  current,
			Baseline: base,
		}

		// Calculate changes
		result.ThroughputChange = (current.Metrics.Throughput.Average - base.Metrics.Throughput.Average) /
			base.Metrics.Throughput.Average * 100
		result.LatencyChange = (current.Metrics.Latency.P95 - base.Metrics.Latency.P95) /
			base.Metrics.Latency.P95 * 100
		result.SuccessRateChange = current.Metrics.SuccessRate - base.Metrics.SuccessRate

		comparison.Results = append(comparison.Results, result)
	}

	return comparison, nil
}

// Private methods

func (hr *HTMLReporter) generateThroughputChart() (string, error) {
	if len(hr.reports) == 0 {
		return "", nil
	}

	// Create bars
	var bars []chart.Value
	for i, report := range hr.reports {
		bars = append(bars, chart.Value{
			Label: truncateString(report.TestName, 20),
			Value: report.Metrics.Throughput.Average,
			Style: chart.Style{
				FillColor:   getColor(i),
				StrokeColor: getColor(i),
			},
		})
	}

	// Create a bar chart directly
	barChart := chart.BarChart{
		Title:      "Throughput Comparison",
		TitleStyle: chart.Shown(),
		Background: chart.Style{
			Padding: chart.Box{
				Top:    20,
				Left:   20,
				Right:  20,
				Bottom: 20,
			},
		},
		Width:      800,
		Height:     400,
		BarWidth:   60,
		BarSpacing: 40,
		Bars:       bars,
		XAxis:      chart.Shown(),
		YAxis: chart.YAxis{
			Name:      "Operations/Second",
			NameStyle: chart.Shown(),
			Style:     chart.Shown(),
		},
	}

	buffer := bytes.NewBuffer([]byte{})
	err := barChart.Render(chart.SVG, buffer)
	if err != nil {
		return "", err
	}

	return buffer.String(), nil
}

func (hr *HTMLReporter) generateLatencyChart() (string, error) {
	if len(hr.reports) == 0 {
		return "", nil
	}

	graph := chart.Chart{
		Title:      "Latency Distribution",
		TitleStyle: chart.Shown(),
		Background: chart.Style{
			Padding: chart.Box{
				Top:    20,
				Left:   20,
				Right:  20,
				Bottom: 20,
			},
		},
		XAxis: chart.XAxis{
			Name:      "Test",
			NameStyle: chart.Shown(),
			Style:     chart.Shown(),
		},
		YAxis: chart.YAxis{
			Name:      "Latency (ms)",
			NameStyle: chart.Shown(),
			Style:     chart.Shown(),
		},
	}

	// Create series for different percentiles
	var p50Values, p95Values, p99Values []float64
	var labels []string

	for _, report := range hr.reports {
		p50Values = append(p50Values, report.Metrics.Latency.Median)
		p95Values = append(p95Values, report.Metrics.Latency.P95)
		p99Values = append(p99Values, report.Metrics.Latency.P99)
		labels = append(labels, truncateString(report.TestName, 15))
	}

	graph.Series = []chart.Series{
		chart.ContinuousSeries{
			Name: "P50",
			Style: chart.Style{
				StrokeColor: drawing.ColorBlue,
				StrokeWidth: 2,
			},
			XValues: generateSequence(len(hr.reports)),
			YValues: p50Values,
		},
		chart.ContinuousSeries{
			Name: "P95",
			Style: chart.Style{
				StrokeColor: drawing.Color{R: 255, G: 165, B: 0, A: 255}, // Orange
				StrokeWidth: 2,
			},
			XValues: generateSequence(len(hr.reports)),
			YValues: p95Values,
		},
		chart.ContinuousSeries{
			Name: "P99",
			Style: chart.Style{
				StrokeColor: drawing.ColorRed,
				StrokeWidth: 2,
			},
			XValues: generateSequence(len(hr.reports)),
			YValues: p99Values,
		},
	}

	graph.Elements = []chart.Renderable{
		chart.Legend(&graph),
	}

	buffer := bytes.NewBuffer([]byte{})
	err := graph.Render(chart.SVG, buffer)
	if err != nil {
		return "", err
	}

	return buffer.String(), nil
}

func (hr *HTMLReporter) generateSummary() Summary {
	if len(hr.reports) == 0 {
		return Summary{}
	}

	var totalOps int64
	var totalDuration time.Duration
	var avgThroughput, avgLatency float64
	var minLatency, maxLatency float64 = 999999, 0
	var successRates []float64

	for _, report := range hr.reports {
		totalOps += report.Metrics.TotalOperations
		totalDuration += report.Duration
		avgThroughput += report.Metrics.Throughput.Average
		avgLatency += report.Metrics.Latency.Median
		successRates = append(successRates, report.Metrics.SuccessRate)

		if report.Metrics.Latency.Min < minLatency {
			minLatency = report.Metrics.Latency.Min
		}
		if report.Metrics.Latency.Max > maxLatency {
			maxLatency = report.Metrics.Latency.Max
		}
	}

	n := float64(len(hr.reports))

	// Calculate average success rate
	var avgSuccessRate float64
	for _, rate := range successRates {
		avgSuccessRate += rate
	}
	avgSuccessRate /= n

	return Summary{
		TotalTests:      len(hr.reports),
		TotalOperations: totalOps,
		TotalDuration:   totalDuration,
		AvgThroughput:   avgThroughput / n,
		AvgLatency:      avgLatency / n,
		MinLatency:      minLatency,
		MaxLatency:      maxLatency,
		AvgSuccessRate:  avgSuccessRate,
	}
}

// Helper types

type Summary struct {
	TotalTests      int
	TotalOperations int64
	TotalDuration   time.Duration
	AvgThroughput   float64
	AvgLatency      float64
	MinLatency      float64
	MaxLatency      float64
	AvgSuccessRate  float64
}

type ComparisonReport struct {
	Timestamp time.Time
	Results   []ComparisonResult
}

type ComparisonResult struct {
	TestName          string
	Current           *utils.PerformanceReport
	Baseline          *utils.PerformanceReport
	ThroughputChange  float64 // percentage
	LatencyChange     float64 // percentage
	SuccessRateChange float64 // absolute
}

// Helper functions

func truncateString(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max-3] + "..."
}

func getColor(index int) drawing.Color {
	colors := []drawing.Color{
		drawing.ColorBlue,
		drawing.ColorGreen,
		drawing.ColorRed,
		drawing.Color{R: 255, G: 165, B: 0, A: 255}, // Orange
		drawing.ColorPurple,
		drawing.Color{R: 0, G: 255, B: 255, A: 255}, // Cyan
	}
	return colors[index%len(colors)]
}

func generateSequence(n int) []float64 {
	seq := make([]float64, n)
	for i := range seq {
		seq[i] = float64(i)
	}
	return seq
}

// HTML template
const htmlTemplate = `<!DOCTYPE html>
<html>
<head>
    <title>{{.Title}}</title>
    <style>
        body {
            font-family: Arial, sans-serif;
            margin: 20px;
            background-color: #f5f5f5;
        }
        .container {
            max-width: 1200px;
            margin: 0 auto;
            background-color: white;
            padding: 20px;
            border-radius: 8px;
            box-shadow: 0 2px 4px rgba(0,0,0,0.1);
        }
        h1, h2 {
            color: #333;
        }
        .summary {
            background-color: #e8f4f8;
            padding: 15px;
            border-radius: 5px;
            margin-bottom: 20px;
        }
        .summary-grid {
            display: grid;
            grid-template-columns: repeat(auto-fit, minmax(200px, 1fr));
            gap: 15px;
        }
        .summary-item {
            background-color: white;
            padding: 10px;
            border-radius: 5px;
            text-align: center;
        }
        .summary-value {
            font-size: 24px;
            font-weight: bold;
            color: #2196F3;
        }
        .summary-label {
            font-size: 12px;
            color: #666;
            margin-top: 5px;
        }
        table {
            width: 100%;
            border-collapse: collapse;
            margin-top: 20px;
        }
        th, td {
            padding: 10px;
            text-align: left;
            border-bottom: 1px solid #ddd;
        }
        th {
            background-color: #f8f9fa;
            font-weight: bold;
        }
        tr:hover {
            background-color: #f5f5f5;
        }
        .chart-container {
            margin: 20px 0;
            text-align: center;
        }
        .metadata {
            color: #666;
            font-size: 14px;
            margin-bottom: 20px;
        }
        .success { color: #4CAF50; }
        .warning { color: #FF9800; }
        .error { color: #F44336; }
    </style>
</head>
<body>
    <div class="container">
        <h1>{{.Title}}</h1>
        <div class="metadata">Generated at: {{.GeneratedAt}}</div>
        
        <div class="summary">
            <h2>Executive Summary</h2>
            <div class="summary-grid">
                <div class="summary-item">
                    <div class="summary-value">{{.Summary.TotalTests}}</div>
                    <div class="summary-label">Total Tests</div>
                </div>
                <div class="summary-item">
                    <div class="summary-value">{{.Summary.TotalOperations}}</div>
                    <div class="summary-label">Total Operations</div>
                </div>
                <div class="summary-item">
                    <div class="summary-value">{{printf "%.2f" .Summary.AvgThroughput}}</div>
                    <div class="summary-label">Avg Throughput (ops/s)</div>
                </div>
                <div class="summary-item">
                    <div class="summary-value">{{printf "%.2f" .Summary.AvgLatency}}</div>
                    <div class="summary-label">Avg Latency (ms)</div>
                </div>
                <div class="summary-item">
                    <div class="summary-value {{if ge .Summary.AvgSuccessRate 95.0}}success{{else if ge .Summary.AvgSuccessRate 90.0}}warning{{else}}error{{end}}">
                        {{printf "%.1f%%" .Summary.AvgSuccessRate}}
                    </div>
                    <div class="summary-label">Avg Success Rate</div>
                </div>
            </div>
        </div>

        <div class="chart-container">
            <h2>Throughput Comparison</h2>
            {{.ThroughputChart}}
        </div>

        <div class="chart-container">
            <h2>Latency Distribution</h2>
            {{.LatencyChart}}
        </div>

        <h2>Detailed Results</h2>
        <table>
            <thead>
                <tr>
                    <th>Test Name</th>
                    <th>Duration</th>
                    <th>Operations</th>
                    <th>Success Rate</th>
                    <th>Avg Throughput</th>
                    <th>P50 Latency</th>
                    <th>P95 Latency</th>
                    <th>P99 Latency</th>
                </tr>
            </thead>
            <tbody>
                {{range .Reports}}
                <tr>
                    <td>{{.TestName}}</td>
                    <td>{{.Duration}}</td>
                    <td>{{.Metrics.TotalOperations}}</td>
                    <td class="{{if ge .Metrics.SuccessRate 95.0}}success{{else if ge .Metrics.SuccessRate 90.0}}warning{{else}}error{{end}}">
                        {{printf "%.1f%%" .Metrics.SuccessRate}}
                    </td>
                    <td>{{printf "%.2f" .Metrics.Throughput.Average}} ops/s</td>
                    <td>{{printf "%.2f" .Metrics.Latency.Median}} ms</td>
                    <td>{{printf "%.2f" .Metrics.Latency.P95}} ms</td>
                    <td>{{printf "%.2f" .Metrics.Latency.P99}} ms</td>
                </tr>
                {{end}}
            </tbody>
        </table>
    </div>
</body>
</html>`
