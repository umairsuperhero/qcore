package s1ap

import "fmt"

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
// (eNB → MME) confirming radio bearer establishment for a UE.
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
