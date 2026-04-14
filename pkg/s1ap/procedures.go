package s1ap

import (
	"fmt"
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
