package s1ap

import (
	"encoding/binary"
	"fmt"
)

// IE encoding/decoding helpers for S1AP Information Elements.
// Each function encodes/decodes a specific IE value (the contents inside
// the OPEN TYPE wrapper, not the ProtocolIE container).

// EncodeGlobalENBID encodes a Global-ENB-ID IE.
// GlobalENBID ::= SEQUENCE { pLMNidentity, eNB-ID CHOICE { macro, home } }
func EncodeGlobalENBID(g GlobalENBID) ([]byte, error) {
	enc := NewPEREncoder()
	// SEQUENCE with extension marker, no optional fields
	enc.PutSequenceHeader(true, 0, 0)

	// pLMNidentity: OCTET STRING (SIZE(3))
	enc.PutFixedOctetString(g.PLMN[:])

	// eNB-ID: CHOICE { macroENB-ID BIT STRING (SIZE(20)), homeENB-ID BIT STRING (SIZE(28)) }
	switch g.Type {
	case MacroENBID:
		if err := enc.PutChoiceIndex(0, 2); err != nil {
			return nil, err
		}
		// 20-bit macro eNB ID → 3 bytes (4 unused bits at the end)
		b := make([]byte, 3)
		b[0] = uint8(g.ENBID >> 12)
		b[1] = uint8(g.ENBID >> 4)
		b[2] = uint8(g.ENBID<<4) & 0xF0
		enc.PutBitString(b, 20)
	case HomeENBID:
		if err := enc.PutChoiceIndex(1, 2); err != nil {
			return nil, err
		}
		// 28-bit home eNB ID → 4 bytes (4 unused bits at the end)
		b := make([]byte, 4)
		b[0] = uint8(g.ENBID >> 20)
		b[1] = uint8(g.ENBID >> 12)
		b[2] = uint8(g.ENBID >> 4)
		b[3] = uint8(g.ENBID<<4) & 0xF0
		enc.PutBitString(b, 28)
	default:
		return nil, fmt.Errorf("unknown eNB ID type: %d", g.Type)
	}

	return enc.Bytes(), nil
}

// DecodeGlobalENBID decodes a Global-ENB-ID IE value.
func DecodeGlobalENBID(data []byte) (GlobalENBID, error) {
	dec := NewPERDecoder(data)
	var g GlobalENBID

	// Extension marker
	_, _, err := dec.GetSequenceHeader(true, 0)
	if err != nil {
		return g, err
	}

	// pLMNidentity
	plmn, err := dec.GetFixedOctetString(3)
	if err != nil {
		return g, fmt.Errorf("decoding PLMN: %w", err)
	}
	copy(g.PLMN[:], plmn)

	// eNB-ID CHOICE
	choice, err := dec.GetChoiceIndex(2)
	if err != nil {
		return g, fmt.Errorf("decoding eNB-ID choice: %w", err)
	}

	switch choice {
	case 0: // macroENB-ID (20 bits)
		g.Type = MacroENBID
		b, err := dec.GetFixedOctetString(3)
		if err != nil {
			return g, fmt.Errorf("decoding macro eNB ID: %w", err)
		}
		g.ENBID = uint32(b[0])<<12 | uint32(b[1])<<4 | uint32(b[2])>>4
	case 1: // homeENB-ID (28 bits)
		g.Type = HomeENBID
		b, err := dec.GetFixedOctetString(4)
		if err != nil {
			return g, fmt.Errorf("decoding home eNB ID: %w", err)
		}
		g.ENBID = uint32(b[0])<<20 | uint32(b[1])<<12 | uint32(b[2])<<4 | uint32(b[3])>>4
	default:
		return g, fmt.Errorf("unknown eNB-ID choice: %d", choice)
	}

	return g, nil
}

// EncodeSupportedTAs encodes a SupportedTAs IE (list of SupportedTA).
func EncodeSupportedTAs(tas []SupportedTA) ([]byte, error) {
	enc := NewPEREncoder()

	// SupportedTAs ::= SEQUENCE (SIZE(1..maxnoofTACs)) OF SupportedTAs-Item
	// maxnoofTACs = 256, so length is constrained 1..256
	if err := enc.PutConstrainedInt(int64(len(tas)), 1, 256); err != nil {
		return nil, err
	}

	for _, ta := range tas {
		// SupportedTAs-Item ::= SEQUENCE { tAC, broadcastPLMNs, ... }
		enc.PutSequenceHeader(true, 0, 0)

		// tAC: OCTET STRING (SIZE(2))
		tacBytes := make([]byte, 2)
		binary.BigEndian.PutUint16(tacBytes, ta.TAC)
		enc.PutFixedOctetString(tacBytes)

		// broadcastPLMNs: BPLMNs ::= SEQUENCE (SIZE(1..maxnoofBPLMNs)) OF PLMNidentity
		// maxnoofBPLMNs = 6
		if err := enc.PutConstrainedInt(int64(len(ta.PLMNs)), 1, 6); err != nil {
			return nil, err
		}
		for _, plmn := range ta.PLMNs {
			enc.PutFixedOctetString(plmn[:])
		}
	}

	return enc.Bytes(), nil
}

// DecodeSupportedTAs decodes a SupportedTAs IE value.
func DecodeSupportedTAs(data []byte) ([]SupportedTA, error) {
	dec := NewPERDecoder(data)

	count, err := dec.GetConstrainedInt(1, 256)
	if err != nil {
		return nil, fmt.Errorf("decoding TA count: %w", err)
	}

	tas := make([]SupportedTA, count)
	for i := range tas {
		_, _, err := dec.GetSequenceHeader(true, 0)
		if err != nil {
			return nil, fmt.Errorf("decoding TA %d header: %w", i, err)
		}

		tacBytes, err := dec.GetFixedOctetString(2)
		if err != nil {
			return nil, fmt.Errorf("decoding TA %d TAC: %w", i, err)
		}
		tas[i].TAC = binary.BigEndian.Uint16(tacBytes)

		plmnCount, err := dec.GetConstrainedInt(1, 6)
		if err != nil {
			return nil, fmt.Errorf("decoding TA %d PLMN count: %w", i, err)
		}

		tas[i].PLMNs = make([][3]byte, plmnCount)
		for j := range tas[i].PLMNs {
			plmn, err := dec.GetFixedOctetString(3)
			if err != nil {
				return nil, fmt.Errorf("decoding TA %d PLMN %d: %w", i, j, err)
			}
			copy(tas[i].PLMNs[j][:], plmn)
		}
	}

	return tas, nil
}

// EncodeServedGUMMEIs encodes a ServedGUMMEIs IE.
func EncodeServedGUMMEIs(gummeis []ServedGUMMEI) ([]byte, error) {
	enc := NewPEREncoder()

	// ServedGUMMEIs ::= SEQUENCE (SIZE(1..maxnoofRATs)) OF ServedGUMMEIsItem
	// maxnoofRATs = 8
	if err := enc.PutConstrainedInt(int64(len(gummeis)), 1, 8); err != nil {
		return nil, err
	}

	for _, g := range gummeis {
		// ServedGUMMEIsItem ::= SEQUENCE { servedPLMNs, servedGroupIDs, servedMMECs, ... }
		enc.PutSequenceHeader(true, 0, 0)

		// servedPLMNs: SEQUENCE (SIZE(1..maxnoofPLMNsPerMME=32)) OF PLMNidentity
		if err := enc.PutConstrainedInt(int64(len(g.ServedPLMNs)), 1, 32); err != nil {
			return nil, err
		}
		for _, plmn := range g.ServedPLMNs {
			enc.PutFixedOctetString(plmn[:])
		}

		// servedGroupIDs: SEQUENCE (SIZE(1..maxnoofGroupIDs=65535)) OF MME-Group-ID
		// MME-Group-ID is OCTET STRING (SIZE(2))
		if err := enc.PutConstrainedInt(int64(len(g.ServedGroupIDs)), 1, 65535); err != nil {
			return nil, err
		}
		for _, gid := range g.ServedGroupIDs {
			b := make([]byte, 2)
			binary.BigEndian.PutUint16(b, gid)
			enc.PutFixedOctetString(b)
		}

		// servedMMECs: SEQUENCE (SIZE(1..maxnoofMMECs=256)) OF MME-Code
		// MME-Code is OCTET STRING (SIZE(1))
		if err := enc.PutConstrainedInt(int64(len(g.ServedMMECs)), 1, 256); err != nil {
			return nil, err
		}
		for _, code := range g.ServedMMECs {
			enc.PutFixedOctetString([]byte{code})
		}
	}

	return enc.Bytes(), nil
}

// EncodeTAI encodes a TAI IE.
func EncodeTAI(tai TAI) ([]byte, error) {
	enc := NewPEREncoder()
	enc.PutSequenceHeader(true, 0, 0)
	enc.PutFixedOctetString(tai.PLMN[:])
	tacBytes := make([]byte, 2)
	binary.BigEndian.PutUint16(tacBytes, tai.TAC)
	enc.PutFixedOctetString(tacBytes)
	return enc.Bytes(), nil
}

// DecodeTAI decodes a TAI IE value.
func DecodeTAI(data []byte) (TAI, error) {
	dec := NewPERDecoder(data)
	var tai TAI

	_, _, err := dec.GetSequenceHeader(true, 0)
	if err != nil {
		return tai, err
	}

	plmn, err := dec.GetFixedOctetString(3)
	if err != nil {
		return tai, err
	}
	copy(tai.PLMN[:], plmn)

	tacBytes, err := dec.GetFixedOctetString(2)
	if err != nil {
		return tai, err
	}
	tai.TAC = binary.BigEndian.Uint16(tacBytes)

	return tai, nil
}

// EncodeECGI encodes an E-UTRAN CGI IE.
func EncodeECGI(ecgi ECGI) ([]byte, error) {
	enc := NewPEREncoder()
	enc.PutSequenceHeader(true, 0, 0)
	enc.PutFixedOctetString(ecgi.PLMN[:])
	// Cell-ID: BIT STRING (SIZE(28))
	b := make([]byte, 4)
	b[0] = uint8(ecgi.CellID >> 20)
	b[1] = uint8(ecgi.CellID >> 12)
	b[2] = uint8(ecgi.CellID >> 4)
	b[3] = uint8(ecgi.CellID<<4) & 0xF0
	enc.PutBitString(b, 28)
	return enc.Bytes(), nil
}

// DecodeECGI decodes an E-UTRAN CGI IE value.
func DecodeECGI(data []byte) (ECGI, error) {
	dec := NewPERDecoder(data)
	var ecgi ECGI

	_, _, err := dec.GetSequenceHeader(true, 0)
	if err != nil {
		return ecgi, err
	}

	plmn, err := dec.GetFixedOctetString(3)
	if err != nil {
		return ecgi, err
	}
	copy(ecgi.PLMN[:], plmn)

	b, err := dec.GetFixedOctetString(4)
	if err != nil {
		return ecgi, err
	}
	ecgi.CellID = uint32(b[0])<<20 | uint32(b[1])<<12 | uint32(b[2])<<4 | uint32(b[3])>>4

	return ecgi, nil
}
