// Command wpt-runner runs curated Web Platform Tests against goosie.
//
// Usage:
//
//	go run ./cmd/wpt-runner [flags]
//
// Flags:
//
//	-config string     path to wpt-config.json (default "testdata/wpt-config.json")
//	-dirs string       comma-separated WPT directories to test (overrides config)
//	-wpt-dir string    path to WPT checkout (overrides config)
//	-binary string     path to goosie binary (overrides config)
//	-output string     output directory for results (overrides config)
//	-download          download WPT checkout before running
//	-threshold float   pixel-match pass threshold (overrides config)
//	-json              write JSON report
//	-html              write HTML dashboard
//	-verbose           print per-test results as they run
package main

import (
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/vyquocvu/goosie/test/wpt"
)

func main() {
	var (
		configPath = flag.String("config", "testdata/wpt-config.json", "path to wpt-config.json")
		dirs       = flag.String("dirs", "", "comma-separated WPT directories to test")
		wptDir     = flag.String("wpt-dir", "", "path to WPT checkout")
		binary     = flag.String("binary", "", "path to goosie binary")
		output     = flag.String("output", "", "output directory for results")
		download   = flag.Bool("download", false, "download WPT checkout before running")
		threshold  = flag.Float64("threshold", 0, "pixel-match pass threshold")
		writeJSON  = flag.Bool("json", true, "write JSON report")
		writeHTML  = flag.Bool("html", true, "write HTML dashboard")
		verbose    = flag.Bool("verbose", false, "print per-test results")
	)
	flag.Parse()

	// Load configuration.
	cfg, err := wpt.LoadConfig(*configPath)
	if err != nil {
		// If the config file doesn't exist, use defaults.
		if os.IsNotExist(err) {
			cfg = wpt.DefaultConfig()
		} else {
			fmt.Fprintf(os.Stderr, "wpt-runner: %v\n", err)
			os.Exit(1)
		}
	}

	// Apply flag overrides.
	if *dirs != "" {
		cfg.Directories = splitTrim(*dirs)
	}
	if *wptDir != "" {
		cfg.WPTDir = *wptDir
	}
	if *binary != "" {
		cfg.GoosieBinary = *binary
	}
	if *output != "" {
		cfg.OutputDir = *output
	}
	if *threshold > 0 {
		cfg.PassThreshold = *threshold
	}

	// Download WPT if requested.
	if *download {
		checkout := wpt.NewCheckout(cfg)
		dir, err := checkout.Ensure()
		if err != nil {
			fmt.Fprintf(os.Stderr, "wpt-runner: checkout: %v\n", err)
			os.Exit(1)
		}
		cfg.WPTDir = dir
	}

	// Run the suite.
	runner := wpt.NewRunner(cfg)
	result, err := runner.Run()
	if err != nil {
		fmt.Fprintf(os.Stderr, "wpt-runner: %v\n", err)
		os.Exit(1)
	}

	// Print results.
	reporter := wpt.NewReporter(result)
	reporter.PrintSummary(os.Stdout)

	if *verbose {
		fmt.Println("\n--- Per-Test Results ---")
		for _, tr := range result.Results {
			status := strings.ToUpper(tr.Status)
			score := ""
			if tr.Score > 0 {
				score = fmt.Sprintf(" (%.1f%%)", tr.Score)
			}
			msg := ""
			if tr.Message != "" {
				msg = " - " + tr.Message
			}
			fmt.Printf("  [%s] %s%s%s\n", status, tr.Path, score, msg)
		}
	}

	// Write reports.
	if *writeJSON {
		jsonPath := cfg.OutputDir + "/results.json"
		if err := reporter.WriteJSONFile(jsonPath); err != nil {
			fmt.Fprintf(os.Stderr, "wpt-runner: write JSON: %v\n", err)
		} else {
			fmt.Printf("\nJSON report: %s\n", jsonPath)
		}
	}
	if *writeHTML {
		htmlPath := cfg.OutputDir + "/report.html"
		if err := reporter.WriteHTMLFile(htmlPath); err != nil {
			fmt.Fprintf(os.Stderr, "wpt-runner: write HTML: %v\n", err)
		} else {
			fmt.Printf("HTML report: %s\n", htmlPath)
		}
	}

	// Exit with non-zero if there were failures.
	if result.Failed > 0 || result.Errored > 0 {
		os.Exit(1)
	}
}

func splitTrim(s string) []string {
	var result []string
	for _, part := range strings.Split(s, ",") {
		trimmed := strings.TrimSpace(part)
		if trimmed != "" {
			result = append(result, trimmed)
		}
	}
	return result
}
