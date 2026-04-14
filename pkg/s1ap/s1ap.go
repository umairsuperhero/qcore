// Package s1ap implements S1AP message encoding/decoding for the S1 interface
// between eNodeBs and MME, per 3GPP TS 36.413.
//
// S1AP uses ASN.1 ALIGNED PER (Packed Encoding Rules) per ITU-T X.691.
// Rather than a full ASN.1 compiler, we implement the subset of PER used by
// the S1AP messages needed for initial attach.
package s1ap

import "fmt"

// PDU types (TS 36.413 Section 9.1)
type PDUType int

const (
	PDUInitiatingMessage    PDUType = 0
	PDUSuccessfulOutcome    PDUType = 1
	PDUUnsuccessfulOutcome  PDUType = 2
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
		return "Unknown"
	}
}

// ProcedureCode identifies the S1AP procedure (TS 36.413 Section 9.1.4)
type ProcedureCode uint8

const (
	ProcHandoverPreparation       ProcedureCode = 0
	ProcHandoverResourceAlloc     ProcedureCode = 1
	ProcPathSwitchRequest         ProcedureCode = 3
	ProcE_RABSetup                ProcedureCode = 5
	ProcE_RABModify               ProcedureCode = 6
	ProcE_RABRelease              ProcedureCode = 7
	ProcInitialContextSetup       ProcedureCode = 9
	ProcPaging                    ProcedureCode = 10
	ProcDownlinkNASTransport      ProcedureCode = 11
	ProcInitialUEMessage          ProcedureCode = 12
	ProcUplinkNASTransport        ProcedureCode = 13
	ProcReset                     ProcedureCode = 14
	ProcErrorIndication           ProcedureCode = 15
	ProcNASNonDeliveryIndication  ProcedureCode = 16
	ProcS1Setup                   ProcedureCode = 17
	ProcUEContextRelease          ProcedureCode = 23
	ProcUEContextReleaseRequest   ProcedureCode = 18
	ProcUEContextModification     ProcedureCode = 21
)

func (p ProcedureCode) String() string {
	switch p {
	case ProcS1Setup:
		return "S1Setup"
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
	case ProcPaging:
		return "Paging"
	case ProcReset:
		return "Reset"
	default:
		return fmt.Sprintf("Procedure(%d)", p)
	}
}

// Criticality (TS 36.413)
type Criticality uint8

const (
	CriticalityReject Criticality = 0
	CriticalityIgnore Criticality = 1
	CriticalityNotify Criticality = 2
)

// ProtocolIE IDs (TS 36.413 Section 9.3)
type ProtocolIEID uint16

const (
	IEID_MME_UE_S1AP_ID           ProtocolIEID = 0
	IEID_ENB_UE_S1AP_ID           ProtocolIEID = 8
	IEID_NAS_PDU                  ProtocolIEID = 26
	IEID_TAI                      ProtocolIEID = 67
	IEID_EUTRAN_CGI               ProtocolIEID = 100
	IEID_RRC_Establishment_Cause  ProtocolIEID = 134
	IEID_Global_ENB_ID            ProtocolIEID = 59
	IEID_ENBname                  ProtocolIEID = 60
	IEID_SupportedTAs             ProtocolIEID = 64
	IEID_DefaultPagingDRX         ProtocolIEID = 137
	IEID_MMEname                  ProtocolIEID = 61
	IEID_ServedGUMMEIs            ProtocolIEID = 105
	IEID_RelativeMMECapacity      ProtocolIEID = 87
	IEID_Cause                    ProtocolIEID = 2
	IEID_UESecurityCapabilities   ProtocolIEID = 107
	IEID_SecurityKey              ProtocolIEID = 73
	IEID_E_RABToBeSetupListCtxtSUReq ProtocolIEID = 24
	IEID_UEAggMaxBitRate          ProtocolIEID = 66
)

// Cause values (TS 36.413 Section 9.2.1.3)
type CauseGroup uint8

const (
	CauseRadioNetwork CauseGroup = 0
	CauseTransport    CauseGroup = 1
	CauseNAS          CauseGroup = 2
	CauseProtocol     CauseGroup = 3
	CauseMisc         CauseGroup = 4
)

// RRC Establishment Cause (TS 36.413)
type RRCEstablishmentCause uint8

const (
	RRCEmergency            RRCEstablishmentCause = 0
	RRCHighPriorityAccess   RRCEstablishmentCause = 1
	RRCMtAccess             RRCEstablishmentCause = 2
	RRCMoSignalling         RRCEstablishmentCause = 3
	RRCMoData               RRCEstablishmentCause = 4
	RRCDelayTolerantAccess  RRCEstablishmentCause = 5
)

// PDU is the top-level S1AP message container.
type PDU struct {
	Type          PDUType
	ProcedureCode ProcedureCode
	Criticality   Criticality
	Value         []byte // encoded SEQUENCE of ProtocolIEs
}

// ProtocolIE is a single Information Element within an S1AP message.
type ProtocolIE struct {
	ID          ProtocolIEID
	Criticality Criticality
	Value       []byte // encoded IE value
}

// TAI represents a Tracking Area Identity.
type TAI struct {
	PLMN [3]byte
	TAC  uint16
}

// ECGI represents an E-UTRAN Cell Global Identifier.
type ECGI struct {
	PLMN   [3]byte
	CellID uint32 // 28-bit
}

// GlobalENBID identifies an eNodeB globally.
type GlobalENBID struct {
	PLMN  [3]byte
	ENBID uint32
	Type  ENBIDType
}

type ENBIDType int

const (
	MacroENBID ENBIDType = iota // 20-bit
	HomeENBID                   // 28-bit
)

// SupportedTA is a Tracking Area supported by an eNB, with broadcast PLMNs.
type SupportedTA struct {
	TAC   uint16
	PLMNs [][3]byte
}

// ServedGUMMEI represents a GUMMEI served by the MME.
type ServedGUMMEI struct {
	ServedPLMNs    [][3]byte
	ServedGroupIDs []uint16
	ServedMMECs    []uint8
}
