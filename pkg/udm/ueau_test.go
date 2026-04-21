package udm

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"testing"
	"time"

	"github.com/qcore-project/qcore/pkg/logger"
	"github.com/qcore-project/qcore/pkg/sbi"
	"github.com/qcore-project/qcore/pkg/subscriber"
)

// fakeAuthGen is an AuthGenerator that runs real Milenage / 5G-AKA
// crypto against an in-memory subscriber map — no gorm. SQN is
// incremented on each call, mirroring Service.Generate5GAuthVector.
type fakeAuthGen struct {
	subs map[string]*subscriber.Subscriber
}

func (f *fakeAuthGen) Generate5GAuthVector(_ context.Context, imsi, snName string) (*subscriber.AuthVector5G, error) {
	sub, ok := f.subs[imsi]
	if !ok {
		return nil, fmt.Errorf("subscriber %s not found", imsi)
	}
	ki, err := sub.KiBytes()
	if err != nil {
		return nil, err
	}
	opc, err := sub.OPcBytes()
	if err != nil {
		return nil, err
	}
	sqn, err := sub.SQNBytes()
	if err != nil {
		return nil, err
	}
	amf, err := sub.AMFBytes()
	if err != nil {
		return nil, err
	}
	av, err := subscriber.Generate5GAuthVector(ki, opc, sqn, amf, snName)
	if err != nil {
		return nil, err
	}
	if err := sub.IncrementSQN(); err != nil {
		return nil, err
	}
	return av, nil
}

// TestUDM_UEAU_GenerateAuthData drives Nudm_UEAU generate-auth-data over
// a real pkg/sbi h2c server. Happy path + missing servingNetworkName
// (400) + unknown SUPI (404) + resync attempt (501).
func TestUDM_UEAU_GenerateAuthData(t *testing.T) {
	log := logger.New("error", "text")

	sub := &subscriber.Subscriber{
		IMSI: "001010000000001",
		Ki:   "465b5ce8b199b49faa5f0a2ee238a6bc",
		OPc:  "cd63cb71954a9f4e48a5994e37a02baf",
		AMF:  "8000",
		SQN:  "000000000001",
	}
	gen := &fakeAuthGen{subs: map[string]*subscriber.Subscriber{"001010000000001": sub}}

	// SDM still needs its own source; UEAU is the one under test here.
	// Reuse fakeStore from udm_test.go (same package).
	store := &fakeStore{subs: map[string]*subscriber.Subscriber{"001010000000001": sub}}

	udm := NewService(NewStoreSource(store), log).WithAuthSource(NewStoreAuthSource(gen))

	port := pickFreePort(t)
	srv := sbi.NewServer(sbi.ServerConfig{
		BindAddress: "127.0.0.1",
		Port:        port,
		NFType:      "UDM",
	}, log, udm.Handler())
	go func() { _ = srv.Serve() }()
	time.Sleep(50 * time.Millisecond)
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		_ = srv.Shutdown(ctx)
	})

	client := sbi.NewClient("http://127.0.0.1:"+strconv.Itoa(port), "TEST-AUSF", false)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	const path = "/nudm-ueau/v1/imsi-001010000000001/security-information/generate-auth-data"

	t.Run("happy path returns Av5gHeAka", func(t *testing.T) {
		req := AuthenticationInfoRequest{ServingNetworkName: "5G:mnc001.mcc001.3gppnetwork.org"}
		var resp AuthenticationInfoResult
		if err := client.DoJSON(ctx, "POST", path, &req, &resp); err != nil {
			t.Fatalf("DoJSON: %v", err)
		}
		if resp.AuthType != AuthType5GAka {
			t.Errorf("authType: want 5G_AKA, got %q", resp.AuthType)
		}
		if resp.AuthenticationVector == nil {
			t.Fatalf("authenticationVector is nil")
		}
		av := resp.AuthenticationVector
		if av.AvType != "5G_HE_AKA" {
			t.Errorf("avType: want 5G_HE_AKA, got %q", av.AvType)
		}
		if got, want := decodedLen(t, av.RAND), 16; got != want {
			t.Errorf("RAND: want %d bytes, got %d", want, got)
		}
		if got, want := decodedLen(t, av.AUTN), 16; got != want {
			t.Errorf("AUTN: want %d bytes, got %d", want, got)
		}
		if got, want := decodedLen(t, av.XResStar), 16; got != want {
			t.Errorf("XResStar: want %d bytes, got %d", want, got)
		}
		if got, want := decodedLen(t, av.KAUSF), 32; got != want {
			t.Errorf("KAUSF: want %d bytes, got %d", want, got)
		}
		if resp.Supi != "imsi-001010000000001" {
			t.Errorf("supi: want imsi-001010000000001, got %q", resp.Supi)
		}
	})

	t.Run("missing servingNetworkName returns 400", func(t *testing.T) {
		body := bytes.NewBufferString(`{}`)
		httpResp, err := http.Post("http://127.0.0.1:"+strconv.Itoa(port)+path, "application/json", body)
		if err != nil {
			t.Fatalf("POST: %v", err)
		}
		defer httpResp.Body.Close()
		if httpResp.StatusCode != http.StatusBadRequest {
			t.Errorf("status: want 400, got %d", httpResp.StatusCode)
		}
		var pd sbi.ProblemDetails
		_ = json.NewDecoder(httpResp.Body).Decode(&pd)
		if pd.Cause != "MANDATORY_IE_MISSING" {
			t.Errorf("cause: want MANDATORY_IE_MISSING, got %q", pd.Cause)
		}
	})

	t.Run("unknown SUPI returns 404 USER_NOT_FOUND", func(t *testing.T) {
		req := AuthenticationInfoRequest{ServingNetworkName: "5G:mnc001.mcc001.3gppnetwork.org"}
		err := client.DoJSON(ctx, "POST", "/nudm-ueau/v1/imsi-999999999999999/security-information/generate-auth-data", &req, nil)
		pd, ok := err.(*sbi.ProblemDetails)
		if !ok {
			t.Fatalf("want *sbi.ProblemDetails, got %T: %v", err, err)
		}
		if pd.Status != http.StatusNotFound {
			t.Errorf("status: want 404, got %d", pd.Status)
		}
		if pd.Cause != "USER_NOT_FOUND" {
			t.Errorf("cause: want USER_NOT_FOUND, got %q", pd.Cause)
		}
	})

	t.Run("resync request returns 501", func(t *testing.T) {
		req := AuthenticationInfoRequest{
			ServingNetworkName: "5G:mnc001.mcc001.3gppnetwork.org",
			ResynchronizationInfo: &ResynchronizationInfo{
				RAND: "00000000000000000000000000000000",
				AUTS: "0000000000000000000000000000",
			},
		}
		err := client.DoJSON(ctx, "POST", path, &req, nil)
		pd, ok := err.(*sbi.ProblemDetails)
		if !ok {
			t.Fatalf("want *sbi.ProblemDetails, got %T: %v", err, err)
		}
		if pd.Status != http.StatusNotImplemented {
			t.Errorf("status: want 501, got %d", pd.Status)
		}
	})
}

// TestUDM_UEAU_NotConfigured — with no AuthSource attached, the UEAU
// route must exist only through WithAuthSource. Absent that call, the
// mux shouldn't match /nudm-ueau at all (404), so operators can't
// accidentally ship a UDM where UEAU is a silent no-op.
func TestUDM_UEAU_NotConfigured(t *testing.T) {
	log := logger.New("error", "text")
	store := &fakeStore{subs: map[string]*subscriber.Subscriber{}}

	udm := NewService(NewStoreSource(store), log) // no WithAuthSource

	port := pickFreePort(t)
	srv := sbi.NewServer(sbi.ServerConfig{BindAddress: "127.0.0.1", Port: port, NFType: "UDM"}, log, udm.Handler())
	go func() { _ = srv.Serve() }()
	time.Sleep(50 * time.Millisecond)
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		_ = srv.Shutdown(ctx)
	})

	client := sbi.NewClient("http://127.0.0.1:"+strconv.Itoa(port), "TEST-AUSF", false)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	req := AuthenticationInfoRequest{ServingNetworkName: "5G:mnc001.mcc001.3gppnetwork.org"}
	err := client.DoJSON(ctx, "POST", "/nudm-ueau/v1/imsi-001010000000001/security-information/generate-auth-data", &req, nil)
	pd, ok := err.(*sbi.ProblemDetails)
	if !ok {
		t.Fatalf("want *sbi.ProblemDetails, got %T: %v", err, err)
	}
	if pd.Status != http.StatusNotFound {
		t.Errorf("status: want 404 (route not registered), got %d", pd.Status)
	}
}

func decodedLen(t *testing.T, s string) int {
	t.Helper()
	b, err := hex.DecodeString(s)
	if err != nil {
		t.Errorf("hex decode %q: %v", s, err)
		return -1
	}
	return len(b)
}
