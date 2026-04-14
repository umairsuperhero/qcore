package s1ap

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPEREncoderDecoderBits(t *testing.T) {
	t.Run("single bits", func(t *testing.T) {
		enc := NewPEREncoder()
		enc.PutBool(true)
		enc.PutBool(false)
		enc.PutBool(true)
		enc.PutBool(true)
		enc.PutBool(false)
		enc.PutBool(false)
		enc.PutBool(true)
		enc.PutBool(false)
		// 10110010 = 0xB2
		assert.Equal(t, []byte{0xB2}, enc.Bytes())

		dec := NewPERDecoder(enc.Bytes())
		for i, want := range []bool{true, false, true, true, false, false, true, false} {
			got, err := dec.GetBool()
			require.NoError(t, err, "bit %d", i)
			assert.Equal(t, want, got, "bit %d", i)
		}
	})

	t.Run("constrained int 2 bits", func(t *testing.T) {
		enc := NewPEREncoder()
		err := enc.PutConstrainedInt(2, 0, 3)
		require.NoError(t, err)
		// value 2 in 2 bits = 10, padded = 10000000 = 0x80
		assert.Equal(t, []byte{0x80}, enc.Bytes())

		dec := NewPERDecoder(enc.Bytes())
		v, err := dec.GetConstrainedInt(0, 3)
		require.NoError(t, err)
		assert.Equal(t, int64(2), v)
	})

	t.Run("constrained int 1 byte", func(t *testing.T) {
		enc := NewPEREncoder()
		err := enc.PutConstrainedInt(200, 0, 255)
		require.NoError(t, err)
		assert.Equal(t, []byte{200}, enc.Bytes())

		dec := NewPERDecoder(enc.Bytes())
		v, err := dec.GetConstrainedInt(0, 255)
		require.NoError(t, err)
		assert.Equal(t, int64(200), v)
	})

	t.Run("constrained int 2 bytes", func(t *testing.T) {
		enc := NewPEREncoder()
		err := enc.PutConstrainedInt(1000, 0, 65535)
		require.NoError(t, err)
		assert.Equal(t, []byte{0x03, 0xE8}, enc.Bytes())

		dec := NewPERDecoder(enc.Bytes())
		v, err := dec.GetConstrainedInt(0, 65535)
		require.NoError(t, err)
		assert.Equal(t, int64(1000), v)
	})

	t.Run("length determinant short", func(t *testing.T) {
		enc := NewPEREncoder()
		err := enc.PutLengthDeterminant(42)
		require.NoError(t, err)
		assert.Equal(t, []byte{42}, enc.Bytes())

		dec := NewPERDecoder(enc.Bytes())
		v, err := dec.GetLengthDeterminant()
		require.NoError(t, err)
		assert.Equal(t, 42, v)
	})

	t.Run("length determinant long", func(t *testing.T) {
		enc := NewPEREncoder()
		err := enc.PutLengthDeterminant(500)
		require.NoError(t, err)
		// 500 = 0x01F4, with high bit set: 0x81F4
		assert.Equal(t, []byte{0x81, 0xF4}, enc.Bytes())

		dec := NewPERDecoder(enc.Bytes())
		v, err := dec.GetLengthDeterminant()
		require.NoError(t, err)
		assert.Equal(t, 500, v)
	})
}

func TestPDURoundTrip(t *testing.T) {
	original := &PDU{
		Type:          PDUInitiatingMessage,
		ProcedureCode: ProcS1Setup,
		Criticality:   CriticalityReject,
		Value:         []byte{0xDE, 0xAD, 0xBE, 0xEF},
	}

	encoded, err := EncodePDU(original)
	require.NoError(t, err)

	decoded, err := DecodePDU(encoded)
	require.NoError(t, err)

	assert.Equal(t, original.Type, decoded.Type)
	assert.Equal(t, original.ProcedureCode, decoded.ProcedureCode)
	assert.Equal(t, original.Criticality, decoded.Criticality)
	assert.Equal(t, original.Value, decoded.Value)
}

func TestProtocolIEContainerRoundTrip(t *testing.T) {
	ies := []ProtocolIE{
		{ID: IEID_Global_ENB_ID, Criticality: CriticalityReject, Value: []byte{0x01, 0x02, 0x03}},
		{ID: IEID_ENBname, Criticality: CriticalityIgnore, Value: []byte("test-enb")},
		{ID: IEID_SupportedTAs, Criticality: CriticalityReject, Value: []byte{0xAA, 0xBB}},
	}

	encoded, err := EncodeProtocolIEContainer(ies)
	require.NoError(t, err)

	decoded, err := DecodeProtocolIEContainer(encoded)
	require.NoError(t, err)

	require.Len(t, decoded, 3)
	for i, ie := range decoded {
		assert.Equal(t, ies[i].ID, ie.ID)
		assert.Equal(t, ies[i].Criticality, ie.Criticality)
		assert.Equal(t, ies[i].Value, ie.Value)
	}
}

func TestGlobalENBIDRoundTrip(t *testing.T) {
	t.Run("macro eNB", func(t *testing.T) {
		original := GlobalENBID{
			PLMN:  [3]byte{0x00, 0xF1, 0x10},
			ENBID: 0x12345, // 20-bit
			Type:  MacroENBID,
		}

		encoded, err := EncodeGlobalENBID(original)
		require.NoError(t, err)

		decoded, err := DecodeGlobalENBID(encoded)
		require.NoError(t, err)

		assert.Equal(t, original.PLMN, decoded.PLMN)
		assert.Equal(t, original.ENBID, decoded.ENBID)
		assert.Equal(t, original.Type, decoded.Type)
	})

	t.Run("home eNB", func(t *testing.T) {
		original := GlobalENBID{
			PLMN:  [3]byte{0x00, 0xF1, 0x10},
			ENBID: 0x1234567, // 28-bit
			Type:  HomeENBID,
		}

		encoded, err := EncodeGlobalENBID(original)
		require.NoError(t, err)

		decoded, err := DecodeGlobalENBID(encoded)
		require.NoError(t, err)

		assert.Equal(t, original.PLMN, decoded.PLMN)
		assert.Equal(t, original.ENBID, decoded.ENBID)
		assert.Equal(t, original.Type, decoded.Type)
	})
}

func TestSupportedTAsRoundTrip(t *testing.T) {
	original := []SupportedTA{
		{
			TAC:   0x0001,
			PLMNs: [][3]byte{{0x00, 0xF1, 0x10}},
		},
		{
			TAC:   0x0002,
			PLMNs: [][3]byte{{0x00, 0xF1, 0x10}, {0x00, 0xF2, 0x20}},
		},
	}

	encoded, err := EncodeSupportedTAs(original)
	require.NoError(t, err)

	decoded, err := DecodeSupportedTAs(encoded)
	require.NoError(t, err)

	require.Len(t, decoded, 2)
	assert.Equal(t, original[0].TAC, decoded[0].TAC)
	assert.Equal(t, original[0].PLMNs, decoded[0].PLMNs)
	assert.Equal(t, original[1].TAC, decoded[1].TAC)
	assert.Equal(t, original[1].PLMNs, decoded[1].PLMNs)
}

func TestTAIRoundTrip(t *testing.T) {
	original := TAI{
		PLMN: [3]byte{0x00, 0xF1, 0x10},
		TAC:  0x0001,
	}

	encoded, err := EncodeTAI(original)
	require.NoError(t, err)

	decoded, err := DecodeTAI(encoded)
	require.NoError(t, err)

	assert.Equal(t, original, decoded)
}

func TestECGIRoundTrip(t *testing.T) {
	original := ECGI{
		PLMN:   [3]byte{0x00, 0xF1, 0x10},
		CellID: 0x1234567,
	}

	encoded, err := EncodeECGI(original)
	require.NoError(t, err)

	decoded, err := DecodeECGI(encoded)
	require.NoError(t, err)

	assert.Equal(t, original, decoded)
}

func TestS1SetupResponseEncode(t *testing.T) {
	resp := &S1SetupResponse{
		MMEName: "qcore-mme",
		ServedGUMMEIs: []ServedGUMMEI{
			{
				ServedPLMNs:    [][3]byte{{0x00, 0xF1, 0x10}},
				ServedGroupIDs: []uint16{1},
				ServedMMECs:    []uint8{1},
			},
		},
		RelativeCapacity: 127,
	}

	encoded, err := EncodeS1SetupResponse(resp)
	require.NoError(t, err)
	assert.NotEmpty(t, encoded)

	// Verify it decodes as a valid PDU
	pdu, err := DecodePDU(encoded)
	require.NoError(t, err)
	assert.Equal(t, PDUSuccessfulOutcome, pdu.Type)
	assert.Equal(t, ProcS1Setup, pdu.ProcedureCode)
}

func TestChoiceIndex(t *testing.T) {
	for _, tc := range []struct {
		index, numChoices int
	}{
		{0, 3},
		{1, 3},
		{2, 3},
		{0, 2},
		{1, 2},
	} {
		enc := NewPEREncoder()
		err := enc.PutChoiceIndex(tc.index, tc.numChoices)
		require.NoError(t, err)

		dec := NewPERDecoder(enc.Bytes())
		got, err := dec.GetChoiceIndex(tc.numChoices)
		require.NoError(t, err)
		assert.Equal(t, tc.index, got)
	}
}
