// Package main implements the OJS conformance gRPC test runner.
//
// This runner executes the same conformance test suites as the HTTP runner,
// but communicates with the OJS server via gRPC instead of HTTP. It maps
// HTTP test actions to equivalent gRPC RPCs and translates assertions from
// HTTP status codes to gRPC status codes.
//
// Usage:
//
//	ojs-conformance-grpc-runner -url localhost:9090 -suites ./suites
//	ojs-conformance-grpc-runner -url localhost:9090 -suites ./suites -level 1
//	ojs-conformance-grpc-runner -url localhost:9090 -suites ./suites -category retry
//	ojs-conformance-grpc-runner -url localhost:9090 -suites ./suites -test L1-RET-001
//	ojs-conformance-grpc-runner -url localhost:9090 -suites ./suites -format json
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/openjobspec/ojs-conformance/lib"
)

const suiteVersion = "1.0.0-rc.1"

func main() {
	var (
		grpcAddr     string
		suitesDir    string
		level        int
		category     string
		testID       string
		outputFormat string
		verbose      bool
		tolerancePct float64
		timeoutSec   int
	)

	flag.StringVar(&grpcAddr, "url", "localhost:9090", "gRPC server address (host:port)")
	flag.StringVar(&suitesDir, "suites", "./suites", "Path to test suite directory")
	flag.IntVar(&level, "level", -1, "Filter by conformance level (0-4), -1 for all")
	flag.StringVar(&category, "category", "", "Filter by category (e.g., envelope, retry)")
	flag.StringVar(&testID, "test", "", "Run a single test by ID (e.g., L0-ENV-001)")
	flag.StringVar(&outputFormat, "format", "table", "Output format: table or json")
	flag.BoolVar(&verbose, "verbose", false, "Show detailed step results")
	flag.Float64Var(&tolerancePct, "tolerance", 50, "Timing tolerance percentage")
	flag.IntVar(&timeoutSec, "timeout", 30, "Per-RPC timeout in seconds")
	flag.Parse()

	// Connect to gRPC server
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	client, err := NewOJSClient(ctx, grpcAddr)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to connect to gRPC server at %s: %v\n", grpcAddr, err)
		os.Exit(2)
	}
	defer client.Close()

	// Load test cases
	tests, err := loadTests(suitesDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error loading tests: %v\n", err)
		os.Exit(2)
	}

	// Filter tests
	tests = filterTests(tests, level, category, testID)
	if len(tests) == 0 {
		fmt.Fprintln(os.Stderr, "No tests match the specified filters.")
		os.Exit(2)
	}

	timingCfg := lib.TimingConfig{
		TolerancePct:   tolerancePct,
		MinToleranceMs: 100,
		MaxWaitMs:      30000,
	}

	rpcTimeout := time.Duration(timeoutSec) * time.Second

	// Run tests
	suiteStart := time.Now()
	var results []lib.TestResult

	for _, tc := range tests {
		result := runTest(tc, client, rpcTimeout, timingCfg, verbose)
		results = append(results, result)
	}

	suiteDuration := time.Since(suiteStart)

	// Build report
	report := buildReport(results, "grpc://"+grpcAddr, level, suiteDuration)

	// Output results
	switch outputFormat {
	case "json":
		outputJSON(report)
	default:
		outputTable(report, results, verbose)
	}

	// Exit code
	if report.Conformant {
		os.Exit(0)
	} else {
		os.Exit(1)
	}
}

// outputJSON writes the report as JSON to stdout.
func outputJSON(report lib.SuiteReport) {
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	enc.Encode(report)
}
