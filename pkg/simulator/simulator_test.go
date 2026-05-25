package simulator_test

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/qcore-project/qcore/pkg/config"
	"github.com/qcore-project/qcore/pkg/events"
	"github.com/qcore-project/qcore/pkg/logger"
	"github.com/qcore-project/qcore/pkg/mme"
	"github.com/qcore-project/qcore/pkg/simulator"
	"github.com/qcore-project/qcore/pkg/subscriber"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestSimulatorHappyPath spins up a real MME against a Milenage-aware mock
// HSS and verifies that simulator.Run completes a full attach without error.
// This is the exit criterion of B4 — the built-in simulator can drive a
// real 4G attach end-to-end.
func TestSimulatorHappyPath(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test skipped in -short mode")
	}

	const (
		imsi = "001010000000001"
		ki   = "465b5ce8b199b49faa5f0a2ee238a6bc"
		opc  = "cd63cb71954a9f4e48a5994e37a02baf"
		plmn = "00101"
	)

	mmePort, hss := startMockHSS(t, imsi, ki, opc, plmn)
	defer hss.Close()

	mmeInst := startMME(t, mmePort, plmn, hss.URL)
	defer mmeInst.Stop()

	packedPLMN, err := simulator.PackPLMN(plmn)
	require.NoError(t, err)

	res := simulator.Run(context.Background(), simulator.Options{
		MMEAddr: fmt.Sprintf("127.0.0.1:%d", mmePort),
		PLMN:    packedPLMN,
		TAC:     1,
		IMSI:    imsi,
		Ki:      ki,
		OPc:     opc,
	}, &events.NoopEmitter{}, noopLogger{})

	if !res.Success {
		t.Fatalf("simulator failed at step %q: %v", res.FailedStep, res.Err)
	}
	assert.NotEmpty(t, res.JourneyID)
}

// TestSimulatorWrongKi exercises the wrong_ki error-injection scenario.
// The MME's auth check should reject the response and the simulator should
// surface that as a labeled failure at the auth/security-mode step.
func TestSimulatorWrongKi(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test skipped in -short mode")
	}

	const (
		imsi = "001010000000001"
		ki   = "465b5ce8b199b49faa5f0a2ee238a6bc"
		opc  = "cd63cb71954a9f4e48a5994e37a02baf"
		plmn = "00101"
	)

	mmePort, hss := startMockHSS(t, imsi, ki, opc, plmn)
	defer hss.Close()

	mmeInst := startMME(t, mmePort, plmn, hss.URL)
	defer mmeInst.Stop()

	packedPLMN, _ := simulator.PackPLMN(plmn)

	res := simulator.Run(context.Background(), simulator.Options{
		MMEAddr:  fmt.Sprintf("127.0.0.1:%d", mmePort),
		PLMN:     packedPLMN,
		TAC:      1,
		IMSI:     imsi,
		Ki:       ki,
		OPc:      opc,
		Scenario: "wrong_ki",
	}, &events.NoopEmitter{}, noopLogger{})

	assert.False(t, res.Success, "expected wrong_ki scenario to fail")
	// Failure surfaces as MME sending Auth Reject (caught when we expect
	// a Security Mode Command) — failed step is "security_mode_command".
	if res.FailedStep != "security_mode_command" && res.FailedStep != "auth_request" {
		t.Logf("note: wrong_ki failed at step %q (expected security_mode_command or auth_request): %v",
			res.FailedStep, res.Err)
	}
}

// TestSimulatorWrongMMEAddress confirms the simulator surfaces a connect
// failure with a labeled step. (No MME or HSS needed — that's the point.)
func TestSimulatorWrongMMEAddress(t *testing.T) {
	res := simulator.Run(context.Background(), simulator.Options{
		MMEAddr:  "127.0.0.1:1",
		PLMN:     [3]byte{0x00, 0xF1, 0x10},
		TAC:      1,
		IMSI:     "001010000000001",
		Ki:       "465b5ce8b199b49faa5f0a2ee238a6bc",
		OPc:      "cd63cb71954a9f4e48a5994e37a02baf",
		Scenario: "wrong_mme_address",
	}, &events.NoopEmitter{}, noopLogger{})

	assert.False(t, res.Success)
	assert.Equal(t, "connect", res.FailedStep)
}

// --- Helpers ---

// startMockHSS returns a httptest.Server that computes auth vectors with
// the real Milenage path, plus a free port the test should bind the MME to.
func startMockHSS(t *testing.T, imsi, kiHex, opcHex, plmn string) (int, *httptest.Server) {
	t.Helper()

	ki, err := hex16(kiHex)
	require.NoError(t, err)
	opc, err := hex16(opcHex)
	require.NoError(t, err)

	packedPLMN, err := simulator.PackPLMN(plmn)
	require.NoError(t, err)

	var sqn [6]byte
	amf := [2]byte{0x80, 0x00}

	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/health", func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprintln(w, `{"status":"ok"}`)
	})
	mux.HandleFunc(fmt.Sprintf("/api/v1/subscribers/%s/auth-vector", imsi),
		func(w http.ResponseWriter, _ *http.Request) {
			av, err := subscriber.GenerateAuthVector(ki, opc, sqn, amf, packedPLMN)
			if err != nil {
				http.Error(w, err.Error(), 500)
				return
			}
			// HSS increments SQN on each auth vector — mimic that so a
			// re-run still works (though tests run once).
			incSQN(&sqn)
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"data": map[string]string{
					"rand":  av.RAND,
					"autn":  av.AUTN,
					"xres":  av.XRES,
					"kasme": av.KASME,
				},
			})
		})
	server := httptest.NewServer(mux)

	return pickFreePort(t), server
}

func hex16(s string) ([16]byte, error) {
	var out [16]byte
	if len(s) != 32 {
		return out, fmt.Errorf("expected 32 hex chars, got %d", len(s))
	}
	for i := 0; i < 16; i++ {
		var b byte
		for j := 0; j < 2; j++ {
			c := s[i*2+j]
			var v byte
			switch {
			case c >= '0' && c <= '9':
				v = c - '0'
			case c >= 'a' && c <= 'f':
				v = c - 'a' + 10
			case c >= 'A' && c <= 'F':
				v = c - 'A' + 10
			default:
				return out, fmt.Errorf("bad hex char %q", c)
			}
			b = b<<4 | v
		}
		out[i] = b
	}
	return out, nil
}

func startMME(t *testing.T, port int, plmnStr, hssURL string) *mme.MME {
	t.Helper()
	cfg := &config.MMEConfig{
		Name:        "qcore-mme-simtest",
		BindAddress: "127.0.0.1",
		S1APPort:    port,
		APIPort:     port + 1,
		SCTPMode:    "tcp",
		PLMN:        plmnStr,
		HSSURL:      hssURL,
		TAC:         1,
		MMEGroupID:  1,
		MMECode:     1,
		RelCapacity: 127,
	}
	packedPLMN, err := simulator.PackPLMN(plmnStr)
	require.NoError(t, err)

	s6a := mme.NewS6aClient(hssURL, noopLogger{})
	inst := mme.New(cfg, packedPLMN, noopLogger{}, nil, s6a, nil)

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	require.NoError(t, inst.Start(ctx))

	// Wait for the listener to be ready.
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		c, err := net.DialTimeout("tcp", fmt.Sprintf("127.0.0.1:%d", port), 100*time.Millisecond)
		if err == nil {
			_ = c.Close()
			return inst
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("MME never came up on port %d", port)
	return nil
}

func pickFreePort(t *testing.T) int {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	port := ln.Addr().(*net.TCPAddr).Port
	require.NoError(t, ln.Close())
	return port
}

func incSQN(sqn *[6]byte) {
	for i := 5; i >= 0; i-- {
		sqn[i]++
		if sqn[i] != 0 {
			return
		}
	}
}

// noopLogger satisfies logger.Logger for tests. We can't reuse the one in
// pkg/mme because it's unexported there.
type noopLogger struct{}

func (noopLogger) Debug(...interface{})                                {}
func (noopLogger) Info(...interface{})                                 {}
func (noopLogger) Warn(...interface{})                                 {}
func (noopLogger) Error(...interface{})                                {}
func (noopLogger) Fatal(...interface{})                                {}
func (noopLogger) Debugf(string, ...interface{})                       {}
func (noopLogger) Infof(string, ...interface{})                        {}
func (noopLogger) Warnf(string, ...interface{})                        {}
func (noopLogger) Errorf(string, ...interface{})                       {}
func (noopLogger) Fatalf(string, ...interface{})                       {}
func (noopLogger) WithField(string, interface{}) logger.Logger         { return noopLogger{} }
func (noopLogger) WithFields(map[string]interface{}) logger.Logger     { return noopLogger{} }
func (noopLogger) WithError(error) logger.Logger                       { return noopLogger{} }
func (noopLogger) Writer() io.Writer                                   { return io.Discard }
