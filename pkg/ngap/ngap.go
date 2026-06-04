// Package ngap implements NGAP (Next Generation Application Protocol) message
// encoding/decoding for the N2 interface between gNBs and AMF, per 3GPP TS 38.413.
//
// NGAP uses ASN.1 ALIGNED PER (Packed Encoding Rules) per ITU-T X.691.
// The wire structure mirrors S1AP: a top-level PDU CHOICE carries an IE container
// whose ProtocolIEs are keyed by ID. Only the procedure codes, IE IDs, and
// type shapes differ from S1AP.
package ngap

import (
	"fmt"

	"github.com/qcore-project/qcore/pkg/ident"
)

// PDUType identifies the outer NGAP-PDU CHOICE alternative (TS 38.413 §9.1).
type PDUType int

const (
	PDUInitiatingMessage   PDUType = 0
	PDUSuccessfulOutcome   PDUType = 1
	PDUUnsuccessfulOutcome PDUType = 2
)

func (t PDUType) String() string {
	switch t {
	case PDUInitiatingMessage:
		return "InitiatingMessage"
	case PDUSuccessfulOutcome:
		return "SuccessfulOutcome"
	case PDUUnsuccessfulOutcome:
		return "UnsuccessfulOutcome"
	default:
		return fmt.Sprintf("PDUType(%d)", t)
	}
}

// ProcedureCode identifies the NGAP procedure (TS 38.413 Table 9.1.3.1-1).
type ProcedureCode uint8

const (
	ProcAMFConfigurationUpdate ProcedureCode = 0
	ProcAMFStatusIndication    ProcedureCode = 1
	ProcCellTrafficTrace       ProcedureCode = 2
	ProcDeactivateTrace        ProcedureCode = 3
	ProcDownlinkNASTransport   ProcedureCode = 4
	ProcErrorIndication        ProcedureCode = 9
	ProcInitialContextSetup    ProcedureCode = 14
	ProcInitialUEMessage       ProcedureCode = 15
	ProcNGReset                ProcedureCode = 20
	ProcNGSetup                ProcedureCode = 21
	ProcPaging                 ProcedureCode = 24
	ProcUEContextRelease       ProcedureCode = 41
	ProcUEContextModification  ProcedureCode = 42
	ProcUplinkNASTransport     ProcedureCode = 46
)

func (p ProcedureCode) String() string {
	switch p {
	case ProcNGSetup:
		return "NGSetup"
	case ProcInitialUEMessage:
		return "InitialUEMessage"
	case ProcDownlinkNASTransport:
		return "DownlinkNASTransport"
	case ProcUplinkNASTransport:
		return "UplinkNASTransport"
	case ProcInitialContextSetup:
		return "InitialContextSetup"
	case ProcUEContextRelease:
		return "UEContextRelease"
	case ProcErrorIndication:
		return "ErrorIndication"
	default:
		return fmt.Sprintf("Procedure(%d)", p)
	}
}

// Criticality (TS 38.413 §9.1).
type Criticality uint8

const (
	CriticalityReject Criticality = 0
	CriticalityIgnore Criticality = 1
	CriticalityNotify Criticality = 2
)

// ProtocolIEID is an IE identifier (TS 38.413 Table 9.3.3.1-1).
type ProtocolIEID uint16

// IE IDs used in this package (verified against TS 38.413 Table 9.3.3.1-1).
const (
	IEIDAllowedNSSAI           ProtocolIEID = 0
	IEIDAMFName                ProtocolIEID = 1
	IEIDAMFUENGAPID            ProtocolIEID = 10
	IEIDCause                  ProtocolIEID = 15
	IEIDDefaultPagingDRX       ProtocolIEID = 21
	IEIDGlobalRANNodeID        ProtocolIEID = 27
	IEIDGUAMI                  ProtocolIEID = 28
	IEIDNASPDU                 ProtocolIEID = 38
	IEIDRelativeAMFCapacity    ProtocolIEID = 86
	IEIDPLMNSupportList        ProtocolIEID = 80
	IEIDRANNodeName            ProtocolIEID = 82
	IEIDRANUENGAPID            ProtocolIEID = 85
	IEIDRRCEstablishmentCause  ProtocolIEID = 90
	IEIDSecurityKey            ProtocolIEID = 94
	IEIDServedGUAMIList        ProtocolIEID = 96
	IEIDSupportedTAList        ProtocolIEID = 102
	IEIDTimeToWait             ProtocolIEID = 107
	IEIDUEAggMaxBitRate        ProtocolIEID = 110
	IEIDUESecurityCapabilities ProtocolIEID = 119
	IEIDUserLocationInfo       ProtocolIEID = 121
)

// PDU is the top-level NGAP message container.
type PDU struct {
	Type          PDUType
	ProcedureCode ProcedureCode
	Criticality   Criticality
	Value         []byte // encoded SEQUENCE of ProtocolIEs
}

// ProtocolIE is a single Information Element within an NGAP message.
type ProtocolIE struct {
	ID          ProtocolIEID
	Criticality Criticality
	Value       []byte
}

// PLMN is a 3-byte PLMN identity (MCC+MNC BCD per TS 24.501).
type PLMN [3]byte

// PLMNFromMCCMNC converts string MCC and MNC to PLMN bytes.
// MCC is always 3 digits; MNC is 2 or 3 digits.
// PLMNFromMCCMNC encodes an MCC and MNC string pair into the 3-byte PLMN
// identity defined by TS 24.008 §10.5.1.13. Delegates to pkg/ident so the
// encoding is guaranteed identical to subscriber.ParsePLMN and SUCI handling.
func PLMNFromMCCMNC(mcc, mnc string) PLMN {
	return PLMN(ident.EncodePLMN(mcc, mnc))
}

// TAI is a 5G Tracking Area Identity. TAC is 3 bytes (vs 2 bytes in S1AP).
type TAI struct {
	PLMN PLMN
	TAC  [3]byte
}

// NRCGI is an NR Cell Global Identifier.
type NRCGI struct {
	PLMN         PLMN
	NRCellID     uint64 // 36-bit value
}

// UserLocationInformationNR carries NR location (NR-CGI + TAI).
type UserLocationInformationNR struct {
	NRCGI NRCGI
	TAI   TAI
}

// GUAMI is a Globally Unique AMF Identifier.
type GUAMI struct {
	PLMN        PLMN
	AMFRegionID uint8  // 8-bit
	AMFSetID    uint16 // 10-bit (0..1023)
	AMFPointer  uint8  // 6-bit (0..63)
}

// GlobalGNBID identifies a gNB globally.
type GlobalGNBID struct {
	PLMN      PLMN
	GNBID     uint32 // gNB-ID value
	GNBIDBits uint8  // number of significant bits (22–32)
}

// SupportedTA is a tracking area supported by a gNB, with broadcast PLMN+slice info.
type SupportedTA struct {
	TAC           [3]byte
	BroadcastPLMN []BroadcastPLMNItem
}

// BroadcastPLMNItem is a PLMN broadcasted in a TA with its supported slices.
type BroadcastPLMNItem struct {
	PLMN   PLMN
	SNSSAI []SNSSAI
}

// SNSSAI is a Single Network Slice Selection Assistance Information (TS 23.003).
type SNSSAI struct {
	SST uint8  // Slice/Service Type (1 byte)
	SD  []byte // Slice Differentiator (3 bytes, optional; nil if absent)
}

// PLMNSupportItem is one entry in the PLMNSupportList (NGSetup Response).
type PLMNSupportItem struct {
	PLMN   PLMN
	SNSSAIs []SNSSAI
}

// ServedGUAMIItem is one entry in ServedGUAMIList (NGSetup Response).
type ServedGUAMIItem struct {
	GUAMI         GUAMI
	BackupAMFName string // optional
}

// CauseGroup identifies the top-level cause category.
type CauseGroup uint8

const (
	CauseRadioNetwork CauseGroup = 0
	CauseTransport    CauseGroup = 1
	CauseNAS          CauseGroup = 2
	CauseProtocol     CauseGroup = 3
	CauseMisc         CauseGroup = 4
)

// RRCEstablishmentCause (TS 38.413 §9.3.1.6).
type RRCEstablishmentCause uint8

const (
	RRCEmergency          RRCEstablishmentCause = 0
	RRCHighPriorityAccess RRCEstablishmentCause = 1
	RRCMtAccess           RRCEstablishmentCause = 2
	RRCMoSignalling       RRCEstablishmentCause = 3
	RRCMoData             RRCEstablishmentCause = 4
	RRCMoVideoTelephony   RRCEstablishmentCause = 5
	RRCMoVoiceCall        RRCEstablishmentCause = 6
	RRCMoSMS              RRCEstablishmentCause = 7
	RRCMoSSTelephony      RRCEstablishmentCause = 8
	RRCNotAvailable       RRCEstablishmentCause = 9
)
