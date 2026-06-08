package ngap

import (
	"encoding/binary"
	"fmt"
	"net"
)

// PDUSessionResourceSetupRequest is the AMF-initiated N2 request that asks the
// gNB to create the N3 tunnel for a PDU session. This implements the narrow
// single-session shape required by UERANSIM T10.
type PDUSessionResourceSetupRequest struct {
	AMFUENGAPID uint64
	RANUENGAPID uint64
	NASPDU      []byte

	PDUSessionID uint8
	SNSSAI       SNSSAI
	UPFTEID      uint32
	UPFIP        net.IP
	QFI          uint8
	FiveQI       uint8
}

type PDUSessionResourceSetupResponse struct {
	AMFUENGAPID uint64
	RANUENGAPID uint64
	Sessions    []PDUSessionResourceSetupResponseItem
}

type PDUSessionResourceSetupResponseItem struct {
	PDUSessionID uint8
	GNBTEID      uint32
	GNBIP        net.IP
	QFI          uint8
}

func EncodePDUSessionResourceSetupRequest(req *PDUSessionResourceSetupRequest) ([]byte, error) {
	amfIDVal, err := EncodeAMFUENGAPID(req.AMFUENGAPID)
	if err != nil {
		return nil, err
	}
	ranIDVal, err := EncodeRANUENGAPID(req.RANUENGAPID)
	if err != nil {
		return nil, err
	}
	setupList, err := encodePDUSessionResourceSetupListSUReq(req)
	if err != nil {
		return nil, err
	}

	ies := []ProtocolIE{
		{ID: IEIDAMFUENGAPID, Criticality: CriticalityReject, Value: amfIDVal},
		{ID: IEIDRANUENGAPID, Criticality: CriticalityReject, Value: ranIDVal},
		{ID: IEIDPDUSessionSetupListSUReq, Criticality: CriticalityReject, Value: setupList},
	}
	containerBytes, err := EncodeIEContainer(ies)
	if err != nil {
		return nil, err
	}
	return EncodePDU(&PDU{
		Type:          PDUInitiatingMessage,
		ProcedureCode: ProcPDUSessionResourceSetup,
		Criticality:   CriticalityReject,
		Value:         containerBytes,
	})
}

func DecodePDUSessionResourceSetupRequest(ies []ProtocolIE) (*PDUSessionResourceSetupRequest, error) {
	req := &PDUSessionResourceSetupRequest{}
	for _, ie := range ies {
		var err error
		switch ie.ID {
		case IEIDAMFUENGAPID:
			req.AMFUENGAPID, err = DecodeAMFUENGAPID(ie.Value)
		case IEIDRANUENGAPID:
			req.RANUENGAPID, err = DecodeRANUENGAPID(ie.Value)
		case IEIDPDUSessionSetupListSUReq:
			var item *PDUSessionResourceSetupRequest
			item, err = decodePDUSessionResourceSetupListSUReq(ie.Value)
			if err == nil && item != nil {
				req.NASPDU = item.NASPDU
				req.PDUSessionID = item.PDUSessionID
				req.SNSSAI = item.SNSSAI
				req.UPFTEID = item.UPFTEID
				req.UPFIP = item.UPFIP
				req.QFI = item.QFI
				req.FiveQI = item.FiveQI
			}
		}
		if err != nil {
			return nil, fmt.Errorf("ngap: PDUSessionResourceSetupRequest IE %d: %w", ie.ID, err)
		}
	}
	return req, nil
}

func DecodePDUSessionResourceSetupResponse(ies []ProtocolIE) (*PDUSessionResourceSetupResponse, error) {
	resp := &PDUSessionResourceSetupResponse{}
	for _, ie := range ies {
		var err error
		switch ie.ID {
		case IEIDAMFUENGAPID:
			resp.AMFUENGAPID, err = DecodeAMFUENGAPID(ie.Value)
		case IEIDRANUENGAPID:
			resp.RANUENGAPID, err = DecodeRANUENGAPID(ie.Value)
		case IEIDPDUSessionSetupListSURes:
			resp.Sessions, err = decodePDUSessionResourceSetupListSURes(ie.Value)
		}
		if err != nil {
			return nil, fmt.Errorf("ngap: PDUSessionResourceSetupResponse IE %d: %w", ie.ID, err)
		}
	}
	return resp, nil
}

func encodePDUSessionResourceSetupListSUReq(req *PDUSessionResourceSetupRequest) ([]byte, error) {
	enc := NewPEREncoder()
	if err := enc.PutConstrainedInt(1, 1, 256); err != nil {
		return nil, err
	}

	nasPresent := len(req.NASPDU) > 0
	var optBits uint64
	if nasPresent {
		optBits = 2 // pDUSessionNAS-PDU present, iE-Extensions absent
	}
	enc.PutSequenceHeader(true, optBits, 2)
	if err := enc.PutConstrainedInt(int64(req.PDUSessionID), 0, 255); err != nil {
		return nil, err
	}
	if nasPresent {
		if err := enc.PutOctetString(req.NASPDU); err != nil {
			return nil, err
		}
	}
	writeSNSSAI(enc, req.SNSSAI)

	transfer, err := encodePDUSessionResourceSetupRequestTransfer(req)
	if err != nil {
		return nil, err
	}
	if err := enc.PutOctetString(transfer); err != nil {
		return nil, err
	}
	return enc.Bytes(), nil
}

func decodePDUSessionResourceSetupListSUReq(data []byte) (*PDUSessionResourceSetupRequest, error) {
	dec := NewPERDecoder(data)
	count, err := dec.GetConstrainedInt(1, 256)
	if err != nil {
		return nil, err
	}
	if count < 1 {
		return nil, fmt.Errorf("empty setup list")
	}
	_, optBits, err := dec.GetSequenceHeader(true, 2)
	if err != nil {
		return nil, err
	}
	id, err := dec.GetConstrainedInt(0, 255)
	if err != nil {
		return nil, err
	}
	req := &PDUSessionResourceSetupRequest{PDUSessionID: uint8(id)}
	if optBits&2 != 0 {
		req.NASPDU, err = dec.GetOctetString()
		if err != nil {
			return nil, err
		}
	}
	req.SNSSAI, err = readSNSSAI(dec)
	if err != nil {
		return nil, err
	}
	transfer, err := dec.GetOctetString()
	if err != nil {
		return nil, err
	}
	req.UPFTEID, req.UPFIP, req.QFI, req.FiveQI, err = decodePDUSessionResourceSetupRequestTransfer(transfer)
	return req, err
}

func encodePDUSessionResourceSetupRequestTransfer(req *PDUSessionResourceSetupRequest) ([]byte, error) {
	ies := make([]ProtocolIE, 0, 4)
	ies = append(ies, ProtocolIE{
		ID:          IEIDULNGUUPTNLInformation,
		Criticality: CriticalityReject,
		Value:       encodeUPTransportLayerInformation(req.UPFIP, req.UPFTEID),
	})
	ies = append(ies, ProtocolIE{
		ID:          IEIDPDUSessionType,
		Criticality: CriticalityReject,
		Value:       encodePDUSessionTypeIPv4(),
	})
	qfi := req.QFI
	if qfi == 0 {
		qfi = 1
	}
	fiveQI := req.FiveQI
	if fiveQI == 0 {
		fiveQI = 9
	}
	ies = append(ies, ProtocolIE{
		ID:          IEIDQosFlowSetupRequestList,
		Criticality: CriticalityReject,
		Value:       encodeQosFlowSetupRequestList(qfi, fiveQI),
	})
	return EncodeIEContainer(ies)
}

func decodePDUSessionResourceSetupRequestTransfer(data []byte) (uint32, net.IP, uint8, uint8, error) {
	ies, err := DecodeIEContainer(data)
	if err != nil {
		return 0, nil, 0, 0, err
	}
	var teid uint32
	var ip net.IP
	var qfi, fiveQI uint8
	for _, ie := range ies {
		switch ie.ID {
		case IEIDULNGUUPTNLInformation:
			teid, ip, err = decodeUPTransportLayerInformation(ie.Value)
		case IEIDQosFlowSetupRequestList:
			qfi, fiveQI, err = decodeQosFlowSetupRequestList(ie.Value)
		}
		if err != nil {
			return 0, nil, 0, 0, err
		}
	}
	return teid, ip, qfi, fiveQI, nil
}

func decodePDUSessionResourceSetupListSURes(data []byte) ([]PDUSessionResourceSetupResponseItem, error) {
	dec := NewPERDecoder(data)
	count, err := dec.GetConstrainedInt(1, 256)
	if err != nil {
		return nil, err
	}
	out := make([]PDUSessionResourceSetupResponseItem, 0, count)
	for i := 0; i < int(count); i++ {
		_, _, err := dec.GetSequenceHeader(true, 1)
		if err != nil {
			return nil, err
		}
		id, err := dec.GetConstrainedInt(0, 255)
		if err != nil {
			return nil, err
		}
		transfer, err := dec.GetOctetString()
		if err != nil {
			return nil, err
		}
		item, err := decodePDUSessionResourceSetupResponseTransfer(transfer)
		if err != nil {
			return nil, err
		}
		item.PDUSessionID = uint8(id)
		out = append(out, item)
	}
	return out, nil
}

func decodePDUSessionResourceSetupResponseTransfer(data []byte) (PDUSessionResourceSetupResponseItem, error) {
	dec := NewPERDecoder(data)
	var item PDUSessionResourceSetupResponseItem
	if _, _, err := dec.GetSequenceHeader(true, 4); err != nil {
		return item, err
	}
	teid, ip, qfi, err := decodeQosFlowPerTNLInformation(dec)
	if err != nil {
		return item, err
	}
	item.GNBTEID = teid
	item.GNBIP = ip
	item.QFI = qfi
	return item, nil
}

func encodeUPTransportLayerInformation(ip net.IP, teid uint32) []byte {
	enc := NewPEREncoder()
	_ = enc.PutChoiceIndex(0, 1, true) // gTPTunnel, not an extension
	writeGTPTunnel(enc, ip, teid)
	return enc.Bytes()
}

func decodeUPTransportLayerInformation(data []byte) (uint32, net.IP, error) {
	dec := NewPERDecoder(data)
	idx, err := dec.GetChoiceIndex(1, true)
	if err != nil {
		return 0, nil, err
	}
	if idx != 0 {
		return 0, nil, fmt.Errorf("unsupported UPTransportLayerInformation choice %d", idx)
	}
	return readGTPTunnel(dec)
}

func writeGTPTunnel(enc *PEREncoder, ip net.IP, teid uint32) {
	enc.PutSequenceHeader(true, 0, 1)
	writeTransportLayerAddress(enc, ip)
	b := make([]byte, 4)
	binary.BigEndian.PutUint32(b, teid)
	enc.PutFixedOctetString(b)
}

func readGTPTunnel(dec *PERDecoder) (uint32, net.IP, error) {
	if _, _, err := dec.GetSequenceHeader(true, 1); err != nil {
		return 0, nil, err
	}
	ip, err := readTransportLayerAddress(dec)
	if err != nil {
		return 0, nil, err
	}
	teidBytes, err := dec.GetFixedOctetString(4)
	if err != nil {
		return 0, nil, err
	}
	return binary.BigEndian.Uint32(teidBytes), ip, nil
}

func writeTransportLayerAddress(enc *PEREncoder, ip net.IP) {
	v4 := ip.To4()
	if v4 == nil {
		v4 = net.IPv4(127, 0, 0, 1).To4()
	}
	enc.PutBool(false) // not extended
	_ = enc.PutConstrainedInt(32, 1, 160)
	enc.PutBitString(v4, 32)
}

func readTransportLayerAddress(dec *PERDecoder) (net.IP, error) {
	extended, err := dec.GetBool()
	if err != nil {
		return nil, err
	}
	if extended {
		return nil, fmt.Errorf("extended TransportLayerAddress not supported")
	}
	nBits, err := dec.GetConstrainedInt(1, 160)
	if err != nil {
		return nil, err
	}
	if nBits != 32 {
		return nil, fmt.Errorf("unsupported TransportLayerAddress length %d", nBits)
	}
	dec.align()
	b, err := dec.GetBytes(4)
	if err != nil {
		return nil, err
	}
	return net.IPv4(b[0], b[1], b[2], b[3]), nil
}

func encodePDUSessionTypeIPv4() []byte {
	enc := NewPEREncoder()
	enc.PutBool(false)
	_ = enc.PutConstrainedInt(0, 0, 4)
	return enc.Bytes()
}

func encodeQosFlowSetupRequestList(qfi, fiveQI uint8) []byte {
	enc := NewPEREncoder()
	_ = enc.PutConstrainedInt(1, 1, 64)
	enc.PutSequenceHeader(true, 0, 2)
	writeQosFlowIdentifier(enc, qfi)
	writeQosFlowLevelQosParameters(enc, fiveQI)
	return enc.Bytes()
}

func decodeQosFlowSetupRequestList(data []byte) (uint8, uint8, error) {
	dec := NewPERDecoder(data)
	count, err := dec.GetConstrainedInt(1, 64)
	if err != nil {
		return 0, 0, err
	}
	if count < 1 {
		return 0, 0, fmt.Errorf("empty QoS flow setup list")
	}
	if _, _, err := dec.GetSequenceHeader(true, 2); err != nil {
		return 0, 0, err
	}
	qfi, err := readQosFlowIdentifier(dec)
	if err != nil {
		return 0, 0, err
	}
	fiveQI, err := readQosFlowLevelQosParameters(dec)
	return qfi, fiveQI, err
}

func writeQosFlowLevelQosParameters(enc *PEREncoder, fiveQI uint8) {
	enc.PutSequenceHeader(true, 0, 4)
	_ = enc.PutChoiceIndex(0, 3, false) // nonDynamic5QI
	enc.PutSequenceHeader(true, 0, 4)
	enc.PutBool(false)
	_ = enc.PutConstrainedInt(int64(fiveQI), 0, 255)
	enc.PutSequenceHeader(true, 0, 1)
	_ = enc.PutConstrainedInt(9, 1, 15)
	enc.PutBool(false)
	_ = enc.PutConstrainedInt(0, 0, 1) // shall-not-trigger-pre-emption
	enc.PutBool(false)
	_ = enc.PutConstrainedInt(1, 0, 1) // pre-emptable
}

func readQosFlowLevelQosParameters(dec *PERDecoder) (uint8, error) {
	if _, _, err := dec.GetSequenceHeader(true, 4); err != nil {
		return 0, err
	}
	idx, err := dec.GetChoiceIndex(3, false)
	if err != nil {
		return 0, err
	}
	if idx != 0 {
		return 0, fmt.Errorf("unsupported QoS characteristics choice %d", idx)
	}
	if _, _, err := dec.GetSequenceHeader(true, 4); err != nil {
		return 0, err
	}
	extended, err := dec.GetBool()
	if err != nil {
		return 0, err
	}
	if extended {
		return 0, fmt.Errorf("extended FiveQI not supported")
	}
	v, err := dec.GetConstrainedInt(0, 255)
	if err != nil {
		return 0, err
	}
	return uint8(v), nil
}

func decodeQosFlowPerTNLInformation(dec *PERDecoder) (uint32, net.IP, uint8, error) {
	if _, _, err := dec.GetSequenceHeader(true, 1); err != nil {
		return 0, nil, 0, err
	}
	teid, ip, err := readUPTransportLayerInformation(dec)
	if err != nil {
		return 0, nil, 0, err
	}
	qfi, err := readAssociatedQosFlowList(dec)
	return teid, ip, qfi, err
}

func readUPTransportLayerInformation(dec *PERDecoder) (uint32, net.IP, error) {
	idx, err := dec.GetChoiceIndex(1, true)
	if err != nil {
		return 0, nil, err
	}
	if idx != 0 {
		return 0, nil, fmt.Errorf("unsupported UPTransportLayerInformation choice %d", idx)
	}
	return readGTPTunnel(dec)
}

func readAssociatedQosFlowList(dec *PERDecoder) (uint8, error) {
	count, err := dec.GetConstrainedInt(1, 64)
	if err != nil {
		return 0, err
	}
	var first uint8
	for i := 0; i < int(count); i++ {
		if _, _, err := dec.GetSequenceHeader(true, 1); err != nil {
			return 0, err
		}
		qfi, err := readQosFlowIdentifier(dec)
		if err != nil {
			return 0, err
		}
		if i == 0 {
			first = qfi
		}
	}
	return first, nil
}

func writeSNSSAI(enc *PEREncoder, s SNSSAI) {
	sdPresent := len(s.SD) == 3
	var optBits uint64
	if sdPresent {
		optBits = 2
	}
	enc.PutSequenceHeader(true, optBits, 2)
	enc.PutFixedOctetString([]byte{s.SST})
	if sdPresent {
		enc.PutFixedOctetString(s.SD[:3])
	}
}

func readSNSSAI(dec *PERDecoder) (SNSSAI, error) {
	var s SNSSAI
	_, optBits, err := dec.GetSequenceHeader(true, 2)
	if err != nil {
		return s, err
	}
	sst, err := dec.GetFixedOctetString(1)
	if err != nil {
		return s, err
	}
	s.SST = sst[0]
	if optBits&2 != 0 {
		s.SD, err = dec.GetFixedOctetString(3)
	}
	return s, err
}

func writeQosFlowIdentifier(enc *PEREncoder, qfi uint8) {
	enc.PutBool(false)
	_ = enc.PutConstrainedInt(int64(qfi), 0, 63)
}

func readQosFlowIdentifier(dec *PERDecoder) (uint8, error) {
	extended, err := dec.GetBool()
	if err != nil {
		return 0, err
	}
	if extended {
		return 0, fmt.Errorf("extended QFI not supported")
	}
	v, err := dec.GetConstrainedInt(0, 63)
	return uint8(v), err
}
