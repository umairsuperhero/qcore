package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"

	"github.com/qcore-project/qcore/pkg/dashboard"
)

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(1)
	}

	switch os.Args[1] {
	case "test":
		runTest()
	default:
		usage()
		os.Exit(1)
	}
}

func usage() {
	fmt.Fprintf(os.Stderr, "Usage: qcore-cli [command]\n\n")
	fmt.Fprintf(os.Stderr, "Commands:\n")
	fmt.Fprintf(os.Stderr, "  test run --scenario <file.yaml> [--dashboard http://localhost:3000] [--json]\n")
}

func runTest() {
	testCmd := flag.NewFlagSet("test run", flag.ExitOnError)
	scenarioFile := testCmd.String("scenario", "", "Path to custom scenario YAML")
	dashboardURL := testCmd.String("dashboard", "http://localhost:3000", "Dashboard URL")
	jsonOut := testCmd.Bool("json", false, "Output machine-readable JSON result")

	// Parse flags starting after `test run`
	if len(os.Args) < 3 || os.Args[2] != "run" {
		usage()
		os.Exit(1)
	}
	testCmd.Parse(os.Args[3:])

	if *scenarioFile == "" {
		fmt.Fprintln(os.Stderr, "Error: --scenario is required")
		testCmd.PrintDefaults()
		os.Exit(1)
	}

	yamlData, err := os.ReadFile(*scenarioFile)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to read scenario file: %v\n", err)
		os.Exit(1)
	}

	if !*jsonOut {
		fmt.Printf("Running scenario from %s...\n", *scenarioFile)
	}

	postURL := fmt.Sprintf("%s/api/scenarios/run", *dashboardURL)
	req, err := http.NewRequest(http.MethodPost, postURL, bytes.NewReader(yamlData))
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to create request: %v\n", err)
		os.Exit(1)
	}
	req.Header.Set("Content-Type", "application/yaml")

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to contact dashboard: %v\n", err)
		os.Exit(1)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		fmt.Fprintf(os.Stderr, "Dashboard rejected scenario (status %d): %s\n", resp.StatusCode, string(body))
		os.Exit(1)
	}

	var result dashboard.ScenarioRunResult
	body, _ := io.ReadAll(resp.Body)
	if err := json.Unmarshal(body, &result); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to parse dashboard response: %v\n", err)
		os.Exit(1)
	}

	if *jsonOut {
		var prettyJSON bytes.Buffer
		_ = json.Indent(&prettyJSON, body, "", "  ")
		fmt.Println(prettyJSON.String())
	} else {
		if result.Pass != nil {
			if *result.Pass {
				fmt.Println("✅ Scenario PASS")
			} else {
				fmt.Println("❌ Scenario FAIL")
				fmt.Printf("   Expected: result=%s cause=%s failed_step=%s\n",
					result.Expected.Result, result.Expected.Cause, result.Expected.FailedStep)
				fmt.Printf("   Actual:   result=%s cause=%s failed_step=%s\n",
					result.Actual.Result, result.Actual.Cause, result.Actual.FailedStep)
			}
		} else {
			if result.Actual.Result == "success" {
				fmt.Println("✅ Scenario completed successfully (no expect block).")
			} else {
				fmt.Printf("❌ Scenario failed at step: %s\n", result.Actual.FailedStep)
				if result.Actual.Error != "" {
					fmt.Printf("   Error: %s\n", result.Actual.Error)
				}
			}
		}
	}

	passed := false
	if result.Pass != nil {
		passed = *result.Pass
	} else {
		passed = (result.Actual.Result == "success")
	}

	if !passed {
		os.Exit(1)
	}
	os.Exit(0)
}
