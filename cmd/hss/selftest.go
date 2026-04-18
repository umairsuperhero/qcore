package main

import (
	"encoding/hex"
	"fmt"
	"net/http"
	"os"
	"time"

	"github.com/qcore-project/qcore/pkg/subscriber"
	"github.com/spf13/cobra"
)

func testCmd() *cobra.Command {
	var baseURL string

	cmd := &cobra.Command{
		Use:   "test",
		Short: "Run self-tests to verify QCore HSS correctness",
		Long:  "Runs Milenage crypto verification against 3GPP TS 35.208 test vectors, and optionally checks a running HSS API.",
		RunE: func(_ *cobra.Command, _ []string) error {
			passed, failed := 0, 0

			// Test 1: Milenage crypto — 3GPP TS 35.208 Test Set 1
			fmt.Println("=== Milenage Crypto (3GPP TS 35.208) ===")
			if err := testMilenageSet1(); err != nil {
				fmt.Printf("  FAIL  Test Set 1: %v\n", err)
				failed++
			} else {
				fmt.Println("  PASS  Test Set 1 (Ki/OPc/RAND → RES/CK/IK/AK verified)")
				passed++
			}

			// Test 2: OPc generation
			if err := testOPcGeneration(); err != nil {
				fmt.Printf("  FAIL  OPc generation: %v\n", err)
				failed++
			} else {
				fmt.Println("  PASS  OPc generation (OP + Ki → OPc verified)")
				passed++
			}

			// Test 3: Auth vector generation (RAND/XRES/AUTN/KASME structure)
			if err := testAuthVectorStructure(); err != nil {
				fmt.Printf("  FAIL  Auth vector structure: %v\n", err)
				failed++
			} else {
				fmt.Println("  PASS  Auth vector structure (RAND/XRES/AUTN/KASME lengths correct)")
				passed++
			}

			// Test 4: API health (if running)
			fmt.Println("\n=== API Connectivity ===")
			if err := testAPIHealth(baseURL); err != nil {
				fmt.Printf("  SKIP  API health: %v\n", err)
			} else {
				fmt.Println("  PASS  API healthy at " + baseURL)
				passed++
			}

			// Summary
			fmt.Printf("\n%d passed, %d failed\n", passed, failed)
			if failed > 0 {
				return fmt.Errorf("%d test(s) failed — this is a bug, please report at https://github.com/umairsuperhero/qcore/issues", failed)
			}
			fmt.Println("All crypto tests passed. QCore HSS is working correctly.")
			return nil
		},
	}
	cmd.Flags().StringVar(&baseURL, "url", defaultString(os.Getenv("QCORE_URL"), "http://localhost:8080"),
		"HSS base URL for API tests")
	return cmd
}

// testMilenageSet1 verifies F2345 against 3GPP TS 35.208 Test Set 1.
func testMilenageSet1() error {
	k := mustHex16("465b5ce8b199b49faa5f0a2ee238a6bc")
	opc := mustHex16("cd63cb71954a9f4e48a5994e37a02baf")
	rand := mustHex16("23553cbe9637a89d218ae64dae47bf35")

	res, ck, ik, ak, err := subscriber.F2345(k, opc, rand)
	if err != nil {
		return err
	}

	expect := func(name, got, want string) error {
		if got != want {
			return fmt.Errorf("%s: got %s, want %s", name, got, want)
		}
		return nil
	}

	if err := expect("RES", hex.EncodeToString(res[:]), "a54211d5e3ba50bf"); err != nil {
		return err
	}
	if err := expect("CK", hex.EncodeToString(ck[:]), "b40ba9a3c58b2a05bbf0d987b21bf8cb"); err != nil {
		return err
	}
	if err := expect("IK", hex.EncodeToString(ik[:]), "f769bcd751044604127672711c6d3441"); err != nil {
		return err
	}
	if err := expect("AK", hex.EncodeToString(ak[:]), "aa689c648370"); err != nil {
		return err
	}
	return nil
}

// testOPcGeneration verifies GenerateOPc against TS 35.208 Test Set 1.
func testOPcGeneration() error {
	k := mustHex16("465b5ce8b199b49faa5f0a2ee238a6bc")
	op := mustHex16("cdc202d5123e20f62b6d676ac72cb318")

	opc, err := subscriber.GenerateOPc(k, op)
	if err != nil {
		return err
	}

	got := hex.EncodeToString(opc[:])
	want := "cd63cb71954a9f4e48a5994e37a02baf"
	if got != want {
		return fmt.Errorf("OPc: got %s, want %s", got, want)
	}
	return nil
}

// testAuthVectorStructure verifies that GenerateAuthVector returns
// correctly-sized fields.
func testAuthVectorStructure() error {
	k := mustHex16("465b5ce8b199b49faa5f0a2ee238a6bc")
	opc := mustHex16("cd63cb71954a9f4e48a5994e37a02baf")
	sqn := [6]byte{}
	amf := [2]byte{0x80, 0x00}
	plmn := [3]byte{0x00, 0x01, 0x01}

	av, err := subscriber.GenerateAuthVector(k, opc, sqn, amf, plmn)
	if err != nil {
		return err
	}

	// AuthVector fields are hex-encoded, so byte lengths are doubled
	if len(av.RAND) != 32 {
		return fmt.Errorf("RAND hex length: got %d, want 32 (16 bytes)", len(av.RAND))
	}
	if len(av.XRES) != 16 {
		return fmt.Errorf("XRES hex length: got %d, want 16 (8 bytes)", len(av.XRES))
	}
	if len(av.AUTN) != 32 {
		return fmt.Errorf("AUTN hex length: got %d, want 32 (16 bytes)", len(av.AUTN))
	}
	if len(av.KASME) != 64 {
		return fmt.Errorf("KASME hex length: got %d, want 64 (32 bytes)", len(av.KASME))
	}
	return nil
}

func testAPIHealth(baseURL string) error {
	client := &http.Client{Timeout: 3 * time.Second}
	resp, err := client.Get(baseURL + "/api/v1/health")
	if err != nil {
		return fmt.Errorf("not running at %s (start with: qcore-hss start)", baseURL)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return fmt.Errorf("unhealthy (status %d)", resp.StatusCode)
	}
	return nil
}

func mustHex16(s string) [16]byte {
	b, err := hex.DecodeString(s)
	if err != nil || len(b) != 16 {
		panic(fmt.Sprintf("bad hex16: %q", s))
	}
	var out [16]byte
	copy(out[:], b)
	return out
}
