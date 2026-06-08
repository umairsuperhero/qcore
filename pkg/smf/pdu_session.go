package smf

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"time"

	"github.com/qcore-project/qcore/pkg/diag"
	"github.com/qcore-project/qcore/pkg/events"
	"github.com/qcore-project/qcore/pkg/pfcp"
	"github.com/qcore-project/qcore/pkg/sbi"
)

// SMContextCreateData — TS 29.502 §6.1.6.2.2.
// Sent by AMF to create a new SM context.
type SMContextCreateData struct {
	Supi           string           `json:"supi"`
	Pei            string           `json:"pei,omitempty"`
	Gpsi           string           `json:"gpsi,omitempty"`
	PduSessionID   int              `json:"pduSessionId"`
	Dnn            string           `json:"dnn"`
	SNssai         SNssai           `json:"snssai,omitempty"`
	ServingNfId    string           `json:"servingNfId"`
	Guami          *GUAMI           `json:"guami,omitempty"`
	ServingNetwork *PLMN            `json:"servingNetwork"`
	RequestType    string           `json:"requestType,omitempty"`
	N1SmMsg        *RefToBinaryData `json:"n1SmMsg,omitempty"`
}

// SMContextCreatedData — TS 29.502 §6.1.6.2.3.
// Returned by SMF upon successful context creation.
type SMContextCreatedData struct {
	PduSessionID int              `json:"pduSessionId"`
	SNssai       SNssai           `json:"snssai,omitempty"`
	UpCnxState   string           `json:"upCnxState,omitempty"` // ACTIVATED or DEACTIVATED
	N1SmMsg      *RefToBinaryData `json:"n1SmMsg,omitempty"`
	N2SmInfo     *RefToBinaryData `json:"n2SmInfo,omitempty"`
	// UeIpv4Addr is the IPv4 address allocated to the UE for this session.
	// QCore (simplified) returns it here so the AMF can place it in the PDU
	// address IE of the PDU Session Establishment Accept it relays to the UE.
	UeIpv4Addr   string `json:"ueIpv4Addr,omitempty"`
	SmContextRef string `json:"smContextRef,omitempty"`
	UpfN3Ip      string `json:"upfN3Ip,omitempty"`
	UpfN3Teid    uint32 `json:"upfN3Teid,omitempty"`
}

type SMContextModifyData struct {
	Supi         string `json:"supi"`
	PduSessionID int    `json:"pduSessionId"`
	GnbN3Ip      string `json:"gnbN3Ip"`
	GnbN3Teid    uint32 `json:"gnbN3Teid"`
}

type SNssai struct {
	Sst int    `json:"sst"`
	Sd  string `json:"sd,omitempty"`
}

type GUAMI struct {
	PlmnId PLMN   `json:"plmnId"`
	AmfId  string `json:"amfId"`
}

type PLMN struct {
	Mcc string `json:"mcc"`
	Mnc string `json:"mnc"`
}

type RefToBinaryData struct {
	ContentId string `json:"contentId"`
}

// postSMContexts — TS 29.502 §5.2.2.2.2. Creates a PDU Session.
func (s *Service) postSMContexts(w http.ResponseWriter, r *http.Request) {
	// Parse Multipart or JSON body. For QCore v0.6, we assume standard JSON payload structure
	// while skipping full multipart NAS/NGAP binary attachments for this stub.
	var req SMContextCreateData
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		sbi.WriteProblem(w, &sbi.ProblemDetails{
			Status: http.StatusBadRequest,
			Title:  "Bad Request",
			Detail: "malformed SMContextCreateData: " + err.Error(),
			Cause:  "MANDATORY_IE_INCORRECT",
		})
		return
	}

	if req.Supi == "" {
		sbi.WriteProblem(w, &sbi.ProblemDetails{
			Status: http.StatusBadRequest,
			Title:  "Bad Request",
			Detail: "supi is required",
			Cause:  "MANDATORY_IE_MISSING",
		})
		return
	}

	// 1. Allocate UE IP Address
	ueIP, err := s.ipam.Allocate()
	if err != nil {
		s.log.WithError(err).Error("smf: failed to allocate UE IP")
		s.emitter.Emit(events.Event{
			NF:       "smf",
			Category: events.ErrorEvent,
			Severity: events.SeverityError,
			Protocol: "sbi",
			Message:  "PDU session rejected: IP pool exhausted",
			Payload: events.PDUSessionResultPayload{
				SUPI:         req.Supi,
				PDUSessionID: uint8(req.PduSessionID),
				DNN:          req.Dnn,
				Success:      false,
				Cause:        diag.CauseIPPoolExhausted,
				Detail:       err.Error(),
			},
		})
		sbi.WriteProblem(w, sbi.InternalError("IP pool exhausted"))
		return
	}

	s.log.WithField("ip", ueIP.String()).Info("smf: allocated UE IP")
	s.emitter.Emit(events.Event{
		NF:       "smf",
		Category: events.SignalingRx,
		Severity: events.SeverityInfo,
		Protocol: "sbi",
		Message:  "PDU session created: IP allocated",
		Payload: events.SMFSessionPayload{
			SUPI:         req.Supi,
			PDUSessionID: req.PduSessionID,
			DNN:          req.Dnn,
			UEIP:         ueIP.String(),
			Success:      true,
		},
	})

	upfSEID, upfTEID, upfIP, err := s.establishPFCPSession(r.Context(), req, ueIP)
	if err != nil {
		s.log.WithError(err).Error("smf: failed to establish PFCP session")
		sbi.WriteProblem(w, sbi.InternalError("PFCP session establishment failed"))
		return
	}

	ref := smContextKey(req.Supi, req.PduSessionID)
	s.mu.Lock()
	s.sessions[ref] = &SessionContext{
		SUPI:         req.Supi,
		PDUSessionID: req.PduSessionID,
		UEIP:         ueIP.String(),
		UPFSEID:      upfSEID,
		UPFTEID:      upfTEID,
		UPFN3IP:      upfIP.String(),
	}
	s.mu.Unlock()

	// Create the created response
	resp := SMContextCreatedData{
		PduSessionID: req.PduSessionID,
		UpCnxState:   "ACTIVATED",
		UeIpv4Addr:   ueIP.String(),
		SmContextRef: ref,
		UpfN3Ip:      upfIP.String(),
		UpfN3Teid:    upfTEID,
	}

	// 3. Return 201 Created
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Location", "/nsmf-pdusession/v1/sm-contexts/"+req.Supi)
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(resp)
}

func (s *Service) postSMContextModify(w http.ResponseWriter, r *http.Request) {
	var req SMContextModifyData
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		sbi.WriteProblem(w, &sbi.ProblemDetails{
			Status: http.StatusBadRequest,
			Title:  "Bad Request",
			Detail: "malformed SMContextModifyData: " + err.Error(),
			Cause:  "MANDATORY_IE_INCORRECT",
		})
		return
	}
	ref := smContextKey(req.Supi, req.PduSessionID)
	s.mu.Lock()
	sess := s.sessions[ref]
	s.mu.Unlock()
	if sess == nil {
		sbi.WriteProblem(w, &sbi.ProblemDetails{
			Status: http.StatusNotFound,
			Title:  "Not Found",
			Detail: "SM context not found",
			Cause:  "CONTEXT_NOT_FOUND",
		})
		return
	}
	if err := s.modifyPFCPSession(r.Context(), sess, req.GnbN3Ip, req.GnbN3Teid); err != nil {
		s.log.WithError(err).Warn("smf: PFCP session modification failed")
		sbi.WriteProblem(w, sbi.InternalError("PFCP session modification failed"))
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Service) establishPFCPSession(ctx context.Context, req SMContextCreateData, ueIP net.IP) (uint64, uint32, net.IP, error) {
	nodeIP := net.ParseIP(s.cfg.LocalPfcpIP).To4()
	if nodeIP == nil || nodeIP.IsUnspecified() {
		nodeIP = net.IPv4(127, 0, 0, 1)
	}
	nodeID := pfcp.NewNodeID(pfcp.NodeIDTypeIPv4, nodeIP, "")
	fseid := pfcp.NewFSEID(uint64(time.Now().UnixNano()), nodeIP, nil)
	msg := pfcp.NewSessionEstablishmentRequest(0, nodeID, fseid, pfcp.NewUEIPAddress(ueIP))
	resp, err := s.pfcpCli.SendRequest(ctx, msg)
	if err != nil {
		return 0, 0, nil, err
	}
	if cause, ok := findPFCPCause(resp); ok && cause != pfcp.CauseRequestAccepted {
		return 0, 0, nil, fmt.Errorf("UPF rejected PFCP establishment cause=%d", cause)
	}
	upfSEID := resp.Header.SEID
	var upfTEID uint32
	var upfIP net.IP
	for _, ie := range resp.IEs {
		switch ie.Type {
		case pfcp.IETypeFSEID:
			seid, ip, err := pfcp.ParseFSEID(ie)
			if err == nil {
				upfSEID = seid
				upfIP = ip
			}
		case pfcp.IETypeFTEID:
			teid, ip, err := pfcp.ParseFTEID(ie)
			if err == nil {
				upfTEID = teid
				if ip != nil {
					upfIP = ip
				}
			}
		}
	}
	if upfIP == nil || upfIP.IsLoopback() || upfIP.IsUnspecified() {
		upfIP = s.pfcpCli.RemoteIP()
	}
	if upfSEID == 0 || upfTEID == 0 || upfIP == nil {
		return 0, 0, nil, fmt.Errorf("UPF PFCP response missing tunnel state seid=%d teid=%d ip=%v", upfSEID, upfTEID, upfIP)
	}
	return upfSEID, upfTEID, upfIP, nil
}

func (s *Service) modifyPFCPSession(ctx context.Context, sess *SessionContext, gnbIP string, gnbTEID uint32) error {
	ip := net.ParseIP(gnbIP).To4()
	if ip == nil || gnbTEID == 0 {
		return fmt.Errorf("invalid gNB tunnel ip=%q teid=%d", gnbIP, gnbTEID)
	}
	msg := pfcp.NewSessionModificationRequest(0, sess.UPFSEID, pfcp.NewFTEID(gnbTEID, ip))
	resp, err := s.pfcpCli.SendRequest(ctx, msg)
	if err != nil {
		return err
	}
	if cause, ok := findPFCPCause(resp); ok && cause != pfcp.CauseRequestAccepted {
		return fmt.Errorf("UPF rejected PFCP modification cause=%d", cause)
	}
	return nil
}

func findPFCPCause(msg *pfcp.Message) (uint8, bool) {
	for _, ie := range msg.IEs {
		if ie.Type != pfcp.IETypeCause {
			continue
		}
		cause, err := pfcp.ParseCause(ie)
		return cause, err == nil
	}
	return 0, false
}

func smContextKey(supi string, pduSessionID int) string {
	return fmt.Sprintf("%s:%d", supi, pduSessionID)
}
