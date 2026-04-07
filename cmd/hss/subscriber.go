package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"

	"github.com/spf13/cobra"
)

// subscriberCmd builds the `qcore-hss subscriber ...` command group.
// These talk to a running HSS over HTTP, so users can manage subscribers
// without learning curl.
func subscriberCmd() *cobra.Command {
	var baseURL string

	cmd := &cobra.Command{
		Use:   "subscriber",
		Short: "Manage subscribers on a running HSS",
		Long:  "Thin HTTP client for the HSS REST API. Set --url or QCORE_URL to target a non-local HSS.",
	}
	cmd.PersistentFlags().StringVar(&baseURL, "url", defaultString(os.Getenv("QCORE_URL"), "http://localhost:8080"),
		"HSS base URL (or set QCORE_URL)")

	cmd.AddCommand(subscriberAddCmd(&baseURL))
	cmd.AddCommand(subscriberListCmd(&baseURL))
	cmd.AddCommand(subscriberGetCmd(&baseURL))
	cmd.AddCommand(subscriberAuthCmd(&baseURL))
	cmd.AddCommand(subscriberDeleteCmd(&baseURL))
	return cmd
}

func subscriberAddCmd(baseURL *string) *cobra.Command {
	var imsi, ki, opc, amf, apn string
	c := &cobra.Command{
		Use:   "add",
		Short: "Create a new subscriber",
		Example: `  qcore-hss subscriber add \
    --imsi 001010000000001 \
    --ki   465b5ce8b199b49faa5f0a2ee238a6bc \
    --opc  cd63cb71954a9f4e48a5994e37a02baf`,
		RunE: func(_ *cobra.Command, _ []string) error {
			if imsi == "" || ki == "" || opc == "" {
				return fmt.Errorf("--imsi, --ki, and --opc are required")
			}
			body := map[string]string{"imsi": imsi, "ki": ki, "opc": opc, "amf": amf, "apn": apn}
			return doJSON("POST", *baseURL+"/api/v1/subscribers", body)
		},
	}
	c.Flags().StringVar(&imsi, "imsi", "", "IMSI (15 digits)")
	c.Flags().StringVar(&ki, "ki", "", "Ki (32 hex chars)")
	c.Flags().StringVar(&opc, "opc", "", "OPc (32 hex chars)")
	c.Flags().StringVar(&amf, "amf", "8000", "AMF (4 hex chars)")
	c.Flags().StringVar(&apn, "apn", "internet", "APN")
	return c
}

func subscriberListCmd(baseURL *string) *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List all subscribers",
		RunE: func(_ *cobra.Command, _ []string) error {
			return doJSON("GET", *baseURL+"/api/v1/subscribers", nil)
		},
	}
}

func subscriberGetCmd(baseURL *string) *cobra.Command {
	return &cobra.Command{
		Use:   "get <imsi>",
		Short: "Get a subscriber by IMSI",
		Args:  cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			return doJSON("GET", *baseURL+"/api/v1/subscribers/"+args[0], nil)
		},
	}
}

func subscriberAuthCmd(baseURL *string) *cobra.Command {
	return &cobra.Command{
		Use:   "auth <imsi>",
		Short: "Generate an authentication vector",
		Args:  cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			return doJSON("POST", *baseURL+"/api/v1/subscribers/"+args[0]+"/auth-vector", nil)
		},
	}
}

func subscriberDeleteCmd(baseURL *string) *cobra.Command {
	return &cobra.Command{
		Use:   "delete <imsi>",
		Short: "Delete a subscriber by IMSI",
		Args:  cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			return doJSON("DELETE", *baseURL+"/api/v1/subscribers/"+args[0], nil)
		},
	}
}

// doJSON performs an HTTP request, pretty-prints the response, and returns
// actionable errors for the common cases (HSS down, validation failure).
func doJSON(method, url string, body any) error {
	var reqBody io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("encoding request: %w", err)
		}
		reqBody = bytes.NewReader(b)
	}

	req, err := http.NewRequest(method, url, reqBody)
	if err != nil {
		return fmt.Errorf("building request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("HSS at %s not reachable: %v\n  hint: is it running? try: qcore-hss start --config config.yaml", url, err)
	}
	defer resp.Body.Close()

	out, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("reading response: %w", err)
	}

	// pretty-print JSON if valid
	var pretty bytes.Buffer
	if json.Indent(&pretty, out, "", "  ") == nil {
		fmt.Println(pretty.String())
	} else {
		fmt.Println(string(out))
	}

	if resp.StatusCode >= 400 {
		return fmt.Errorf("HSS returned %d %s", resp.StatusCode, http.StatusText(resp.StatusCode))
	}
	return nil
}

func defaultString(v, fallback string) string {
	if v != "" {
		return v
	}
	return fallback
}
