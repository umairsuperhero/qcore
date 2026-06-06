package ngap

import (
	"bytes"
	"encoding/hex"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// plmn001_01 is PLMN for MCC=001, MNC=01 (test network).
var plmn001_01 = PLMNFromMCCMNC("001", "01")

// testTAI is a representative 5G TAI.
var testTAI = TAI{
	PLMN: plmn001_01,
	TAC:  [3]byte{0x00, 0x00, 0x01},
}

// testNRCGI is a representative NR-CGI.
var testNRCGI = NRCGI{
	PLMN:     plmn001_01,
	NRCellID: 0x0000001, // 36-bit cell ID
}

// testGUAMI is a representative GUAMI.
var testGUAMI = GUAMI{
	PLMN:        plmn001_01,
	AMFRegionID: 0x01,
	AMFSetID:    0x001,
	AMFPointer:  0x00,
}

// testGNBID is a representative gNB ID.
var testGNBID = GlobalGNBID{
	PLMN:      plmn001_01,
	GNBID:     0x00000001,
	GNBIDBits: 32,
}

func TestPLMNEncoding(t *testing.T) {
	plmn := PLMNFromMCCMNC("001", "01")
	// TS 24.008 §10.5.1.13 canonical layout (verified against UERANSIM config):
	// MCC=001 MNC=01 (2-digit → MNC3=0xF filler):
	//   byte0: MCC2|MCC1 = (0<<4)|0 = 0x00
	//   byte1: MNC3|MCC3 = (F<<4)|1 = 0xF1
	//   byte2: MNC2|MNC1 = (1<<4)|0 = 0x10
	assert.Equal(t, PLMN{0x00, 0xF1, 0x10}, plmn)
}

func TestTAIRoundTrip(t *testing.T) {
	encoded := EncodeTAI(testTAI)
	decoded, err := DecodeTAI(encoded)
	require.NoError(t, err)
	assert.Equal(t, testTAI, decoded)
}

func TestNRCGIRoundTrip(t *testing.T) {
	encoded := EncodeNRCGI(testNRCGI)
	decoded, err := DecodeNRCGI(encoded)
	require.NoError(t, err)
	assert.Equal(t, testNRCGI, decoded)
}

func TestGUAMIRoundTrip(t *testing.T) {
	encoded := EncodeGUAMI(testGUAMI)
	decoded, err := DecodeGUAMI(encoded)
	require.NoError(t, err)
	assert.Equal(t, testGUAMI, decoded)
}

func TestGUAMI_AMFIDFields(t *testing.T) {
	g := GUAMI{
		PLMN:        plmn001_01,
		AMFRegionID: 0xAB,
		AMFSetID:    0x3FF, // max 10-bit
		AMFPointer:  0x3F,  // max 6-bit
	}
	encoded := EncodeGUAMI(g)
	decoded, err := DecodeGUAMI(encoded)
	require.NoError(t, err)
	assert.Equal(t, g.AMFRegionID, decoded.AMFRegionID)
	assert.Equal(t, g.AMFSetID, decoded.AMFSetID)
	assert.Equal(t, g.AMFPointer, decoded.AMFPointer)
}

func TestGlobalGNBIDRoundTrip(t *testing.T) {
	encoded, err := EncodeGlobalGNBID(testGNBID)
	require.NoError(t, err)
	decoded, err := DecodeGlobalGNBID(encoded)
	require.NoError(t, err)
	assert.Equal(t, testGNBID.GNBID, decoded.GNBID)
	assert.Equal(t, testGNBID.GNBIDBits, decoded.GNBIDBits)
	assert.Equal(t, testGNBID.PLMN, decoded.PLMN)
}

func TestUserLocationInformationNRRoundTrip(t *testing.T) {
	loc := UserLocationInformationNR{
		NRCGI: testNRCGI,
		TAI:   testTAI,
	}
	encoded := EncodeUserLocationInformationNR(loc)
	decoded, err := DecodeUserLocationInformationNR(encoded)
	require.NoError(t, err)
	assert.Equal(t, loc.NRCGI, decoded.NRCGI)
	assert.Equal(t, loc.TAI, decoded.TAI)
}

func TestSupportedTAListRoundTrip(t *testing.T) {
	tas := []SupportedTA{
		{
			TAC: [3]byte{0x00, 0x00, 0x01},
			BroadcastPLMN: []BroadcastPLMNItem{
				{
					PLMN:   plmn001_01,
					SNSSAI: []SNSSAI{{SST: 1}},
				},
			},
		},
	}
	encoded, err := EncodeSupportedTAList(tas)
	require.NoError(t, err)
	decoded, err := DecodeSupportedTAList(encoded)
	require.NoError(t, err)
	require.Len(t, decoded, 1)
	assert.Equal(t, tas[0].TAC, decoded[0].TAC)
	require.Len(t, decoded[0].BroadcastPLMN, 1)
	assert.Equal(t, tas[0].BroadcastPLMN[0].PLMN, decoded[0].BroadcastPLMN[0].PLMN)
	require.Len(t, decoded[0].BroadcastPLMN[0].SNSSAI, 1)
	assert.Equal(t, uint8(1), decoded[0].BroadcastPLMN[0].SNSSAI[0].SST)
}

func TestSupportedTAList_WithSD(t *testing.T) {
	tas := []SupportedTA{
		{
			TAC: [3]byte{0x00, 0x00, 0x02},
			BroadcastPLMN: []BroadcastPLMNItem{
				{
					PLMN:   plmn001_01,
					SNSSAI: []SNSSAI{{SST: 2, SD: []byte{0x01, 0x02, 0x03}}},
				},
			},
		},
	}
	encoded, err := EncodeSupportedTAList(tas)
	require.NoError(t, err)
	decoded, err := DecodeSupportedTAList(encoded)
	require.NoError(t, err)
	nssai := decoded[0].BroadcastPLMN[0].SNSSAI[0]
	assert.Equal(t, uint8(2), nssai.SST)
	assert.Equal(t, []byte{0x01, 0x02, 0x03}, nssai.SD)
}

func TestServedGUAMIListRoundTrip(t *testing.T) {
	items := []ServedGUAMIItem{{GUAMI: testGUAMI}}
	encoded, err := EncodeServedGUAMIList(items)
	require.NoError(t, err)
	decoded, err := DecodeServedGUAMIList(encoded)
	require.NoError(t, err)
	require.Len(t, decoded, 1)
	assert.Equal(t, testGUAMI, decoded[0].GUAMI)
}

func TestCauseRoundTrip(t *testing.T) {
	encoded := EncodeCause(CauseNAS, 0x01)
	group, value, err := DecodeCause(encoded)
	require.NoError(t, err)
	assert.Equal(t, CauseNAS, group)
	assert.Equal(t, uint8(0x01), value)
}

func TestUESecurityCapabilitiesRoundTrip(t *testing.T) {
	nrEnc := [2]byte{0xE0, 0x00} // NIA1+NIA2+NIA3 supported
	nrInt := [2]byte{0xE0, 0x00}
	eutraEnc := [2]byte{0xF0, 0x00}
	eutraInt := [2]byte{0xF0, 0x00}
	encoded := EncodeUESecurityCapabilities(nrEnc, nrInt, eutraEnc, eutraInt)
	gotNREnc, gotNRInt, gotEUTRAEnc, gotEUTRAInt, err := DecodeUESecurityCapabilities(encoded)
	require.NoError(t, err)
	assert.Equal(t, nrEnc, gotNREnc)
	assert.Equal(t, nrInt, gotNRInt)
	assert.Equal(t, eutraEnc, gotEUTRAEnc)
	assert.Equal(t, eutraInt, gotEUTRAInt)
}

func TestAMFUENGAPIDRoundTrip(t *testing.T) {
	for _, id := range []uint64{0, 1, 255, 256, 65535, 65536, 1<<32 - 1, 1099511627775} {
		encoded, err := EncodeAMFUENGAPID(id)
		require.NoError(t, err)
		decoded, err := DecodeAMFUENGAPID(encoded)
		require.NoError(t, err)
		assert.Equal(t, id, decoded, "id=%d", id)
	}
}

func TestRANUENGAPIDRoundTrip(t *testing.T) {
	for _, id := range []uint64{0, 1, 0xFFFF, 0xFFFFFFFF} {
		encoded, err := EncodeRANUENGAPID(id)
		require.NoError(t, err)
		decoded, err := DecodeRANUENGAPID(encoded)
		require.NoError(t, err)
		assert.Equal(t, id, decoded, "id=%d", id)
	}
}

// --- Full PDU round-trips ---------------------------------------------------

func TestNGSetupRequestRoundTrip(t *testing.T) {
	req := &NGSetupRequest{
		GlobalRANNodeID: testGNBID,
		RANNodeName:     "qcore-gnb-01",
		SupportedTAList: []SupportedTA{
			{
				TAC: [3]byte{0x00, 0x00, 0x01},
				BroadcastPLMN: []BroadcastPLMNItem{
					{PLMN: plmn001_01, SNSSAI: []SNSSAI{{SST: 1}}},
				},
			},
		},
		DefaultPagingDRX: 2, // v128
	}

	rawPDU, err := EncodeNGSetupRequest(req)
	require.NoError(t, err)

	pdu, err := DecodePDU(rawPDU)
	require.NoError(t, err)
	assert.Equal(t, PDUInitiatingMessage, pdu.Type)
	assert.Equal(t, ProcNGSetup, pdu.ProcedureCode)

	ies, err := DecodeIEContainer(pdu.Value)
	require.NoError(t, err)

	decoded, err := DecodeNGSetupRequest(ies)
	require.NoError(t, err)

	assert.Equal(t, req.GlobalRANNodeID.GNBID, decoded.GlobalRANNodeID.GNBID)
	assert.Equal(t, req.RANNodeName, decoded.RANNodeName)
	require.Len(t, decoded.SupportedTAList, 1)
	assert.Equal(t, req.SupportedTAList[0].TAC, decoded.SupportedTAList[0].TAC)
	assert.Equal(t, req.DefaultPagingDRX, decoded.DefaultPagingDRX)
}

func TestNGSetupResponseRoundTrip(t *testing.T) {
	resp := &NGSetupResponse{
		AMFName:          "QCore-AMF",
		ServedGUAMIList:  []ServedGUAMIItem{{GUAMI: testGUAMI}},
		RelativeCapacity: 100,
		PLMNSupportList: []PLMNSupportItem{
			{PLMN: plmn001_01, SNSSAIs: []SNSSAI{{SST: 1}}},
		},
	}

	rawPDU, err := EncodeNGSetupResponse(resp)
	require.NoError(t, err)

	pdu, err := DecodePDU(rawPDU)
	require.NoError(t, err)
	assert.Equal(t, PDUSuccessfulOutcome, pdu.Type)
	assert.Equal(t, ProcNGSetup, pdu.ProcedureCode)

	ies, err := DecodeIEContainer(pdu.Value)
	require.NoError(t, err)

	decoded, err := DecodeNGSetupResponse(ies)
	require.NoError(t, err)
	assert.Equal(t, resp.AMFName, decoded.AMFName)
	require.Len(t, decoded.ServedGUAMIList, 1)
	assert.Equal(t, testGUAMI, decoded.ServedGUAMIList[0].GUAMI)
	assert.Equal(t, resp.RelativeCapacity, decoded.RelativeCapacity)
}

func TestInitialUEMessageRoundTrip(t *testing.T) {
	nasPDU := []byte{0x7E, 0x00, 0x41, 0x01, 0x02, 0x03} // fake NAS Registration Request

	msg := &InitialUEMessage{
		RANUENGAPID: 42,
		NASPDU:      nasPDU,
		UserLocationInfo: UserLocationInformationNR{
			NRCGI: testNRCGI,
			TAI:   testTAI,
		},
		RRCEstablishmentCause: RRCMoSignalling,
	}

	rawPDU, err := EncodeInitialUEMessage(msg)
	require.NoError(t, err)

	pdu, err := DecodePDU(rawPDU)
	require.NoError(t, err)
	assert.Equal(t, PDUInitiatingMessage, pdu.Type)
	assert.Equal(t, ProcInitialUEMessage, pdu.ProcedureCode)

	ies, err := DecodeIEContainer(pdu.Value)
	require.NoError(t, err)

	decoded, err := DecodeInitialUEMessage(ies)
	require.NoError(t, err)
	assert.Equal(t, msg.RANUENGAPID, decoded.RANUENGAPID)
	assert.True(t, bytes.Equal(nasPDU, decoded.NASPDU))
	assert.Equal(t, msg.UserLocationInfo.NRCGI, decoded.UserLocationInfo.NRCGI)
	assert.Equal(t, msg.UserLocationInfo.TAI, decoded.UserLocationInfo.TAI)
	assert.Equal(t, msg.RRCEstablishmentCause, decoded.RRCEstablishmentCause)
}

func TestDownlinkNASTransportRoundTrip(t *testing.T) {
	nasPDU := []byte{0x7E, 0x00, 0x56, 0xAA, 0xBB} // fake NAS Auth Request

	msg := &DownlinkNASTransport{
		AMFUENGAPID: 1,
		RANUENGAPID: 42,
		NASPDU:      nasPDU,
	}

	rawPDU, err := EncodeDownlinkNASTransport(msg)
	require.NoError(t, err)

	pdu, err := DecodePDU(rawPDU)
	require.NoError(t, err)
	assert.Equal(t, PDUInitiatingMessage, pdu.Type)
	assert.Equal(t, ProcDownlinkNASTransport, pdu.ProcedureCode)

	ies, err := DecodeIEContainer(pdu.Value)
	require.NoError(t, err)

	decoded, err := DecodeDownlinkNASTransport(ies)
	require.NoError(t, err)
	assert.Equal(t, msg.AMFUENGAPID, decoded.AMFUENGAPID)
	assert.Equal(t, msg.RANUENGAPID, decoded.RANUENGAPID)
	assert.True(t, bytes.Equal(nasPDU, decoded.NASPDU))
}

func TestAMFUENGAPIDAPERGolden(t *testing.T) {
	tests := []struct {
		name string
		id   uint64
		hex  string
	}{
		{name: "zero", id: 0, hex: "0000"},
		{name: "one", id: 1, hex: "0001"},
		{name: "max", id: 1099511627775, hex: "ffffffffff"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			encoded, err := EncodeAMFUENGAPID(tt.id)
			require.NoError(t, err)
			assert.Equal(t, tt.hex, hex.EncodeToString(encoded))

			decoded, err := DecodeAMFUENGAPID(encoded)
			require.NoError(t, err)
			assert.Equal(t, tt.id, decoded)
		})
	}
}

func TestRANUENGAPIDAPERGolden(t *testing.T) {
	tests := []struct {
		name string
		id   uint64
		hex  string
	}{
		{name: "zero", id: 0, hex: "0000"},
		{name: "forty-two", id: 42, hex: "002a"},
		{name: "max", id: 4294967295, hex: "ffffffff"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			encoded, err := EncodeRANUENGAPID(tt.id)
			require.NoError(t, err)
			assert.Equal(t, tt.hex, hex.EncodeToString(encoded))

			decoded, err := DecodeRANUENGAPID(encoded)
			require.NoError(t, err)
			assert.Equal(t, tt.id, decoded)
		})
	}
}

func TestDownlinkNASTransportAPERGolden(t *testing.T) {
	rawPDU, err := EncodeDownlinkNASTransport(&DownlinkNASTransport{
		AMFUENGAPID: 1,
		RANUENGAPID: 42,
		NASPDU:      []byte{0x7E, 0x00, 0x56, 0xAA, 0xBB},
	})
	require.NoError(t, err)

	assert.Equal(t,
		"00044019000003000a0002000100550002002a00260006057e0056aabb",
		hex.EncodeToString(rawPDU),
	)
}

func TestUplinkNASTransportRoundTrip(t *testing.T) {
	nasPDU := []byte{0x7E, 0x00, 0x57, 0x01, 0x02} // fake NAS Auth Response

	msg := &UplinkNASTransport{
		AMFUENGAPID: 1,
		RANUENGAPID: 42,
		NASPDU:      nasPDU,
		UserLocationInfo: UserLocationInformationNR{
			NRCGI: testNRCGI,
			TAI:   testTAI,
		},
	}

	rawPDU, err := EncodeUplinkNASTransport(msg)
	require.NoError(t, err)

	pdu, err := DecodePDU(rawPDU)
	require.NoError(t, err)
	assert.Equal(t, ProcUplinkNASTransport, pdu.ProcedureCode)

	ies, err := DecodeIEContainer(pdu.Value)
	require.NoError(t, err)

	decoded, err := DecodeUplinkNASTransport(ies)
	require.NoError(t, err)
	assert.Equal(t, msg.AMFUENGAPID, decoded.AMFUENGAPID)
	assert.Equal(t, msg.RANUENGAPID, decoded.RANUENGAPID)
	assert.True(t, bytes.Equal(nasPDU, decoded.NASPDU))
}

func TestInitialContextSetupRoundTrip(t *testing.T) {
	req := &InitialContextSetupRequest{
		AMFUENGAPID:                 1,
		RANUENGAPID:                 42,
		UEAggregateMaximumBitRateDL: 1000000000,
		UEAggregateMaximumBitRateUL: 500000000,
		GUAMI:                       testGUAMI,
		AllowedNSSAI:                []SNSSAI{{SST: 1}},
		SecurityKey:                 [32]byte{0: 0xAA, 31: 0xBB},
		NASPDU:                      []byte{0x7E, 0x00, 0x42}, // fake Registration Accept
	}
	req.UESecurityCapabilities.NREncAlgs = [2]byte{0xE0, 0x00}
	req.UESecurityCapabilities.NRIntAlgs = [2]byte{0xE0, 0x00}

	rawPDU, err := EncodeInitialContextSetupRequest(req)
	require.NoError(t, err)

	pdu, err := DecodePDU(rawPDU)
	require.NoError(t, err)
	assert.Equal(t, PDUInitiatingMessage, pdu.Type)
	assert.Equal(t, ProcInitialContextSetup, pdu.ProcedureCode)

	ies, err := DecodeIEContainer(pdu.Value)
	require.NoError(t, err)

	decoded, err := DecodeInitialContextSetupRequest(ies)
	require.NoError(t, err)
	assert.Equal(t, req.AMFUENGAPID, decoded.AMFUENGAPID)
	assert.Equal(t, req.RANUENGAPID, decoded.RANUENGAPID)
	assert.Equal(t, req.UEAggregateMaximumBitRateDL, decoded.UEAggregateMaximumBitRateDL)
	assert.Equal(t, req.UEAggregateMaximumBitRateUL, decoded.UEAggregateMaximumBitRateUL)
	assert.Equal(t, testGUAMI, decoded.GUAMI)
	assert.Equal(t, req.SecurityKey, decoded.SecurityKey)
	assert.True(t, bytes.Equal(req.NASPDU, decoded.NASPDU))
}

func TestInitialContextSetupResponseRoundTrip(t *testing.T) {
	resp := &InitialContextSetupResponse{
		AMFUENGAPID: 1,
		RANUENGAPID: 42,
	}
	rawPDU, err := EncodeInitialContextSetupResponse(resp)
	require.NoError(t, err)

	pdu, err := DecodePDU(rawPDU)
	require.NoError(t, err)
	assert.Equal(t, PDUSuccessfulOutcome, pdu.Type)
	assert.Equal(t, ProcInitialContextSetup, pdu.ProcedureCode)
}
