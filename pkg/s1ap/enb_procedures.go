package s1ap

import (
	"encoding/binary"
	"fmt"
)

// This file contains the eNB-side encoders and decoders (counterparts to the
// MME-side encoders in procedures.go). These are needed for integration tests
// and for any Go program that wants to act as a fake/test eNB against the MME.

// EncodeS1SetupRequest encodes an S1 SETUP REQUEST (eNB → MME).
func EncodeS1SetupRequest(req *S1SetupRequest) ([]byte, error) {
	var ies []ProtocolIE

	// Global-ENB-ID (mandatory)
	gBytes, err := EncodeGlobalENBID(req.GlobalENBID)
	if err != nil {
		return nil, fmt.Errorf("encoding Global-ENB-ID: %w", err)
	}
	ies = append(ies, ProtocolIE{ID: IEID_Global_ENB_ID, Criticality: CriticalityReject, Value: gBytes})

	// eNB Name (optional)
	if req.ENBName != "" {
		enc := NewPEREncoder()
		if err := enc.PutOctetString([]byte(req.ENBName)); err != nil {
			return nil, fmt.Errorf("encoding eNB name: %w", err)
		}
		ies = append(ies, ProtocolIE{ID: IEID_ENBname, Criticality: CriticalityIgnore, Value: enc.Bytes()})
	}

	// Supported TAs (mandatory)
	tasBytes, err := EncodeSupportedTAs(req.SupportedTAs)
	if err != nil {
		return nil, fmt.Errorf("encoding SupportedTAs: %w", err)
	}
	ies = append(ies, ProtocolIE{ID: IEID_SupportedTAs, Criticality: CriticalityReject, Value: tasBytes})

	// Default Paging DRX (mandatory): ENUMERATED v32, v64, v128, v256
	drxEnc := NewPEREncoder()
	if err := drxEnc.PutConstrainedInt(int64(req.PagingDRX), 0, 3); err != nil {
		return nil, err
	}
	ies = append(ies, ProtocolIE{ID: IEID_DefaultPagingDRX, Criticality: CriticalityIgnore, Value: drxEnc.Bytes()})

	containerBytes, err := EncodeProtocolIEContainer(ies)
	if err != nil {
		return nil, err
	}

	return EncodePDU(&PDU{
		Type:          PDUInitiatingMessage,
		ProcedureCode: ProcS1Setup,
		Criticality:   CriticalityReject,
		Value:         containerBytes,
	})
}

// EncodeInitialUEMessage encodes an INITIAL UE MESSAGE (eNB → MME).
func EncodeInitialUEMessage(msg *InitialUEMessage) ([]byte, error) {
	var ies []ProtocolIE

	// eNB-UE-S1AP-ID (mandatory)
	enbIDEnc := NewPEREncoder()
	if err := enbIDEnc.PutConstrainedInt(int64(msg.ENBUES1APID), 0, 0x00FFFFFF); err != nil {
		return nil, err
	}
	ies = append(ies, ProtocolIE{ID: IEID_ENB_UE_S1AP_ID, Criticality: CriticalityReject, Value: enbIDEnc.Bytes()})

	// NAS-PDU (mandatory)
	nasEnc := NewPEREncoder()
	if err := nasEnc.PutOctetString(msg.NASPDU); err != nil {
		return nil, err
	}
	ies = append(ies, ProtocolIE{ID: IEID_NAS_PDU, Criticality: CriticalityReject, Value: nasEnc.Bytes()})

	// TAI (mandatory)
	taiBytes, err := EncodeTAI(msg.TAI)
	if err != nil {
		return nil, err
	}
	ies = append(ies, ProtocolIE{ID: IEID_TAI, Criticality: CriticalityIgnore, Value: taiBytes})

	// E-UTRAN-CGI (mandatory)
	ecgiBytes, err := EncodeECGI(msg.ECGI)
	if err != nil {
		return nil, err
	}
	ies = append(ies, ProtocolIE{ID: IEID_EUTRAN_CGI, Criticality: CriticalityIgnore, Value: ecgiBytes})

	// RRC Establishment Cause (mandatory)
	causeEnc := NewPEREncoder()
	if err := causeEnc.PutConstrainedInt(int64(msg.RRCCause), 0, 5); err != nil {
		return nil, err
	}
	ies = append(ies, ProtocolIE{ID: IEID_RRC_Establishment_Cause, Criticality: CriticalityIgnore, Value: causeEnc.Bytes()})

	containerBytes, err := EncodeProtocolIEContainer(ies)
	if err != nil {
		return nil, err
	}

	return EncodePDU(&PDU{
		Type:          PDUInitiatingMessage,
		ProcedureCode: ProcInitialUEMessage,
		Criticality:   CriticalityIgnore,
		Value:         containerBytes,
	})
}

// EncodeUplinkNASTransport encodes an UPLINK NAS TRANSPORT (eNB → MME).
func EncodeUplinkNASTransport(msg *UplinkNASTransport) ([]byte, error) {
	var ies []ProtocolIE

	mmeIDEnc := NewPEREncoder()
	if err := mmeIDEnc.PutConstrainedInt(int64(msg.MMEUES1APID), 0, 0xFFFFFFFF); err != nil {
		return nil, err
	}
	ies = append(ies, ProtocolIE{ID: IEID_MME_UE_S1AP_ID, Criticality: CriticalityReject, Value: mmeIDEnc.Bytes()})

	enbIDEnc := NewPEREncoder()
	if err := enbIDEnc.PutConstrainedInt(int64(msg.ENBUES1APID), 0, 0x00FFFFFF); err != nil {
		return nil, err
	}
	ies = append(ies, ProtocolIE{ID: IEID_ENB_UE_S1AP_ID, Criticality: CriticalityReject, Value: enbIDEnc.Bytes()})

	nasEnc := NewPEREncoder()
	if err := nasEnc.PutOctetString(msg.NASPDU); err != nil {
		return nil, err
	}
	ies = append(ies, ProtocolIE{ID: IEID_NAS_PDU, Criticality: CriticalityReject, Value: nasEnc.Bytes()})

	// EUTRAN-CGI and TAI (mandatory)
	ecgiBytes, err := EncodeECGI(msg.ECGI)
	if err != nil {
		return nil, err
	}
	ies = append(ies, ProtocolIE{ID: IEID_EUTRAN_CGI, Criticality: CriticalityIgnore, Value: ecgiBytes})

	taiBytes, err := EncodeTAI(msg.TAI)
	if err != nil {
		return nil, err
	}
	ies = append(ies, ProtocolIE{ID: IEID_TAI, Criticality: CriticalityIgnore, Value: taiBytes})

	containerBytes, err := EncodeProtocolIEContainer(ies)
	if err != nil {
		return nil, err
	}

	return EncodePDU(&PDU{
		Type:          PDUInitiatingMessage,
		ProcedureCode: ProcUplinkNASTransport,
		Criticality:   CriticalityIgnore,
		Value:         containerBytes,
	})
}

// DecodeDownlinkNASTransport decodes a DOWNLINK NAS TRANSPORT (MME → eNB).
func DecodeDownlinkNASTransport(ies []ProtocolIE) (*DownlinkNASTransport, error) {
	msg := &DownlinkNASTransport{}
	for _, ie := range ies {
		switch ie.ID {
		case IEID_MME_UE_S1AP_ID:
			dec := NewPERDecoder(ie.Value)
			v, err := dec.GetConstrainedInt(0, 0xFFFFFFFF)
			if err != nil {
				return nil, fmt.Errorf("decoding MME-UE-S1AP-ID: %w", err)
			}
			msg.MMEUES1APID = uint32(v)
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
		}
	}
	return msg, nil
}

// EncodeInitialContextSetupResponse encodes an INITIAL CONTEXT SETUP RESPONSE
// (eNB → MME) confirming radio bearer establishment for a UE. If ERABs is
// populated, an E-RABSetupListCtxtSURes IE is appended so the MME learns the
// eNB-allocated S1-U TEID for each bearer.
func EncodeInitialContextSetupResponse(resp *InitialContextSetupResponse) ([]byte, error) {
	var ies []ProtocolIE

	mmeIDEnc := NewPEREncoder()
	if err := mmeIDEnc.PutConstrainedInt(int64(resp.MMEUES1APID), 0, 0xFFFFFFFF); err != nil {
		return nil, err
	}
	ies = append(ies, ProtocolIE{ID: IEID_MME_UE_S1AP_ID, Criticality: CriticalityReject, Value: mmeIDEnc.Bytes()})

	enbIDEnc := NewPEREncoder()
	if err := enbIDEnc.PutConstrainedInt(int64(resp.ENBUES1APID), 0, 0x00FFFFFF); err != nil {
		return nil, err
	}
	ies = append(ies, ProtocolIE{ID: IEID_ENB_UE_S1AP_ID, Criticality: CriticalityReject, Value: enbIDEnc.Bytes()})

	if len(resp.ERABs) > 0 {
		listVal, err := encodeERABSetupResultList(resp.ERABs)
		if err != nil {
			return nil, fmt.Errorf("encoding E-RABSetupListCtxtSURes: %w", err)
		}
		ies = append(ies, ProtocolIE{ID: IEID_E_RABSetupListCtxtSURes, Criticality: CriticalityIgnore, Value: listVal})
	}

	containerBytes, err := EncodeProtocolIEContainer(ies)
	if err != nil {
		return nil, err
	}

	return EncodePDU(&PDU{
		Type:          PDUSuccessfulOutcome,
		ProcedureCode: ProcInitialContextSetup,
		Criticality:   CriticalityReject,
		Value:         containerBytes,
	})
}

// encodeERABSetupResultList mirrors encodeERABSetupList (request side) but
// encodes the response-side item (E-RABSetupItemCtxtSURes).
func encodeERABSetupResultList(items []ERABSetupResult) ([]byte, error) {
	enc := NewPEREncoder()
	if err := enc.PutConstrainedInt(int64(len(items)), 1, 256); err != nil {
		return nil, err
	}
	for _, it := range items {
		itemBytes, err := encodeERABSetupResultItem(it)
		if err != nil {
			return nil, err
		}
		idBytes := make([]byte, 2)
		binary.BigEndian.PutUint16(idBytes, uint16(IEID_E_RABSetupItemCtxtSURes))
		enc.PutBytes(idBytes)
		if err := enc.PutConstrainedInt(int64(CriticalityIgnore), 0, 2); err != nil {
			return nil, err
		}
		enc.align()
		if err := enc.PutLengthDeterminant(len(itemBytes)); err != nil {
			return nil, err
		}
		enc.PutBytes(itemBytes)
	}
	return enc.Bytes(), nil
}

// encodeERABSetupResultItem encodes E-RABSetupItemCtxtSURes:
//   SEQUENCE { e-RAB-ID, transportLayerAddress, gTP-TEID, iE-Extensions OPTIONAL, ... }
// Extensible, one OPTIONAL field (iE-Extensions), not present.
func encodeERABSetupResultItem(it ERABSetupResult) ([]byte, error) {
	enc := NewPEREncoder()
	enc.PutSequenceHeader(true, 0, 1) // extensible, 1 optional (iE-Extensions), absent
	if err := enc.PutConstrainedInt(int64(it.ERABID), 0, 15); err != nil {
		return nil, fmt.Errorf("e-RAB-ID: %w", err)
	}
	ip := it.TransportLayerAddr.To4()
	if ip == nil {
		return nil, fmt.Errorf("transportLayerAddress must be IPv4")
	}
	// BIT STRING(SIZE(1..160,...)) — emit a 32-bit length determinant then the 4 bytes.
	// This matches the request-side encoder in encodeERABSetupItem.
	if err := enc.PutLengthDeterminant(32); err != nil {
		return nil, err
	}
	enc.align()
	enc.PutBytes(ip)
	enc.PutFixedOctetString(it.GTPTEID[:])
	return enc.Bytes(), nil
}
