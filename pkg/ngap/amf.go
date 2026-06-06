package ngap

import "fmt"

// This file contains AMF → gNB message encoders and gNB → AMF message decoders
// (the AMF's perspective on each procedure).

// --- NGSetup -----------------------------------------------------------------

// NGSetupResponse is sent by the AMF to accept a gNB's NG Setup Request.
// TS 38.413 §9.2.6.2
type NGSetupResponse struct {
	AMFName          string
	ServedGUAMIList  []ServedGUAMIItem
	RelativeCapacity uint8           // 0..255
	PLMNSupportList  []PLMNSupportItem
}

// EncodeNGSetupResponse encodes an NGSetupResponse into a complete NGAP PDU.
func EncodeNGSetupResponse(resp *NGSetupResponse) ([]byte, error) {
	var ies []ProtocolIE

	// AMFName (mandatory)
	nameEnc := NewPEREncoder()
	if err := nameEnc.PutPrintableString(resp.AMFName, 1, 150, true); err != nil {
		return nil, err
	}
	ies = append(ies, ProtocolIE{ID: IEIDAMFName, Criticality: CriticalityReject, Value: nameEnc.Bytes()})

	// ServedGUAMIList (mandatory)
	guamiListVal, err := EncodeServedGUAMIList(resp.ServedGUAMIList)
	if err != nil {
		return nil, fmt.Errorf("ngap: ServedGUAMIList: %w", err)
	}
	ies = append(ies, ProtocolIE{ID: IEIDServedGUAMIList, Criticality: CriticalityReject, Value: guamiListVal})

	// RelativeAMFCapacity (mandatory)
	capEnc := NewPEREncoder()
	if err := capEnc.PutConstrainedInt(int64(resp.RelativeCapacity), 0, 255); err != nil {
		return nil, err
	}
	ies = append(ies, ProtocolIE{ID: IEIDRelativeAMFCapacity, Criticality: CriticalityIgnore, Value: capEnc.Bytes()})

	// PLMNSupportList (mandatory)
	plmnVal, err := EncodePLMNSupportList(resp.PLMNSupportList)
	if err != nil {
		return nil, fmt.Errorf("ngap: PLMNSupportList: %w", err)
	}
	ies = append(ies, ProtocolIE{ID: IEIDPLMNSupportList, Criticality: CriticalityReject, Value: plmnVal})

	containerBytes, err := EncodeIEContainer(ies)
	if err != nil {
		return nil, err
	}
	return EncodePDU(&PDU{
		Type:          PDUSuccessfulOutcome,
		ProcedureCode: ProcNGSetup,
		Criticality:   CriticalityReject,
		Value:         containerBytes,
	})
}

// NGSetupFailure is sent by the AMF to reject a gNB's NG Setup Request.
type NGSetupFailure struct {
	CauseGroup CauseGroup
	CauseValue uint8
}

// EncodeNGSetupFailure encodes an NGSetupFailure into a complete NGAP PDU.
func EncodeNGSetupFailure(fail *NGSetupFailure) ([]byte, error) {
	ies := []ProtocolIE{
		{ID: IEIDCause, Criticality: CriticalityIgnore, Value: EncodeCause(fail.CauseGroup, fail.CauseValue)},
	}
	containerBytes, err := EncodeIEContainer(ies)
	if err != nil {
		return nil, err
	}
	return EncodePDU(&PDU{
		Type:          PDUUnsuccessfulOutcome,
		ProcedureCode: ProcNGSetup,
		Criticality:   CriticalityIgnore,
		Value:         containerBytes,
	})
}

// --- Downlink NAS Transport --------------------------------------------------

// DownlinkNASTransport carries a NAS PDU from AMF to gNB.
// TS 38.413 §9.2.5.2
type DownlinkNASTransport struct {
	AMFUENGAPID uint64
	RANUENGAPID uint64
	NASPDU      []byte
}

// EncodeDownlinkNASTransport encodes a DownlinkNASTransport into a complete NGAP PDU.
func EncodeDownlinkNASTransport(msg *DownlinkNASTransport) ([]byte, error) {
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
	}
	containerBytes, err := EncodeIEContainer(ies)
	if err != nil {
		return nil, err
	}
	return EncodePDU(&PDU{
		Type:          PDUInitiatingMessage,
		ProcedureCode: ProcDownlinkNASTransport,
		Criticality:   CriticalityIgnore,
		Value:         containerBytes,
	})
}

// DecodeDownlinkNASTransport decodes a DownlinkNASTransport from its IE list.
func DecodeDownlinkNASTransport(ies []ProtocolIE) (*DownlinkNASTransport, error) {
	msg := &DownlinkNASTransport{}
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
		}
		if err != nil {
			return nil, fmt.Errorf("ngap: DownlinkNASTransport IE %d: %w", ie.ID, err)
		}
	}
	return msg, nil
}

// --- Initial Context Setup ---------------------------------------------------

// InitialContextSetupRequest is sent by the AMF to establish a UE context at the gNB.
// For basic registration (no PDU session), only the mandatory security IEs are needed.
// TS 38.413 §9.2.2.1
type InitialContextSetupRequest struct {
	AMFUENGAPID          uint64
	RANUENGAPID          uint64
	GUAMI                GUAMI
	AllowedNSSAI         []SNSSAI
	UESecurityCapabilities struct {
		NREncAlgs   [2]byte
		NRIntAlgs   [2]byte
		EUTRAEncAlgs [2]byte
		EUTRAIntAlgs [2]byte
	}
	SecurityKey [32]byte // KgNB (256 bits)
	NASPDU      []byte   // optional: piggybacked NAS message (e.g. Registration Accept)
}

// EncodeInitialContextSetupRequest encodes an InitialContextSetupRequest PDU.
func EncodeInitialContextSetupRequest(req *InitialContextSetupRequest) ([]byte, error) {
	amfIDVal, err := EncodeAMFUENGAPID(req.AMFUENGAPID)
	if err != nil {
		return nil, err
	}
	ranIDVal, err := EncodeRANUENGAPID(req.RANUENGAPID)
	if err != nil {
		return nil, err
	}

	var ies []ProtocolIE

	ies = append(ies, ProtocolIE{ID: IEIDAMFUENGAPID, Criticality: CriticalityReject, Value: amfIDVal})
	ies = append(ies, ProtocolIE{ID: IEIDRANUENGAPID, Criticality: CriticalityReject, Value: ranIDVal})

	// GUAMI (mandatory)
	ies = append(ies, ProtocolIE{ID: IEIDGUAMI, Criticality: CriticalityReject, Value: EncodeGUAMI(req.GUAMI)})

	// AllowedNSSAI (mandatory for 5G registration)
	if len(req.AllowedNSSAI) > 0 {
		nssaiVal, err := encodeAllowedNSSAI(req.AllowedNSSAI)
		if err != nil {
			return nil, fmt.Errorf("ngap: AllowedNSSAI: %w", err)
		}
		ies = append(ies, ProtocolIE{ID: IEIDAllowedNSSAI, Criticality: CriticalityReject, Value: nssaiVal})
	}

	// UESecurityCapabilities (mandatory)
	secCapVal := EncodeUESecurityCapabilities(
		req.UESecurityCapabilities.NREncAlgs,
		req.UESecurityCapabilities.NRIntAlgs,
		req.UESecurityCapabilities.EUTRAEncAlgs,
		req.UESecurityCapabilities.EUTRAIntAlgs,
	)
	ies = append(ies, ProtocolIE{ID: IEIDUESecurityCapabilities, Criticality: CriticalityReject, Value: secCapVal})

	// SecurityKey: BIT STRING(256) = 32 bytes
	keyEnc := NewPEREncoder()
	keyEnc.PutBitString(req.SecurityKey[:], 256)
	ies = append(ies, ProtocolIE{ID: IEIDSecurityKey, Criticality: CriticalityReject, Value: keyEnc.Bytes()})

	// NAS-PDU (optional)
	if len(req.NASPDU) > 0 {
		nasEnc := NewPEREncoder()
		if err := nasEnc.PutOctetString(req.NASPDU); err != nil {
			return nil, err
		}
		ies = append(ies, ProtocolIE{ID: IEIDNASPDU, Criticality: CriticalityIgnore, Value: nasEnc.Bytes()})
	}

	containerBytes, err := EncodeIEContainer(ies)
	if err != nil {
		return nil, err
	}
	return EncodePDU(&PDU{
		Type:          PDUInitiatingMessage,
		ProcedureCode: ProcInitialContextSetup,
		Criticality:   CriticalityReject,
		Value:         containerBytes,
	})
}

// encodeAllowedNSSAI encodes the AllowedNSSAI IE value.
// AllowedNSSAI ::= SEQUENCE(SIZE(1..8)) OF AllowedNSSAI-Item
func encodeAllowedNSSAI(nssais []SNSSAI) ([]byte, error) {
	enc := NewPEREncoder()
	if err := enc.PutConstrainedInt(int64(len(nssais)), 1, 8); err != nil {
		return nil, err
	}
	for _, nssai := range nssais {
		sdPresent := len(nssai.SD) == 3
		// AllowedNSSAI-Item SEQUENCE: extensible, 1 optional (iE-Ext), absent
		enc.PutSequenceHeader(true, 0, 1)
		// S-NSSAI inline
		var optBits uint64
		if sdPresent {
			optBits = 2
		}
		enc.PutSequenceHeader(true, optBits, 2)
		enc.PutFixedOctetString([]byte{nssai.SST})
		if sdPresent {
			enc.PutFixedOctetString(nssai.SD[:3])
		}
	}
	return enc.Bytes(), nil
}

// --- UE Context Release (AMF-initiated) -------------------------------------

// UEContextReleaseCommand is sent by the AMF to release a UE context.
// TS 38.413 §9.2.2.5
type UEContextReleaseCommand struct {
	AMFUENGAPID uint64
	RANUENGAPID uint64
	CauseGroup  CauseGroup
	CauseValue  uint8
}

// EncodeUEContextReleaseCommand encodes a UEContextReleaseCommand PDU.
func EncodeUEContextReleaseCommand(cmd *UEContextReleaseCommand) ([]byte, error) {
	// UE-NGAP-IDs: CHOICE { uE-NGAP-ID-pair(0), aMF-UE-NGAP-ID(1), ... }
	pairEnc := NewPEREncoder()
	_ = pairEnc.PutChoiceIndex(0, 2, true) // pair
	amfIDVal, err := EncodeAMFUENGAPID(cmd.AMFUENGAPID)
	if err != nil {
		return nil, err
	}
	ranIDVal, err := EncodeRANUENGAPID(cmd.RANUENGAPID)
	if err != nil {
		return nil, err
	}
	// UE-NGAP-ID-pair SEQUENCE: extensible, no optionals
	pairEnc.PutSequenceHeader(true, 0, 0)
	pairEnc.PutBytes(amfIDVal)
	pairEnc.PutBytes(ranIDVal)

	ies := []ProtocolIE{
		{ID: 114, Criticality: CriticalityReject, Value: pairEnc.Bytes()}, // id-UE-NGAP-IDs = 114
		{ID: IEIDCause, Criticality: CriticalityIgnore, Value: EncodeCause(cmd.CauseGroup, cmd.CauseValue)},
	}
	containerBytes, err := EncodeIEContainer(ies)
	if err != nil {
		return nil, err
	}
	return EncodePDU(&PDU{
		Type:          PDUInitiatingMessage,
		ProcedureCode: ProcUEContextRelease,
		Criticality:   CriticalityReject,
		Value:         containerBytes,
	})
}
