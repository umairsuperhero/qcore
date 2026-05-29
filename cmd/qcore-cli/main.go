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
	fmt.Fprintf(os.Stderr, "  test run --scenario <file.yaml> [--dashboard http://localhost:3000]\n")
}

func runTest() {
	testCmd := flag.NewFlagSet("test run", flag.ExitOnError)
	scenarioFile := testCmd.String("scenario", "", "Path to custom scenario YAML")
	dashboardURL := testCmd.String("dashboard", "http://localhost:3000", "Dashboard URL")

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

	fmt.Printf("Injecting scenario from %s...\n", *scenarioFile)

	// Inject the custom scenario
	postURL := fmt.Sprintf("%s/api/simulator/custom", *dashboardURL)
	resp, err := http.Post(postURL, "application/yaml", bytes.NewReader(yamlData))
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to contact dashboard: %v\n", err)
		os.Exit(1)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusAccepted {
		body, _ := io.ReadAll(resp.Body)
		fmt.Fprintf(os.Stderr, "Dashboard rejected scenario (status %d): %s\n", resp.StatusCode, string(body))
		os.Exit(1)
	}

	// Poll until done
	statusURL := fmt.Sprintf("%s/api/simulator/status", *dashboardURL)
	client := &http.Client{Timeout: 2 * time.Second}

	for {
		time.Sleep(1 * time.Second)
		req, err := http.NewRequest(http.MethodGet, statusURL, nil)
		if err != nil {
			continue
		}
		res, err := client.Do(req)
		if err != nil {
			continue
		}
		
		var status dashboard.SimulatorStatus
		if err := json.NewDecoder(res.Body).Decode(&status); err != nil {
			res.Body.Close()
			continue
		}
		res.Body.Close()

		if status.State == dashboard.SimulatorSuccess {
			fmt.Println("✅ Scenario completed successfully.")
			os.Exit(0)
		} else if status.State == dashboard.SimulatorFailed {
			fmt.Printf("❌ Scenario failed at step: %s\n", status.FailedStep)
			if status.LastError != "" {
				fmt.Printf("   Error: %s\n", status.LastError)
			}
			os.Exit(1)
		}
		// Still running...
	}
}
