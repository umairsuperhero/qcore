package dashboard

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"

	"github.com/qcore-project/qcore/pkg/config"
	"github.com/qcore-project/qcore/pkg/diag"
)

const (
	testIMSI = "001010000000001"
	testKi   = "465b5ce8b199b49faa5f0a2ee238a6bc"
	testOPc  = "cd63cb71954a9f4e48a5994e37a02baf"
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

func TestHandleRANConfigReconcileMatched(t *testing.T) {
	s := testReconcileServer(t, http.StatusOK)
	req := httptest.NewRequest(http.MethodPost, "/api/ran-config/reconcile", bytes.NewBufferString(reconcileBody("001", "01", 1, testKi, testOPc)))
	rr := httptest.NewRecorder()

	s.handleRANConfigReconcile(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, body=%s", rr.Code, rr.Body.String())
	}
	var got diag.ReconcileResult
	if err := json.NewDecoder(rr.Body).Decode(&got); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if !got.OK {
		t.Fatalf("expected OK, got %+v", got.Mismatches)
	}
}

func TestHandleRANConfigReconcileMismatch(t *testing.T) {
	s := testReconcileServer(t, http.StatusOK)
	req := httptest.NewRequest(http.MethodPost, "/api/ran-config/reconcile", bytes.NewBufferString(reconcileBody("001", "02", 1, "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", testOPc)))
	rr := httptest.NewRecorder()

	s.handleRANConfigReconcile(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, body=%s", rr.Code, rr.Body.String())
	}
	var got diag.ReconcileResult
	if err := json.NewDecoder(rr.Body).Decode(&got); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if got.OK {
		t.Fatal("expected mismatch, got OK")
	}
	if !hasMismatch(got.Mismatches, "gnb.plmn", "plmn_mismatch") {
		t.Fatalf("expected PLMN mismatch, got %+v", got.Mismatches)
	}
	if !hasMismatch(got.Mismatches, "ue.key", diag.CauseKiMismatch) {
		t.Fatalf("expected Ki mismatch, got %+v", got.Mismatches)
	}
}

func TestHandleRANConfigReconcileUnknownSubscriber(t *testing.T) {
	s := testReconcileServer(t, http.StatusNotFound)
	req := httptest.NewRequest(http.MethodPost, "/api/ran-config/reconcile", bytes.NewBufferString(reconcileBody("001", "01", 1, testKi, testOPc)))
	rr := httptest.NewRecorder()

	s.handleRANConfigReconcile(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, body=%s", rr.Code, rr.Body.String())
	}
	var got diag.ReconcileResult
	if err := json.NewDecoder(rr.Body).Decode(&got); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if !hasMismatch(got.Mismatches, "ue.imsi", diag.CauseUnknownSubscriber) {
		t.Fatalf("expected unknown subscriber mismatch, got %+v", got.Mismatches)
	}
}

func testReconcileServer(t *testing.T, hssStatus int) *Server {
	t.Helper()
	hss := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/subscribers/"+testIMSI {
			http.NotFound(w, r)
			return
		}
		if hssStatus == http.StatusNotFound {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(hssStatus)
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"data": map[string]string{
				"imsi": testIMSI,
				"ki":   testKi,
				"opc":  testOPc,
				"apn":  "internet",
			},
		})
	}))
	t.Cleanup(hss.Close)
	hssURL, err := url.Parse(hss.URL)
	if err != nil {
		t.Fatal(err)
	}
	return &Server{
		cfg: &config.Config{
			AMF: config.AMFConfig{
				PLMN:               "00101",
				TAC:                1,
				ServingNetworkName: "5G:mnc001.mcc001.3gppnetwork.org",
			},
		},
		hssURL: hssURL,
	}
}

func reconcileBody(mcc, mnc string, tac uint16, ki, opc string) string {
	return `{
  "gnb_yaml": "mcc: '` + mcc + `'\nmnc: '` + mnc + `'\ntac: ` + strconv.Itoa(int(tac)) + `\nslices:\n  - sst: 1\n    sd: '000001'\n",
  "ue_yaml": "supi: 'imsi-` + testIMSI + `'\nmcc: '` + mcc + `'\nmnc: '` + mnc + `'\nkey: '` + ki + `'\nop: '` + opc + `'\nopType: 'OPC'\nsessions:\n  - apn: 'internet'\n    slice:\n      sst: 1\n      sd: '000001'\nprotectionScheme: 0\n"
}`
}

func hasMismatch(m []diag.ConfigMismatch, field, cause string) bool {
	for _, item := range m {
		if item.Field == field && item.Cause == cause {
			return true
		}
	}
	return false
}
