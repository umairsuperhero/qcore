package nrf

import (
	"context"
	"net"
	"net/http"
	"strconv"
	"testing"
	"time"

	"github.com/qcore-project/qcore/pkg/logger"
	"github.com/qcore-project/qcore/pkg/sbi"
	nrfclient "github.com/qcore-project/qcore/pkg/sbi/nrf"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func pickFreePort(t *testing.T) int {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	port := l.Addr().(*net.TCPAddr).Port
	_ = l.Close()
	return port
}

func startNRF(t *testing.T) (client *sbi.Client) {
	t.Helper()
	store := nrfclient.NewInMemory()
	log := logger.New("error", "text")
	nrfSvc := NewServer(store, log)
	port := pickFreePort(t)
	srv := sbi.NewServer(sbi.ServerConfig{
		BindAddress: "127.0.0.1",
		Port:        port,
		NFType:      "NRF",
	}, log, nrfSvc.Handler())
	go func() { _ = srv.Serve() }()
	time.Sleep(50 * time.Millisecond)
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		_ = srv.Shutdown(ctx)
	})
	return sbi.NewClient("http://127.0.0.1:"+strconv.Itoa(port), "AMF", false)
}

func ctx2s() (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), 2*time.Second)
}

// --- Helpers ----------------------------------------------------------------

func putProfile(t *testing.T, c *sbi.Client, p wireNFProfile) {
	t.Helper()
	ctx, cancel := ctx2s()
	defer cancel()
	var out wireNFProfile
	err := c.DoJSON(ctx, http.MethodPut, "/nnrf-nfm/v1/nf-instances/"+p.NFInstanceID, p, &out)
	require.NoError(t, err)
}

func deleteStatus(t *testing.T, c *sbi.Client, id string) int {
	t.Helper()
	ctx, cancel := ctx2s()
	defer cancel()
	err := c.DoJSON(ctx, http.MethodDelete, "/nnrf-nfm/v1/nf-instances/"+id, nil, nil)
	if err != nil {
		if pd, ok := err.(*sbi.ProblemDetails); ok {
			return pd.Status
		}
		return 0
	}
	return http.StatusNoContent
}

func heartbeatStatus(t *testing.T, c *sbi.Client, id string) int {
	t.Helper()
	ctx, cancel := ctx2s()
	defer cancel()
	patch := []map[string]string{{"op": "replace", "path": "/nfStatus", "value": "REGISTERED"}}
	err := c.DoJSON(ctx, http.MethodPatch, "/nnrf-nfm/v1/nf-instances/"+id, patch, nil)
	if err != nil {
		if pd, ok := err.(*sbi.ProblemDetails); ok {
			return pd.Status
		}
		return 0
	}
	return http.StatusNoContent
}

func discover(t *testing.T, c *sbi.Client, targetType, svcName string) discoveryResponse {
	t.Helper()
	path := "/nnrf-disc/v1/nf-instances?target-nf-type=" + targetType
	if svcName != "" {
		path += "&service-names=" + svcName
	}
	ctx, cancel := ctx2s()
	defer cancel()
	var dr discoveryResponse
	_ = c.DoJSON(ctx, http.MethodGet, path, nil, &dr)
	return dr
}

// --- Tests ------------------------------------------------------------------

func TestNRF_RegisterAndDiscover(t *testing.T) {
	c := startNRF(t)
	putProfile(t, c, wireNFProfile{
		NFInstanceID: "udm-1",
		NFType:       nrfclient.NFTypeUDM,
		NFStatus:     nrfclient.StatusRegistered,
		Services: []nrfclient.NFService{
			{ServiceName: "nudm-sdm", Versions: []string{"v2"}, Scheme: "http", Port: 8080},
		},
		PLMN: "00101",
	})
	dr := discover(t, c, string(nrfclient.NFTypeUDM), "")
	assert.Equal(t, 3600, dr.ValidityPeriod)
	require.Len(t, dr.NFInstances, 1)
	assert.Equal(t, "udm-1", dr.NFInstances[0].NFInstanceID)
	assert.Equal(t, nrfclient.NFTypeUDM, dr.NFInstances[0].NFType)
}

func TestNRF_ServiceFilteredDiscovery(t *testing.T) {
	c := startNRF(t)
	putProfile(t, c, wireNFProfile{
		NFInstanceID: "udm-1",
		NFType:       nrfclient.NFTypeUDM,
		Services:     []nrfclient.NFService{{ServiceName: "nudm-sdm", Port: 8080}},
	})
	putProfile(t, c, wireNFProfile{
		NFInstanceID: "ausf-1",
		NFType:       nrfclient.NFTypeAUSF,
		Services:     []nrfclient.NFService{{ServiceName: "nausf-auth", Port: 8081}},
	})

	dr := discover(t, c, string(nrfclient.NFTypeUDM), "nudm-sdm")
	require.Len(t, dr.NFInstances, 1)
	assert.Equal(t, "udm-1", dr.NFInstances[0].NFInstanceID)

	dr = discover(t, c, string(nrfclient.NFTypeAUSF), "")
	require.Len(t, dr.NFInstances, 1)
	assert.Equal(t, "ausf-1", dr.NFInstances[0].NFInstanceID)

	dr = discover(t, c, string(nrfclient.NFTypeUDM), "no-such-svc")
	assert.Empty(t, dr.NFInstances)
}

func TestNRF_Heartbeat(t *testing.T) {
	c := startNRF(t)
	putProfile(t, c, wireNFProfile{NFInstanceID: "amf-1", NFType: nrfclient.NFTypeAMF})

	assert.Equal(t, http.StatusNoContent, heartbeatStatus(t, c, "amf-1"))
	assert.Equal(t, http.StatusNotFound, heartbeatStatus(t, c, "ghost"))
}

func TestNRF_Deregister(t *testing.T) {
	c := startNRF(t)
	putProfile(t, c, wireNFProfile{NFInstanceID: "smf-1", NFType: nrfclient.NFTypeSMF})

	require.Len(t, discover(t, c, string(nrfclient.NFTypeSMF), "").NFInstances, 1)
	assert.Equal(t, http.StatusNoContent, deleteStatus(t, c, "smf-1"))
	assert.Empty(t, discover(t, c, string(nrfclient.NFTypeSMF), "").NFInstances)
	assert.Equal(t, http.StatusNotFound, deleteStatus(t, c, "smf-1"))
}

func TestNRF_GetNFInstance(t *testing.T) {
	c := startNRF(t)
	putProfile(t, c, wireNFProfile{NFInstanceID: "udr-1", NFType: nrfclient.NFTypeUDR})

	ctx, cancel := ctx2s()
	defer cancel()
	var p wireNFProfile
	err := c.DoJSON(ctx, http.MethodGet, "/nnrf-nfm/v1/nf-instances/udr-1", nil, &p)
	require.NoError(t, err)
	assert.Equal(t, "udr-1", p.NFInstanceID)

	ctx2, cancel2 := ctx2s()
	defer cancel2()
	err = c.DoJSON(ctx2, http.MethodGet, "/nnrf-nfm/v1/nf-instances/nobody", nil, nil)
	require.Error(t, err)
	pd, ok := err.(*sbi.ProblemDetails)
	require.True(t, ok, "expected *sbi.ProblemDetails, got %T", err)
	assert.Equal(t, http.StatusNotFound, pd.Status)
}

func TestNRF_HeartbeatTimerRoundTrip(t *testing.T) {
	c := startNRF(t)
	putProfile(t, c, wireNFProfile{NFInstanceID: "pcf-1", NFType: nrfclient.NFTypePCF, HeartbeatTimer: 30})

	ctx, cancel := ctx2s()
	defer cancel()
	var got wireNFProfile
	require.NoError(t, c.DoJSON(ctx, http.MethodGet, "/nnrf-nfm/v1/nf-instances/pcf-1", nil, &got))
	assert.Equal(t, 30, got.HeartbeatTimer)
}
