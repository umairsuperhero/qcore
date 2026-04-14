package s1ap

import (
	"encoding/binary"
	"fmt"
	"net"
)

// S1SetupRequest represents the S1 SETUP REQUEST message from eNB to MME.
// TS 36.413 Section 9.1.7.1
type S1SetupRequest struct {
	GlobalENBID  GlobalENBID
	ENBName      string       // optional
	SupportedTAs []SupportedTA
	PagingDRX    uint8        // DefaultPagingDRX: ENUMERATED { v32, v64, v128, v256 }
}

// S1SetupResponse represents the S1 SETUP RESPONSE from MME to eNB.
// TS 36.413 Section 9.1.7.2
type S1SetupResponse struct {
	MMEName         string // optional
	ServedGUMMEIs   []ServedGUMMEI
	RelativeCapacity uint8 // 0-255
}

// S1SetupFailure represents the S1 SETUP FAILURE from MME to eNB.
// TS 36.413 Section 9.1.7.3
type S1SetupFailure struct {
	CauseGroup CauseGroup
	CauseValue uint8
}

// InitialUEMessage represents the INITIAL UE MESSAGE from eNB to MME.
// TS 36.413 Section 9.1.7.1
type InitialUEMessage struct {
	ENBUES1APID     uint32
	NASPDU          []byte
	TAI             TAI
	ECGI            ECGI
	RRCCause        RRCEstablishmentCause
}

// DownlinkNASTransport carries a NAS PDU from MME to eNB.
// TS 36.413 Section 9.1.5.1
type DownlinkNASTransport struct {
	MMEUES1APID uint32
	ENBUES1APID uint32
	NASPDU      []byte
}

// UplinkNASTransport carries a NAS PDU from eNB to MME.
// TS 36.413 Section 9.1.5.3
type UplinkNASTransport struct {
	MMEUES1APID uint32
	ENBUES1APID uint32
	NASPDU      []byte
	ECGI        ECGI
	TAI         TAI
}

// EncodeS1SetupResponse encodes an S1SetupResponse into a complete S1AP PDU.
func EncodeS1SetupResponse(resp *S1SetupResponse) ([]byte, error) {
	var ies []ProtocolIE

	// MME Name (optional)
	if resp.MMEName != "" {
		nameEnc := NewPEREncoder()
		if err := nameEnc.PutOctetString([]byte(resp.MMEName)); err != nil {
			return nil, fmt.Errorf("encoding MME name: %w", err)
		}
		ies = append(ies, ProtocolIE{
			ID:          IEID_MMEname,
			Criticality: CriticalityIgnore,
			Value:       nameEnc.Bytes(),
		})
	}

	// ServedGUMMEIs (mandatory)
	gummeisVal, err := EncodeServedGUMMEIs(resp.ServedGUMMEIs)
	if err != nil {
		return nil, fmt.Errorf("encoding served GUMMEIs: %w", err)
	}
	ies = append(ies, ProtocolIE{
		ID:          IEID_ServedGUMMEIs,
		Criticality: CriticalityReject,
		Value:       gummeisVal,
	})

	// RelativeMMECapacity (mandatory)
	capEnc := NewPEREncoder()
	capEnc.PutFixedOctetString([]byte{resp.RelativeCapacity})
	ies = append(ies, ProtocolIE{
		ID:          IEID_RelativeMMECapacity,
		Criticality: CriticalityIgnore,
		Value:       capEnc.Bytes(),
	})

	// Encode IE container
	containerBytes, err := EncodeProtocolIEContainer(ies)
	if err != nil {
		return nil, fmt.Errorf("encoding IE container: %w", err)
	}

	// Wrap in PDU
	pdu := &PDU{
		Type:          PDUSuccessfulOutcome,
		ProcedureCode: ProcS1Setup,
		Criticality:   CriticalityReject,
		Value:         containerBytes,
	}

	return EncodePDU(pdu)
}

// EncodeS1SetupFailure encodes an S1SetupFailure into a complete S1AP PDU.
func EncodeS1SetupFailure(fail *S1SetupFailure) ([]byte, error) {
	// Cause IE
	causeEnc := NewPEREncoder()
	if err := causeEnc.PutChoiceIndex(int(fail.CauseGroup), 5); err != nil {
		return nil, err
	}
	// Each cause group has different ranges; for simplicity encode as constrained int
	causeEnc.align()
	causeEnc.PutBytes([]byte{fail.CauseValue})

	ies := []ProtocolIE{
		{
			ID:          IEID_Cause,
			Criticality: CriticalityIgnore,
			Value:       causeEnc.Bytes(),
		},
	}

	containerBytes, err := EncodeProtocolIEContainer(ies)
	if err != nil {
		return nil, err
	}

	pdu := &PDU{
		Type:          PDUUnsuccessfulOutcome,
		ProcedureCode: ProcS1Setup,
		Criticality:   CriticalityReject,
		Value:         containerBytes,
	}

	return EncodePDU(pdu)
}

// DecodeS1SetupRequest decodes an S1SetupRequest from its ProtocolIE list.
func DecodeS1SetupRequest(ies []ProtocolIE) (*S1SetupRequest, error) {
	req := &S1SetupRequest{}

	for _, ie := range ies {
		switch ie.ID {
		case IEID_Global_ENB_ID:
			g, err := DecodeGlobalENBID(ie.Value)
			if err != nil {
				return nil, fmt.Errorf("decoding Global-ENB-ID: %w", err)
			}
			req.GlobalENBID = g

		case IEID_ENBname:
			dec := NewPERDecoder(ie.Value)
			nameBytes, err := dec.GetOctetString()
			if err != nil {
				return nil, fmt.Errorf("decoding eNB name: %w", err)
			}
			req.ENBName = string(nameBytes)

		case IEID_SupportedTAs:
			tas, err := DecodeSupportedTAs(ie.Value)
			if err != nil {
				return nil, fmt.Errorf("decoding SupportedTAs: %w", err)
			}
			req.SupportedTAs = tas

		case IEID_DefaultPagingDRX:
			if len(ie.Value) < 1 {
				return nil, fmt.Errorf("DefaultPagingDRX value too short")
			}
			dec := NewPERDecoder(ie.Value)
			drx, err := dec.GetConstrainedInt(0, 3)
			if err != nil {
				return nil, fmt.Errorf("decoding DefaultPagingDRX: %w", err)
			}
			req.PagingDRX = uint8(drx)
		}
	}

	return req, nil
}

// EncodeDownlinkNASTransport encodes a DownlinkNASTransport into a complete S1AP PDU.
func EncodeDownlinkNASTransport(msg *DownlinkNASTransport) ([]byte, error) {
	var ies []ProtocolIE

	// MME-UE-S1AP-ID
	mmeIDEnc := NewPEREncoder()
	if err := mmeIDEnc.PutConstrainedInt(int64(msg.MMEUES1APID), 0, 0xFFFFFFFF); err != nil {
		return nil, err
	}
	ies = append(ies, ProtocolIE{
		ID:          IEID_MME_UE_S1AP_ID,
		Criticality: CriticalityReject,
		Value:       mmeIDEnc.Bytes(),
	})

	// eNB-UE-S1AP-ID
	enbIDEnc := NewPEREncoder()
	if err := enbIDEnc.PutConstrainedInt(int64(msg.ENBUES1APID), 0, 0x00FFFFFF); err != nil {
		return nil, err
	}
	ies = append(ies, ProtocolIE{
		ID:          IEID_ENB_UE_S1AP_ID,
		Criticality: CriticalityReject,
		Value:       enbIDEnc.Bytes(),
	})

	// NAS-PDU
	nasEnc := NewPEREncoder()
	if err := nasEnc.PutOctetString(msg.NASPDU); err != nil {
		return nil, err
	}
	ies = append(ies, ProtocolIE{
		ID:          IEID_NAS_PDU,
		Criticality: CriticalityReject,
		Value:       nasEnc.Bytes(),
	})

	containerBytes, err := EncodeProtocolIEContainer(ies)
	if err != nil {
		return nil, err
	}

	pdu := &PDU{
		Type:          PDUInitiatingMessage,
		ProcedureCode: ProcDownlinkNASTransport,
		Criticality:   CriticalityIgnore,
		Value:         containerBytes,
	}

	return EncodePDU(pdu)
}

// DecodeInitialUEMessage decodes an InitialUEMessage from its ProtocolIE list.
func DecodeInitialUEMessage(ies []ProtocolIE) (*InitialUEMessage, error) {
	msg := &InitialUEMessage{}

	for _, ie := range ies {
		switch ie.ID {
		case IEID_ENB_UE_S1AP_ID:
			dec := NewPERDecoder(ie.Value)
			v, err := dec.GetConstrainedInt(0, 0x00FFFFFF)
			if err != nil {
				return nil, fmt.Errorf("decoding eNB-UE-S1AP-ID: %w", err)
			}
			msg.ENBUES1APID = uint32(v)

		case IEID_NAS_PDU:
			dec := NewPERDecoder(ie.Value)
			pdu, err := dec.GetOctetString()
			if err != nil {
				return nil, fmt.Errorf("decoding NAS-PDU: %w", err)
			}
			msg.NASPDU = pdu

		case IEID_TAI:
			tai, err := DecodeTAI(ie.Value)
			if err != nil {
				return nil, fmt.Errorf("decoding TAI: %w", err)
			}
			msg.TAI = tai

		case IEID_EUTRAN_CGI:
			ecgi, err := DecodeECGI(ie.Value)
			if err != nil {
				return nil, fmt.Errorf("decoding ECGI: %w", err)
			}
			msg.ECGI = ecgi

		case IEID_RRC_Establishment_Cause:
			dec := NewPERDecoder(ie.Value)
			v, err := dec.GetConstrainedInt(0, 5)
			if err != nil {
				return nil, fmt.Errorf("decoding RRC cause: %w", err)
			}
			msg.RRCCause = RRCEstablishmentCause(v)
		}
	}

	return msg, nil
}

// UEContextReleaseRequest is sent by the eNB to ask the MME to release a UE context.
// TS 36.413 Section 9.1.4.4 (Procedure Code 18)
type UEContextReleaseRequest struct {
	MMEUES1APID uint32
	ENBUES1APID uint32
	CauseGroup  CauseGroup
	CauseValue  uint8
}

// UEContextReleaseCommand is sent by the MME to instruct the eNB to release a UE context.
// TS 36.413 Section 9.1.4.5 (Procedure Code 23)
type UEContextReleaseCommand struct {
	MMEUES1APID uint32
	ENBUES1APID uint32
	CauseGroup  CauseGroup
	CauseValue  uint8
}

// DecodeUEContextReleaseRequest decodes a UE Context Release Request from its ProtocolIE list.
func DecodeUEContextReleaseRequest(ies []ProtocolIE) (*UEContextReleaseRequest, error) {
	req := &UEContextReleaseRequest{}
	for _, ie := range ies {
		switch ie.ID {
		case IEID_MME_UE_S1AP_ID:
			dec := NewPERDecoder(ie.Value)
			v, err := dec.GetConstrainedInt(0, 0xFFFFFFFF)
			if err != nil {
				return nil, fmt.Errorf("decoding MME-UE-S1AP-ID: %w", err)
			}
			req.MMEUES1APID = uint32(v)
		case IEID_ENB_UE_S1AP_ID:
			dec := NewPERDecoder(ie.Value)
			v, err := dec.GetConstrainedInt(0, 0x00FFFFFF)
			if err != nil {
				return nil, fmt.Errorf("decoding eNB-UE-S1AP-ID: %w", err)
			}
			req.ENBUES1APID = uint32(v)
		case IEID_Cause:
			// Cause: CHOICE { radioNetwork, transport, nas, protocol, misc }
			// Encoded as choice index (2 bits aligned) + cause value (1 byte aligned)
			if len(ie.Value) >= 2 {
				dec := NewPERDecoder(ie.Value)
				group, err := dec.GetChoiceIndex(5)
				if err == nil {
					req.CauseGroup = CauseGroup(group)
					dec.align()
					if b, e := dec.GetBytes(1); e == nil {
						req.CauseValue = b[0]
					}
				}
			}
		}
	}
	return req, nil
}

// EncodeUEContextReleaseCommand encodes a UE Context Release Command PDU.
// The UE is identified by the MME-UE-S1AP-ID + eNB-UE-S1AP-ID pair.
func EncodeUEContextReleaseCommand(cmd *UEContextReleaseCommand) ([]byte, error) {
	// UE-S1AP-IDs IE: encode as the pair (choice index 0)
	idsEnc := NewPEREncoder()
	if err := idsEnc.PutChoiceIndex(0, 2); err != nil { // 0 = UE-S1AP-ID-pair
		return nil, err
	}
	idsEnc.align()
	// mME-UE-S1AP-ID
	mmeIDEnc := NewPEREncoder()
	if err := mmeIDEnc.PutConstrainedInt(int64(cmd.MMEUES1APID), 0, 0xFFFFFFFF); err != nil {
		return nil, err
	}
	// eNB-UE-S1AP-ID
	enbIDEnc := NewPEREncoder()
	if err := enbIDEnc.PutConstrainedInt(int64(cmd.ENBUES1APID), 0, 0x00FFFFFF); err != nil {
		return nil, err
	}
	// Pack both IDs together
	pairEnc := NewPEREncoder()
	if err := pairEnc.PutConstrainedInt(int64(cmd.MMEUES1APID), 0, 0xFFFFFFFF); err != nil {
		return nil, err
	}
	if err := pairEnc.PutConstrainedInt(int64(cmd.ENBUES1APID), 0, 0x00FFFFFF); err != nil {
		return nil, err
	}

	// Cause IE
	causeEnc := NewPEREncoder()
	if err := causeEnc.PutChoiceIndex(int(cmd.CauseGroup), 5); err != nil {
		return nil, err
	}
	causeEnc.align()
	causeEnc.PutBytes([]byte{cmd.CauseValue})

	ies := []ProtocolIE{
		{ID: IEID_UE_S1AP_IDs, Criticality: CriticalityReject, Value: pairEnc.Bytes()},
		{ID: IEID_Cause, Criticality: CriticalityIgnore, Value: causeEnc.Bytes()},
	}

	containerBytes, err := EncodeProtocolIEContainer(ies)
	if err != nil {
		return nil, err
	}

	pdu := &PDU{
		Type:          PDUInitiatingMessage,
		ProcedureCode: ProcUEContextRelease,
		Criticality:   CriticalityReject,
		Value:         containerBytes,
	}
	return EncodePDU(pdu)
}

// InitialContextSetupRequest is sent by the MME to the eNB to establish a UE context
// and activate radio bearers. It carries the KeNB and the NAS ATTACH ACCEPT.
// TS 36.413 Section 9.1.4.1 (Procedure Code 9)
type InitialContextSetupRequest struct {
	MMEUES1APID       uint32
	ENBUES1APID       uint32
	UEAggMaxBitRateDL uint64 // bits/sec
	UEAggMaxBitRateUL uint64 // bits/sec
	ERABs             []ERABToSetup
	UESecEncAlgs      [2]byte  // 16-bit encryption algorithm bitmap (MSB first)
	UESecIntAlgs      [2]byte  // 16-bit integrity algorithm bitmap (MSB first)
	SecurityKey       [32]byte // KeNB (256 bits)
}

// ERABToSetup describes one E-RAB in an INITIAL CONTEXT SETUP REQUEST.
type ERABToSetup struct {
	ERABID             uint8  // typically 5 for the default bearer
	QCI                uint8  // QCI (e.g., 9 for internet non-GBR)
	ARPLevel           uint8  // Allocation & Retention Priority level (0–15)
	TransportLayerAddr net.IP // S-GW S1-U IP (IPv4)
	GTPTEID            [4]byte
	NASPDU             []byte // optional NAS PDU (ATTACH ACCEPT) embedded for UE
}

// EncodeInitialContextSetupRequest encodes an INITIAL CONTEXT SETUP REQUEST PDU.
func EncodeInitialContextSetupRequest(req *InitialContextSetupRequest) ([]byte, error) {
	var ies []ProtocolIE

	// MME-UE-S1AP-ID
	mmeEnc := NewPEREncoder()
	if err := mmeEnc.PutConstrainedInt(int64(req.MMEUES1APID), 0, 0xFFFFFFFF); err != nil {
		return nil, err
	}
	ies = append(ies, ProtocolIE{ID: IEID_MME_UE_S1AP_ID, Criticality: CriticalityReject, Value: mmeEnc.Bytes()})

	// eNB-UE-S1AP-ID
	enbEnc := NewPEREncoder()
	if err := enbEnc.PutConstrainedInt(int64(req.ENBUES1APID), 0, 0x00FFFFFF); err != nil {
		return nil, err
	}
	ies = append(ies, ProtocolIE{ID: IEID_ENB_UE_S1AP_ID, Criticality: CriticalityReject, Value: enbEnc.Bytes()})

	// UEAggregateMaximumBitrate: SEQUENCE { dl BitRate(0..10000000000), ul BitRate(0..10000000000) }
	// Each BitRate is a constrained INTEGER with range > 65536 → unconstrained encoding
	aggEnc := NewPEREncoder()
	aggEnc.PutSequenceHeader(true, 0, 0)
	if err := aggEnc.PutConstrainedInt(int64(req.UEAggMaxBitRateDL), 0, 10000000000); err != nil {
		return nil, fmt.Errorf("encoding DL bit rate: %w", err)
	}
	if err := aggEnc.PutConstrainedInt(int64(req.UEAggMaxBitRateUL), 0, 10000000000); err != nil {
		return nil, fmt.Errorf("encoding UL bit rate: %w", err)
	}
	ies = append(ies, ProtocolIE{ID: IEID_UEAggMaxBitRate, Criticality: CriticalityReject, Value: aggEnc.Bytes()})

	// E-RABToBeSetupListCtxtSUReq
	erabListVal, err := encodeERABSetupList(req.ERABs)
	if err != nil {
		return nil, fmt.Errorf("encoding E-RAB list: %w", err)
	}
	ies = append(ies, ProtocolIE{ID: IEID_E_RABToBeSetupListCtxtSUReq, Criticality: CriticalityReject, Value: erabListVal})

	// UESecurityCapabilities: SEQUENCE { encAlgs BIT STRING(16), intAlgs BIT STRING(16), ... }
	secCapEnc := NewPEREncoder()
	secCapEnc.PutSequenceHeader(true, 0, 1) // extensible, 1 optional (iE-Extensions), not present
	secCapEnc.PutFixedOctetString(req.UESecEncAlgs[:])
	secCapEnc.PutFixedOctetString(req.UESecIntAlgs[:])
	ies = append(ies, ProtocolIE{ID: IEID_UESecurityCapabilities, Criticality: CriticalityReject, Value: secCapEnc.Bytes()})

	// SecurityKey: BIT STRING(256) — KeNB, fixed 32 bytes, no length determinant
	keyEnc := NewPEREncoder()
	keyEnc.PutBitString(req.SecurityKey[:], 256)
	ies = append(ies, ProtocolIE{ID: IEID_SecurityKey, Criticality: CriticalityReject, Value: keyEnc.Bytes()})

	containerBytes, err := EncodeProtocolIEContainer(ies)
	if err != nil {
		return nil, err
	}

	pdu := &PDU{
		Type:          PDUInitiatingMessage,
		ProcedureCode: ProcInitialContextSetup,
		Criticality:   CriticalityReject,
		Value:         containerBytes,
	}
	return EncodePDU(pdu)
}

// encodeERABSetupList encodes a E-RABToBeSetupListCtxtSUReq IE value.
// This is a SEQUENCE (SIZE(1..256)) OF ProtocolIE-SingleContainer items.
func encodeERABSetupList(erabs []ERABToSetup) ([]byte, error) {
	enc := NewPEREncoder()
	// Count: constrained int 1..256 (range 256 → 1 aligned byte)
	if err := enc.PutConstrainedInt(int64(len(erabs)), 1, 256); err != nil {
		return nil, err
	}
	for _, erab := range erabs {
		itemVal, err := encodeERABSetupItem(erab)
		if err != nil {
			return nil, err
		}
		// Each element is a ProtocolIE-SingleContainer
		// id(2) + criticality(1 byte, aligned) + length + value
		idBytes := make([]byte, 2)
		binary.BigEndian.PutUint16(idBytes, uint16(IEID_E_RABToBeSetupItemCtxtSUReq))
		enc.PutBytes(idBytes)
		// criticality = reject = 0 (2 bits, then align)
		if err := enc.PutConstrainedInt(int64(CriticalityReject), 0, 2); err != nil {
			return nil, err
		}
		enc.align()
		if err := enc.PutLengthDeterminant(len(itemVal)); err != nil {
			return nil, err
		}
		enc.PutBytes(itemVal)
	}
	return enc.Bytes(), nil
}

// encodeERABSetupItem encodes an E-RABToBeSetupItemCtxtSUReq value (the OPEN TYPE contents).
func encodeERABSetupItem(erab ERABToSetup) ([]byte, error) {
	enc := NewPEREncoder()

	// SEQUENCE { e-RAB-ID, qosParams, transportAddr, gtpTEID, nAS-PDU OPTIONAL, iE-Ext OPTIONAL, ... }
	// Extension bit = 0; optional bitmap: [nAS-PDU present, iE-Ext present]
	nasPDUPresent := len(erab.NASPDU) > 0
	optBits := uint64(0)
	if nasPDUPresent {
		optBits |= 1 // bit 0 = nAS-PDU
	}
	enc.PutSequenceHeader(true, optBits, 2) // extensible, 2 optionals

	// e-RAB-ID: INTEGER (0..15), 4 bits
	if err := enc.PutConstrainedInt(int64(erab.ERABID), 0, 15); err != nil {
		return nil, fmt.Errorf("encoding E-RAB-ID: %w", err)
	}

	// E-RABLevelQoSParameters: SEQUENCE { qCI(0..255), arp, gbrQos OPTIONAL, iE-Ext OPTIONAL, ... }
	enc.PutSequenceHeader(true, 0, 2) // extensible, 2 optionals, none present
	// QCI: INTEGER (0..255), 1 byte (range 256)
	if err := enc.PutConstrainedInt(int64(erab.QCI), 0, 255); err != nil {
		return nil, fmt.Errorf("encoding QCI: %w", err)
	}
	// AllocationAndRetentionPriority: SEQUENCE { level(0..15), preemptCap(0..1), preemptVuln(0..1), iE-Ext OPTIONAL, ... }
	enc.PutSequenceHeader(true, 0, 1) // extensible, 1 optional, not present
	if err := enc.PutConstrainedInt(int64(erab.ARPLevel), 0, 15); err != nil {
		return nil, fmt.Errorf("encoding ARP level: %w", err)
	}
	enc.putBits(0, 1) // pre-emption-capability: shall-not-trigger (0)
	enc.putBits(1, 1) // pre-emption-vulnerability: pre-emptable (1)

	// TransportLayerAddress: BIT STRING (SIZE(1..160))
	// For IPv4: 32 bits; length determinant = constrained int(1..160) for value 32
	ipv4 := erab.TransportLayerAddr.To4()
	if ipv4 == nil {
		return nil, fmt.Errorf("only IPv4 transport layer addresses supported")
	}
	enc.align()
	if err := enc.PutConstrainedInt(32, 1, 160); err != nil { // 32 bits for IPv4
		return nil, fmt.Errorf("encoding transport addr length: %w", err)
	}
	enc.PutBytes(ipv4)

	// GTP-TEID: OCTET STRING (SIZE(4)) — fixed, no length prefix
	enc.PutFixedOctetString(erab.GTPTEID[:])

	// nAS-PDU (optional)
	if nasPDUPresent {
		if err := enc.PutOctetString(erab.NASPDU); err != nil {
			return nil, fmt.Errorf("encoding NAS PDU: %w", err)
		}
	}

	return enc.Bytes(), nil
}

// DecodeInitialContextSetupResponse decodes the eNB's response to the INITIAL CONTEXT SETUP.
// We only need the MME-UE-S1AP-ID to match it to a UE; the rest (E-RAB setup results) are ignored.
type InitialContextSetupResponse struct {
	MMEUES1APID uint32
	ENBUES1APID uint32
}

func DecodeInitialContextSetupResponse(ies []ProtocolIE) (*InitialContextSetupResponse, error) {
	resp := &InitialContextSetupResponse{}
	for _, ie := range ies {
		switch ie.ID {
		case IEID_MME_UE_S1AP_ID:
			dec := NewPERDecoder(ie.Value)
			v, err := dec.GetConstrainedInt(0, 0xFFFFFFFF)
			if err != nil {
				return nil, fmt.Errorf("decoding MME-UE-S1AP-ID: %w", err)
			}
			resp.MMEUES1APID = uint32(v)
		case IEID_ENB_UE_S1AP_ID:
			dec := NewPERDecoder(ie.Value)
			v, err := dec.GetConstrainedInt(0, 0x00FFFFFF)
			if err != nil {
				return nil, fmt.Errorf("decoding eNB-UE-S1AP-ID: %w", err)
			}
			resp.ENBUES1APID = uint32(v)
		}
	}
	return resp, nil
}

// DecodeUplinkNASTransport decodes an UplinkNASTransport from its ProtocolIE list.
func DecodeUplinkNASTransport(ies []ProtocolIE) (*UplinkNASTransport, error) {
	msg := &UplinkNASTransport{}

	for _, ie := range ies {
		switch ie.ID {
		case IEID_MME_UE_S1AP_ID:
			dec := NewPERDecoder(ie.Value)
			v, err := dec.GetConstrainedInt(0, 0xFFFFFFFF)
			if err != nil {
				return nil, err
			}
			msg.MMEUES1APID = uint32(v)

		case IEID_ENB_UE_S1AP_ID:
			dec := NewPERDecoder(ie.Value)
			v, err := dec.GetConstrainedInt(0, 0x00FFFFFF)
			if err != nil {
				return nil, err
			}
			msg.ENBUES1APID = uint32(v)

		case IEID_NAS_PDU:
			dec := NewPERDecoder(ie.Value)
			pdu, err := dec.GetOctetString()
			if err != nil {
				return nil, err
			}
			msg.NASPDU = pdu

		case IEID_EUTRAN_CGI:
			ecgi, err := DecodeECGI(ie.Value)
			if err != nil {
				return nil, err
			}
			msg.ECGI = ecgi

		case IEID_TAI:
			tai, err := DecodeTAI(ie.Value)
			if err != nil {
				return nil, err
			}
			msg.TAI = tai
		}
	}

	return msg, nil
}
