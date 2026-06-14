package dashboard

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/qcore-project/qcore/pkg/diag"
)

// RANConfig is the read-only "connect a real RAN" view (B5 / Pillar 3).
// The point: surface the exact values an engineer must put into their
// gNodeB/eNB so the simulator path and the real-RAN path are first-class
// peers, not "demo vs. real" with the latter buried.
//
// Reconciliation against an actual RAN config is Phase C.
type RANConfig struct {
	PLMN               string            `json:"plmn"`
	TAC                uint16            `json:"tac"`
	MMEAddress         string            `json:"mme_address"`
	MMES1APPort        int               `json:"mme_s1ap_port"`
	MMECode            uint8             `json:"mme_code"`
	MMEGroupID         uint16            `json:"mme_group_id"`
	AMFAddress         string            `json:"amf_address"`
	AMFNGAPPort        int               `json:"amf_ngap_port"`
	AMFPLMN            string            `json:"amf_plmn"`
	AMFTAC             uint16            `json:"amf_tac"`
	ServingNetworkName string            `json:"serving_network_name"`
	SPGWGTPUAddr       string            `json:"spgw_gtpu_address"`
	SPGWGTPUPort       int               `json:"spgw_gtpu_port"`
	UEPool             string            `json:"ue_pool"`
	SampleSub          SampleSubscriber  `json:"sample_subscriber"`
	ConfigSnippet      map[string]string `json:"config_snippet"`
}

// SampleSubscriber is the demo IMSI/Ki/OPc seeded by the HSS at startup.
// Engineers can punch these straight into a test SIM.
type SampleSubscriber struct {
	IMSI string `json:"imsi"`
	Ki   string `json:"ki"`
	OPc  string `json:"opc"`
	AMF  string `json:"amf"`
	APN  string `json:"apn"`
}

func (s *Server) handleRANConfig(w http.ResponseWriter, r *http.Request) {
	mme := s.cfg.MME
	amf := s.cfg.AMF
	spgw := s.cfg.SPGW

	rc := RANConfig{
		PLMN:               mme.PLMN,
		TAC:                mme.TAC,
		MMEAddress:         externalAddress(mme.BindAddress),
		MMES1APPort:        mme.S1APPort,
		MMECode:            mme.MMECode,
		MMEGroupID:         mme.MMEGroupID,
		AMFAddress:         externalAddress(amf.BindAddress),
		AMFNGAPPort:        amf.NGAPPort,
		AMFPLMN:            amf.PLMN,
		AMFTAC:             amf.TAC,
		ServingNetworkName: amf.ServingNetworkName,
		SPGWGTPUAddr:       spgw.SGWU1Addr,
		SPGWGTPUPort:       spgw.S1UPort,
		UEPool:             spgw.UEPool,
		SampleSub: SampleSubscriber{
			// Matches the demo subscriber seeded by cmd/hss/main.go (3GPP
			// TS 35.208 Test Set 1 — published test vectors, not secrets).
			IMSI: "001010000000001",
			Ki:   "465b5ce8b199b49faa5f0a2ee238a6bc",
			OPc:  "cd63cb71954a9f4e48a5994e37a02baf",
			AMF:  "8000",
			APN:  "internet",
		},
	}
	rc.ConfigSnippet = map[string]string{
		"srsran_enb":   srsRANENBSnippet(rc),
		"open5gs_enb":  open5gsENBSnippet(rc),
		"ueransim_gnb": ueransimGNBSnippet(rc),
	}

	writeJSON(w, http.StatusOK, rc)
}

func (s *Server) handleRANConfigReconcile(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 1<<20))
	if err != nil {
		http.Error(w, "reading request body: "+err.Error(), http.StatusBadRequest)
		return
	}

	in, err := diag.ParseReconcileInput(body)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	sub, err := s.fetchReconcileSubscriber(r.Context(), in.IMSI)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}

	result := diag.ReconcileConfig(in, diag.ReconcileCore{
		PLMN:               s.cfg.AMF.PLMN,
		TAC:                s.cfg.AMF.TAC,
		SNSSAIs:            []diag.SNSSAI{{SST: 1, SD: "000001"}},
		ServingNetworkName: s.cfg.AMF.ServingNetworkName,
		DefaultDNN:         "internet",
		Subscriber:         sub,
	})
	writeJSON(w, http.StatusOK, result)
}

func (s *Server) fetchReconcileSubscriber(ctx context.Context, imsi string) (*diag.SubscriberCredentials, error) {
	imsi = strings.TrimSpace(strings.TrimPrefix(imsi, "imsi-"))
	if imsi == "" {
		return nil, nil
	}

	u := *s.hssURL
	u.Path = strings.TrimRight(u.Path, "/") + "/api/v1/subscribers/" + imsi

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("creating subscriber lookup: %w", err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("subscriber lookup failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return nil, nil
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("subscriber lookup returned %s", resp.Status)
	}

	var out struct {
		Data struct {
			IMSI string `json:"imsi"`
			Ki   string `json:"ki"`
			OPc  string `json:"opc"`
			APN  string `json:"apn"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, fmt.Errorf("decoding subscriber lookup: %w", err)
	}
	if out.Data.IMSI == "" {
		return nil, fmt.Errorf("subscriber lookup returned no subscriber data")
	}
	return &diag.SubscriberCredentials{
		IMSI: out.Data.IMSI,
		Ki:   out.Data.Ki,
		OPc:  out.Data.OPc,
		APN:  out.Data.APN,
	}, nil
}

// externalAddress hides the 0.0.0.0 bind address by replacing it with a
// helpful placeholder — bind 0.0.0.0 means "listen on all interfaces" but
// the engineer needs to know what address their RAN should target.
func externalAddress(bind string) string {
	if bind == "0.0.0.0" || bind == "" {
		return "<qcore-host-ip>"
	}
	return bind
}

func splitPLMN(plmn string) (string, string) {
	if len(plmn) < 5 {
		return plmn, ""
	}
	return plmn[:3], plmn[3:]
}

func srsRANENBSnippet(rc RANConfig) string {
	mcc, mnc := splitPLMN(rc.PLMN)
	return fmt.Sprintf(
		`# srsRAN eNB: paste into enb.conf
[enb]
mcc = %s
mnc = %s
mme_addr = %s
gtp_bind_addr = %s
s1c_bind_addr = %s

[enb_files]
sib_config = sib.conf
rr_config  = rr.conf
rb_config  = rb.conf
`,
		mcc, mnc,
		rc.MMEAddress,
		"<ran-host-ip>",
		"<ran-host-ip>",
	)
}

func open5gsENBSnippet(rc RANConfig) string {
	return fmt.Sprintf(
		`# Generic eNB / gNB: copy the values below
PLMN           = %s
TAC            = %d
MME S1-MME     = %s:%d
MME Group ID   = %d
MME Code       = %d
SGW S1-U (GTP) = %s:%d
UE address pool = %s

Test SIM (3GPP TS 35.208 Test Set 1):
  IMSI = %s
  Ki   = %s
  OPc  = %s
`,
		rc.PLMN, rc.TAC,
		rc.MMEAddress, rc.MMES1APPort,
		rc.MMEGroupID, rc.MMECode,
		rc.SPGWGTPUAddr, rc.SPGWGTPUPort,
		rc.UEPool,
		rc.SampleSub.IMSI, rc.SampleSub.Ki, rc.SampleSub.OPc,
	)
}

func ueransimGNBSnippet(rc RANConfig) string {
	mcc, mnc := splitPLMN(rc.AMFPLMN)
	return fmt.Sprintf(
		`# UERANSIM gNB: mirror these values in gnb.yaml
mcc: '%s'
mnc: '%s'
nci: '0x00000000001'
idLength: 32
tac: %d
linkIp: <ran-host-ip>
ngapIp: <ran-host-ip>
gtpIp: <ran-host-ip>
amfConfigs:
  - address: %s
    port: %d
slices:
  - sst: 1
    sd: 000001
`,
		mcc, mnc, rc.AMFTAC, rc.AMFAddress, rc.AMFNGAPPort,
	)
}
