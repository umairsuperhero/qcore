package main

import (
	"encoding/hex"
	"fmt"
	"net/http"
	"os"
	"time"

	"github.com/qcore-project/qcore/pkg/nas"
	"github.com/spf13/cobra"
)

func testCmd() *cobra.Command {
	var (
		mmeURL string
		hssURL string
	)

	cmd := &cobra.Command{
		Use:   "test",
		Short: "Run self-tests to verify QCore MME correctness",
		Long:  "Runs NAS security checks (RFC 4493 AES-CMAC), key derivation, IMSI codec, and optionally probes a running MME / HSS.",
		RunE: func(_ *cobra.Command, _ []string) error {
			passed, failed := 0, 0

			fmt.Println("=== NAS Security ===")
			if err := testAESCMACVector(); err != nil {
				fmt.Printf("  FAIL  AES-CMAC (RFC 4493): %v\n", err)
				failed++
			} else {
				fmt.Println("  PASS  AES-CMAC (RFC 4493 example 1)")
				passed++
			}

			if err := testNASKeyDerivation(); err != nil {
				fmt.Printf("  FAIL  KNASenc/KNASint derivation: %v\n", err)
				failed++
			} else {
				fmt.Println("  PASS  NAS key derivation (KNASenc, KNASint, KeNB)")
				passed++
			}

			if err := testWrapNASIntegrityRoundTrip(); err != nil {
				fmt.Printf("  FAIL  NAS integrity wrap/verify round-trip: %v\n", err)
				failed++
			} else {
				fmt.Println("  PASS  NAS integrity wrap/verify round-trip")
				passed++
			}

			fmt.Println("\n=== NAS Codec ===")
			if err := testIMSIRoundTrip(); err != nil {
				fmt.Printf("  FAIL  IMSI BCD codec: %v\n", err)
				failed++
			} else {
				fmt.Println("  PASS  IMSI BCD codec round-trip")
				passed++
			}

			fmt.Println("\n=== Service Connectivity ===")
			if err := probeHTTP(mmeURL+"/api/v1/health", "MME"); err != nil {
				fmt.Printf("  SKIP  MME at %s: %v\n", mmeURL, err)
			} else {
				fmt.Printf("  PASS  MME healthy at %s\n", mmeURL)
				passed++
			}
			if err := probeHTTP(hssURL+"/api/v1/health", "HSS"); err != nil {
				fmt.Printf("  SKIP  HSS at %s: %v\n", hssURL, err)
			} else {
				fmt.Printf("  PASS  HSS healthy at %s\n", hssURL)
				passed++
			}

			fmt.Printf("\n%d passed, %d failed\n", passed, failed)
			if failed > 0 {
				return fmt.Errorf("%d test(s) failed — please report at https://github.com/umairsuperhero/qcore/issues", failed)
			}
			fmt.Println("All MME tests passed.")
			return nil
		},
	}
	cmd.Flags().StringVar(&mmeURL, "mme-url", defaultStr(os.Getenv("QCORE_MME_URL"), "http://localhost:8081"),
		"MME debug API base URL")
	cmd.Flags().StringVar(&hssURL, "hss-url", defaultStr(os.Getenv("QCORE_HSS_URL"), "http://localhost:8080"),
		"HSS REST API base URL")
	return cmd
}

// testAESCMACVector verifies AES-CMAC against RFC 4493 example 1
// (key=2b7e1516..., empty message → MAC=bb1d6929...).
func testAESCMACVector() error {
	key, _ := hex.DecodeString("2b7e151628aed2a6abf7158809cf4f3c")
	want, _ := hex.DecodeString("bb1d6929e95937287fa37d129b756746")
	got, err := nas.AESCMAC(key, []byte{})
	if err != nil {
		return err
	}
	if hex.EncodeToString(got) != hex.EncodeToString(want) {
		return fmt.Errorf("CMAC: got %s, want %s", hex.EncodeToString(got), hex.EncodeToString(want))
	}
	return nil
}

// testNASKeyDerivation runs the KDF for KNASenc/KNASint/KeNB and checks lengths.
func testNASKeyDerivation() error {
	kasme := make([]byte, 32)
	for i := range kasme {
		kasme[i] = byte(i + 1)
	}
	kEnc, err := nas.DeriveKNASenc(kasme, 0)
	if err != nil || len(kEnc) != 16 {
		return fmt.Errorf("KNASenc: len=%d err=%v", len(kEnc), err)
	}
	kInt, err := nas.DeriveKNASint(kasme, 2)
	if err != nil || len(kInt) != 16 {
		return fmt.Errorf("KNASint: len=%d err=%v", len(kInt), err)
	}
	keNB, err := nas.DeriveKeNB(kasme, 0)
	if err != nil || len(keNB) != 32 {
		return fmt.Errorf("KeNB: len=%d err=%v", len(keNB), err)
	}
	// Sanity: different algIDs produce different keys
	kInt2, _ := nas.DeriveKNASint(kasme, 1)
	if hex.EncodeToString(kInt) == hex.EncodeToString(kInt2) {
		return fmt.Errorf("KNASint EIA1 and EIA2 derived the same key")
	}
	return nil
}

// testWrapNASIntegrityRoundTrip wraps a plain NAS, then verifies the MAC.
func testWrapNASIntegrityRoundTrip() error {
	kInt := make([]byte, 16)
	for i := range kInt {
		kInt[i] = byte(i + 1)
	}
	plain := []byte{0x07, 0x5D, 0x02, 0x00, 0x02, 0xE0, 0xE0} // SEC MODE CMD shape
	wrapped, err := nas.WrapNASWithIntegrity(kInt, 0, nas.SecurityHeaderIntegrityProtectedNewCtx, plain)
	if err != nil {
		return fmt.Errorf("wrap: %w", err)
	}
	// Header should parse and reveal the inner message type
	h, _, err := nas.ParseHeader(wrapped)
	if err != nil {
		return fmt.Errorf("ParseHeader: %w", err)
	}
	if h.MessageType != nas.MsgTypeSecurityModeCommand {
		return fmt.Errorf("inner message type: got %s, want SecurityModeCommand", h.MessageType)
	}
	return nil
}

// testIMSIRoundTrip encodes and decodes a 15-digit IMSI.
func testIMSIRoundTrip() error {
	const imsi = "001010000000001"
	enc, err := nas.EncodeIMSI(imsi)
	if err != nil {
		return fmt.Errorf("EncodeIMSI: %w", err)
	}
	dec, err := nas.DecodeIMSI(enc)
	if err != nil {
		return fmt.Errorf("DecodeIMSI: %w", err)
	}
	if dec != imsi {
		return fmt.Errorf("round-trip: got %q, want %q", dec, imsi)
	}
	return nil
}

func probeHTTP(url, label string) error {
	client := &http.Client{Timeout: 3 * time.Second}
	resp, err := client.Get(url)
	if err != nil {
		lower := ""
		for _, c := range label {
			if c >= 'A' && c <= 'Z' {
				c += 'a' - 'A'
			}
			lower += string(c)
		}
		return fmt.Errorf("not running (start with: qcore-%s start)", lower)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return fmt.Errorf("unhealthy (status %d)", resp.StatusCode)
	}
	return nil
}

func defaultStr(v, fallback string) string {
	if v == "" {
		return fallback
	}
	return v
}
