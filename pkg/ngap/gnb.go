package ngap

import "fmt"

// This file contains gNB → AMF message encoders and AMF → gNB message decoders
// (the gNB's perspective). Used by the AMF to decode incoming NGAP messages,
// and by tests to simulate a gNB.

// --- NGSetup -----------------------------------------------------------------

// NGSetupRequest is sent by the gNB to initiate NG Setup with the AMF.
// TS 38.413 §9.2.6.1
type NGSetupRequest struct {
	GlobalRANNodeID GlobalGNBID
	RANNodeName     string // optional
	SupportedTAList []SupportedTA
	DefaultPagingDRX uint8 // 0=v32, 1=v64, 2=v128, 3=v256
}

// EncodeNGSetupRequest encodes an NGSetupRequest into a complete NGAP PDU.
func EncodeNGSetupRequest(req *NGSetupRequest) ([]byte, error) {
	var ies []ProtocolIE

	// GlobalRANNodeID (mandatory)
	// GlobalRANNodeID ::= CHOICE { globalGNB-ID(0), globalNgENB-ID(1), globalN3IWF-ID(2), ... }
	// Wrap in the CHOICE: ext bit (0) + index 0 (for globalGNB-ID)
	choiceEnc := NewPEREncoder()
	_ = choiceEnc.PutChoiceIndex(0, 3, true) // globalGNB-ID = 0
	if err := writeGlobalGNBID(choiceEnc, req.GlobalRANNodeID); err != nil {
		return nil, fmt.Errorf("ngap: GlobalRANNodeID: %w", err)
	}
	ies = append(ies, ProtocolIE{ID: IEIDGlobalRANNodeID, Criticality: CriticalityReject, Value: choiceEnc.Bytes()})

	// RANNodeName (optional)
	if req.RANNodeName != "" {
		nameEnc := NewPEREncoder()
		if err := nameEnc.PutPrintableString(req.RANNodeName, 1, 150, true); err != nil {
			return nil, err
		}
		ies = append(ies, ProtocolIE{ID: IEIDRANNodeName, Criticality: CriticalityIgnore, Value: nameEnc.Bytes()})
	}

	// SupportedTAList (mandatory)
	taListVal, err := EncodeSupportedTAList(req.SupportedTAList)
	if err != nil {
		return nil, fmt.Errorf("ngap: SupportedTAList: %w", err)
	}
	ies = append(ies, ProtocolIE{ID: IEIDSupportedTAList, Criticality: CriticalityReject, Value: taListVal})

	// DefaultPagingDRX (mandatory): ENUMERATED { v32(0), v64(1), v128(2), v256(3) }
	drxEnc := NewPEREncoder()
	if err := drxEnc.PutConstrainedInt(int64(req.DefaultPagingDRX), 0, 3); err != nil {
		return nil, err
	}
	ies = append(ies, ProtocolIE{ID: IEIDDefaultPagingDRX, Criticality: CriticalityIgnore, Value: drxEnc.Bytes()})

	containerBytes, err := EncodeIEContainer(ies)
	if err != nil {
		return nil, err
	}
	return EncodePDU(&PDU{
		Type:          PDUInitiatingMessage,
		ProcedureCode: ProcNGSetup,
		Criticality:   CriticalityReject,
		Value:         containerBytes,
	})
}

// DecodeNGSetupRequest decodes an NGSetupRequest from its IE list.
func DecodeNGSetupRequest(ies []ProtocolIE) (*NGSetupRequest, error) {
	req := &NGSetupRequest{}

	for _, ie := range ies {
		var err error
		switch ie.ID {
		case IEIDGlobalRANNodeID:
			// Unwrap GlobalRANNodeID CHOICE: ext bit + index + GlobalGNB-ID bytes
			dec := NewPERDecoder(ie.Value)
			idx, e := dec.GetChoiceIndex(3, true)
			if e != nil {
				return nil, fmt.Errorf("ngap: GlobalRANNodeID index: %w", e)
			}
			if idx != 0 {
				return nil, fmt.Errorf("ngap: GlobalRANNodeID: only globalGNB-ID supported, got %d", idx)
			}
			req.GlobalRANNodeID, err = readGlobalGNBID(dec)

		case IEIDRANNodeName:
			dec := NewPERDecoder(ie.Value)
			req.RANNodeName, err = dec.GetPrintableString(1, 150, true)

		case IEIDSupportedTAList:
			req.SupportedTAList, err = DecodeSupportedTAList(ie.Value)

		case IEIDDefaultPagingDRX:
			dec := NewPERDecoder(ie.Value)
			var v int64
			v, err = dec.GetConstrainedInt(0, 3)
			req.DefaultPagingDRX = uint8(v)
		}
		if err != nil {
			return nil, fmt.Errorf("ngap: NGSetupRequest IE %d: %w", ie.ID, err)
		}
	}
	return req, nil
}

// DecodeNGSetupResponse decodes an NGSetupResponse from its IE list.
func DecodeNGSetupResponse(ies []ProtocolIE) (*NGSetupResponse, error) {
	resp := &NGSetupResponse{}
	for _, ie := range ies {
		var err error
		switch ie.ID {
		case IEIDAMFName:
			dec := NewPERDecoder(ie.Value)
			name, err := dec.GetPrintableString(1, 150, true)
			if err == nil {
				resp.AMFName = name
			}
		case IEIDServedGUAMIList:
			resp.ServedGUAMIList, err = DecodeServedGUAMIList(ie.Value)
		case IEIDRelativeAMFCapacity:
			if len(ie.Value) >= 1 {
				resp.RelativeCapacity = ie.Value[0]
			}
		}
		if err != nil {
			return nil, fmt.Errorf("ngap: NGSetupResponse IE %d: %w", ie.ID, err)
		}
	}
	return resp, nil
}

// --- Initial UE Message ------------------------------------------------------

// InitialUEMessage is sent by the gNB to the AMF when a UE initiates contact.
// TS 38.413 §9.2.5.1
type InitialUEMessage struct {
	RANUENGAPID          uint64
	NASPDU               []byte
	UserLocationInfo     UserLocationInformationNR
	RRCEstablishmentCause RRCEstablishmentCause
}

// EncodeInitialUEMessage encodes an InitialUEMessage into a complete NGAP PDU.
func EncodeInitialUEMessage(msg *InitialUEMessage) ([]byte, error) {
	ranIDVal, err := EncodeRANUENGAPID(msg.RANUENGAPID)
	if err != nil {
		return nil, err
	}
	nasEnc := NewPEREncoder()
	if err := nasEnc.PutOctetString(msg.NASPDU); err != nil {
		return nil, err
	}
	rrcEnc := NewPEREncoder()
	if err := rrcEnc.PutConstrainedInt(int64(msg.RRCEstablishmentCause), 0, 9); err != nil {
		return nil, err
	}

	ies := []ProtocolIE{
		{ID: IEIDRANUENGAPID, Criticality: CriticalityReject, Value: ranIDVal},
		{ID: IEIDNASPDU, Criticality: CriticalityReject, Value: nasEnc.Bytes()},
		{ID: IEIDUserLocationInfo, Criticality: CriticalityReject, Value: EncodeUserLocationInformationNR(msg.UserLocationInfo)},
		{ID: IEIDRRCEstablishmentCause, Criticality: CriticalityIgnore, Value: rrcEnc.Bytes()},
	}
	containerBytes, err := EncodeIEContainer(ies)
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

// DecodeInitialUEMessage decodes an InitialUEMessage from its IE list.
func DecodeInitialUEMessage(ies []ProtocolIE) (*InitialUEMessage, error) {
	msg := &InitialUEMessage{}
	for _, ie := range ies {
		var err error
		switch ie.ID {
		case IEIDRANUENGAPID:
			msg.RANUENGAPID, err = DecodeRANUENGAPID(ie.Value)
		case IEIDNASPDU:
			dec := NewPERDecoder(ie.Value)
			msg.NASPDU, err = dec.GetOctetString()
		case IEIDUserLocationInfo:
			msg.UserLocationInfo, err = DecodeUserLocationInformationNR(ie.Value)
		case IEIDRRCEstablishmentCause:
			dec := NewPERDecoder(ie.Value)
			var v int64
			v, err = dec.GetConstrainedInt(0, 9)
			msg.RRCEstablishmentCause = RRCEstablishmentCause(v)
		}
		if err != nil {
			return nil, fmt.Errorf("ngap: InitialUEMessage IE %d: %w", ie.ID, err)
		}
	}
	return msg, nil
}

// --- Uplink NAS Transport ----------------------------------------------------

// UplinkNASTransport carries a NAS PDU from gNB to AMF.
// TS 38.413 §9.2.5.3
type UplinkNASTransport struct {
	AMFUENGAPID      uint64
	RANUENGAPID      uint64
	NASPDU           []byte
	UserLocationInfo UserLocationInformationNR
}

// EncodeUplinkNASTransport encodes an UplinkNASTransport into a complete NGAP PDU.
func EncodeUplinkNASTransport(msg *UplinkNASTransport) ([]byte, error) {
	amfIDVal, err := EncodeAMFUENGAPID(msg.AMFUENGAPID)
	if err != nil {
		return nil, err
	}
	ranIDVal, err := EncodeRANUENGAPID(msg.RANUENGAPID)
	if err != nil {
		return nil, err
	}
	nasEnc := NewPEREncoder()
	if err := nasEnc.PutOctetString(msg.NASPDU); err != nil {
		return nil, err
	}

	ies := []ProtocolIE{
		{ID: IEIDAMFUENGAPID, Criticality: CriticalityReject, Value: amfIDVal},
		{ID: IEIDRANUENGAPID, Criticality: CriticalityReject, Value: ranIDVal},
		{ID: IEIDNASPDU, Criticality: CriticalityReject, Value: nasEnc.Bytes()},
		{ID: IEIDUserLocationInfo, Criticality: CriticalityReject, Value: EncodeUserLocationInformationNR(msg.UserLocationInfo)},
	}
	containerBytes, err := EncodeIEContainer(ies)
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

// DecodeUplinkNASTransport decodes an UplinkNASTransport from its IE list.
func DecodeUplinkNASTransport(ies []ProtocolIE) (*UplinkNASTransport, error) {
	msg := &UplinkNASTransport{}
	for _, ie := range ies {
		var err error
		switch ie.ID {
		case IEIDAMFUENGAPID:
			msg.AMFUENGAPID, err = DecodeAMFUENGAPID(ie.Value)
		case IEIDRANUENGAPID:
			msg.RANUENGAPID, err = DecodeRANUENGAPID(ie.Value)
		case IEIDNASPDU:
			dec := NewPERDecoder(ie.Value)
			msg.NASPDU, err = dec.GetOctetString()
		case IEIDUserLocationInfo:
			msg.UserLocationInfo, err = DecodeUserLocationInformationNR(ie.Value)
		}
		if err != nil {
			return nil, fmt.Errorf("ngap: UplinkNASTransport IE %d: %w", ie.ID, err)
		}
	}
	return msg, nil
}

// --- Initial Context Setup Response ------------------------------------------

// InitialContextSetupResponse is sent by the gNB to confirm UE context establishment.
// TS 38.413 §9.2.2.2
type InitialContextSetupResponse struct {
	AMFUENGAPID uint64
	RANUENGAPID uint64
}

// EncodeInitialContextSetupResponse encodes an InitialContextSetupResponse PDU.
func EncodeInitialContextSetupResponse(resp *InitialContextSetupResponse) ([]byte, error) {
	amfIDVal, err := EncodeAMFUENGAPID(resp.AMFUENGAPID)
	if err != nil {
		return nil, err
	}
	ranIDVal, err := EncodeRANUENGAPID(resp.RANUENGAPID)
	if err != nil {
		return nil, err
	}

	ies := []ProtocolIE{
		{ID: IEIDAMFUENGAPID, Criticality: CriticalityIgnore, Value: amfIDVal},
		{ID: IEIDRANUENGAPID, Criticality: CriticalityIgnore, Value: ranIDVal},
	}
	containerBytes, err := EncodeIEContainer(ies)
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

// DecodeInitialContextSetupResponse decodes the gNB's InitialContextSetupResponse IE list.
func DecodeInitialContextSetupResponse(ies []ProtocolIE) (*InitialContextSetupResponse, error) {
	resp := &InitialContextSetupResponse{}
	for _, ie := range ies {
		var err error
		switch ie.ID {
		case IEIDAMFUENGAPID:
			resp.AMFUENGAPID, err = DecodeAMFUENGAPID(ie.Value)
		case IEIDRANUENGAPID:
			resp.RANUENGAPID, err = DecodeRANUENGAPID(ie.Value)
		}
		if err != nil {
			return nil, fmt.Errorf("ngap: InitialContextSetupResponse IE %d: %w", ie.ID, err)
		}
	}
	return resp, nil
}

// DecodeInitialContextSetupRequest decodes an InitialContextSetupRequest from its IE list.
func DecodeInitialContextSetupRequest(ies []ProtocolIE) (*InitialContextSetupRequest, error) {
	req := &InitialContextSetupRequest{}
	for _, ie := range ies {
		var err error
		switch ie.ID {
		case IEIDAMFUENGAPID:
			req.AMFUENGAPID, err = DecodeAMFUENGAPID(ie.Value)
		case IEIDRANUENGAPID:
			req.RANUENGAPID, err = DecodeRANUENGAPID(ie.Value)
		case IEIDGUAMI:
			req.GUAMI, err = DecodeGUAMI(ie.Value)
		case IEIDUESecurityCapabilities:
			req.UESecurityCapabilities.NREncAlgs,
				req.UESecurityCapabilities.NRIntAlgs,
				req.UESecurityCapabilities.EUTRAEncAlgs,
				req.UESecurityCapabilities.EUTRAIntAlgs,
				err = DecodeUESecurityCapabilities(ie.Value)
		case IEIDSecurityKey:
			if len(ie.Value) >= 32 {
				copy(req.SecurityKey[:], ie.Value[:32])
			}
		case IEIDNASPDU:
			dec := NewPERDecoder(ie.Value)
			req.NASPDU, err = dec.GetOctetString()
		}
		if err != nil {
			return nil, fmt.Errorf("ngap: InitialContextSetupRequest IE %d: %w", ie.ID, err)
		}
	}
	return req, nil
}

// --- UE Context Release ------------------------------------------------------

// UEContextReleaseRequest is sent by the gNB to ask the AMF to release a UE context.
// TS 38.413 §9.2.2.4
type UEContextReleaseRequest struct {
	AMFUENGAPID uint64
	RANUENGAPID uint64
	CauseGroup  CauseGroup
	CauseValue  uint8
}

// EncodeUEContextReleaseRequest encodes a UEContextReleaseRequest PDU.
func EncodeUEContextReleaseRequest(req *UEContextReleaseRequest) ([]byte, error) {
	amfIDVal, err := EncodeAMFUENGAPID(req.AMFUENGAPID)
	if err != nil {
		return nil, err
	}
	ranIDVal, err := EncodeRANUENGAPID(req.RANUENGAPID)
	if err != nil {
		return nil, err
	}

	// UE-NGAP-IDs CHOICE: pair (index 0)
	pairEnc := NewPEREncoder()
	_ = pairEnc.PutChoiceIndex(0, 2, true)
	pairEnc.PutSequenceHeader(true, 0, 0)
	pairEnc.PutBytes(amfIDVal)
	pairEnc.PutBytes(ranIDVal)

	ies := []ProtocolIE{
		{ID: 114, Criticality: CriticalityReject, Value: pairEnc.Bytes()}, // id-UE-NGAP-IDs
		{ID: IEIDCause, Criticality: CriticalityIgnore, Value: EncodeCause(req.CauseGroup, req.CauseValue)},
	}
	containerBytes, err := EncodeIEContainer(ies)
	if err != nil {
		return nil, err
	}
	return EncodePDU(&PDU{
		Type:          PDUInitiatingMessage,
		ProcedureCode: ProcUEContextRelease,
		Criticality:   CriticalityIgnore,
		Value:         containerBytes,
	})
}

// UEContextReleaseComplete is sent by the gNB to confirm context release.
// TS 38.413 §9.2.2.6
type UEContextReleaseComplete struct {
	AMFUENGAPID uint64
	RANUENGAPID uint64
}

// EncodeUEContextReleaseComplete encodes a UEContextReleaseComplete PDU.
func EncodeUEContextReleaseComplete(msg *UEContextReleaseComplete) ([]byte, error) {
	amfIDVal, err := EncodeAMFUENGAPID(msg.AMFUENGAPID)
	if err != nil {
		return nil, err
	}
	ranIDVal, err := EncodeRANUENGAPID(msg.RANUENGAPID)
	if err != nil {
		return nil, err
	}

	ies := []ProtocolIE{
		{ID: IEIDAMFUENGAPID, Criticality: CriticalityIgnore, Value: amfIDVal},
		{ID: IEIDRANUENGAPID, Criticality: CriticalityIgnore, Value: ranIDVal},
	}
	containerBytes, err := EncodeIEContainer(ies)
	if err != nil {
		return nil, err
	}
	return EncodePDU(&PDU{
		Type:          PDUSuccessfulOutcome,
		ProcedureCode: ProcUEContextRelease,
		Criticality:   CriticalityReject,
		Value:         containerBytes,
	})
}
