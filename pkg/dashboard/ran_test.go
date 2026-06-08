package dashboard

import (
	"strings"
	"testing"
)

func TestSplitPLMN(t *testing.T) {
	mcc, mnc := splitPLMN("00101")
	if mcc != "001" || mnc != "01" {
		t.Fatalf("splitPLMN mismatch: mcc=%q mnc=%q", mcc, mnc)
	}
}

func TestUERANSIMGNBSnippetUsesAMFFields(t *testing.T) {
	rc := RANConfig{
		AMFAddress:  "10.0.0.10",
		AMFNGAPPort: 38412,
		AMFPLMN:     "00101",
		AMFTAC:      7,
	}

	snippet := ueransimGNBSnippet(rc)
	for _, want := range []string{
		"mcc: '001'",
		"mnc: '01'",
		"tac: 7",
		"address: 10.0.0.10",
		"port: 38412",
	} {
		if !strings.Contains(snippet, want) {
			t.Fatalf("UERANSIM snippet missing %q:\n%s", want, snippet)
		}
	}
}
