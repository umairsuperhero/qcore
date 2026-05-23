package ngap

import (
	"encoding/binary"
	"fmt"
)

// DecodePDU decodes a complete NGAP PDU from raw bytes.
func DecodePDU(data []byte) (*PDU, error) {
	dec := NewPERDecoder(data)

	// NGAP-PDU CHOICE (3 alternatives)
	pduType, err := dec.GetChoiceIndex(3)
	if err != nil {
		return nil, fmt.Errorf("ngap: decoding PDU type: %w", err)
	}

	// procedureCode: INTEGER (0..255)
	dec.align()
	pcBytes, err := dec.GetBytes(1)
	if err != nil {
		return nil, fmt.Errorf("ngap: decoding procedure code: %w", err)
	}

	// criticality: ENUMERATED (0..2)
	crit, err := dec.GetConstrainedInt(0, 2)
	if err != nil {
		return nil, fmt.Errorf("ngap: decoding criticality: %w", err)
	}

	// value: OPEN TYPE (length + content)
	dec.align()
	valueLen, err := dec.GetLengthDeterminant()
	if err != nil {
		return nil, fmt.Errorf("ngap: decoding value length: %w", err)
	}
	value, err := dec.GetBytes(valueLen)
	if err != nil {
		return nil, fmt.Errorf("ngap: decoding value (%d bytes): %w", valueLen, err)
	}

	return &PDU{
		Type:          PDUType(pduType),
		ProcedureCode: ProcedureCode(pcBytes[0]),
		Criticality:   Criticality(crit),
		Value:         value,
	}, nil
}

// EncodePDU encodes a complete NGAP PDU.
func EncodePDU(pdu *PDU) ([]byte, error) {
	enc := NewPEREncoder()

	// NGAP-PDU CHOICE (3 alternatives)
	if err := enc.PutChoiceIndex(int(pdu.Type), 3); err != nil {
		return nil, fmt.Errorf("ngap: encoding PDU type: %w", err)
	}

	// procedureCode: INTEGER (0..255)
	enc.align()
	enc.PutBytes([]byte{uint8(pdu.ProcedureCode)})

	// criticality: ENUMERATED
	if err := enc.PutConstrainedInt(int64(pdu.Criticality), 0, 2); err != nil {
		return nil, fmt.Errorf("ngap: encoding criticality: %w", err)
	}

	// value: OPEN TYPE
	enc.align()
	if err := enc.PutLengthDeterminant(len(pdu.Value)); err != nil {
		return nil, fmt.Errorf("ngap: encoding value length: %w", err)
	}
	enc.PutBytes(pdu.Value)

	return enc.Bytes(), nil
}

// DecodeIEContainer decodes the ProtocolIE container from the PDU value field.
func DecodeIEContainer(data []byte) ([]ProtocolIE, error) {
	dec := NewPERDecoder(data)

	// Count: 2 bytes big-endian
	countBytes, err := dec.GetBytes(2)
	if err != nil {
		return nil, fmt.Errorf("ngap: decoding IE count: %w", err)
	}
	count := int(binary.BigEndian.Uint16(countBytes))

	ies := make([]ProtocolIE, 0, count)
	for i := 0; i < count; i++ {
		// id: ProtocolIE-ID (2 bytes)
		idBytes, err := dec.GetBytes(2)
		if err != nil {
			return nil, fmt.Errorf("ngap: decoding IE %d id: %w", i, err)
		}
		id := ProtocolIEID(binary.BigEndian.Uint16(idBytes))

		// criticality: ENUMERATED (0..2)
		crit, err := dec.GetConstrainedInt(0, 2)
		if err != nil {
			return nil, fmt.Errorf("ngap: decoding IE %d (id=%d) criticality: %w", i, id, err)
		}

		// value: OPEN TYPE
		dec.align()
		valueLen, err := dec.GetLengthDeterminant()
		if err != nil {
			return nil, fmt.Errorf("ngap: decoding IE %d (id=%d) value length: %w", i, id, err)
		}
		value, err := dec.GetBytes(valueLen)
		if err != nil {
			return nil, fmt.Errorf("ngap: decoding IE %d (id=%d) value: %w", i, id, err)
		}

		ies = append(ies, ProtocolIE{
			ID:          id,
			Criticality: Criticality(crit),
			Value:       value,
		})
	}

	return ies, nil
}

// EncodeIEContainer encodes a list of ProtocolIEs.
func EncodeIEContainer(ies []ProtocolIE) ([]byte, error) {
	enc := NewPEREncoder()

	// Count: 2 bytes big-endian
	countBytes := make([]byte, 2)
	binary.BigEndian.PutUint16(countBytes, uint16(len(ies)))
	enc.PutBytes(countBytes)

	for _, ie := range ies {
		// id: 2 bytes
		idBytes := make([]byte, 2)
		binary.BigEndian.PutUint16(idBytes, uint16(ie.ID))
		enc.PutBytes(idBytes)

		// criticality: ENUMERATED
		if err := enc.PutConstrainedInt(int64(ie.Criticality), 0, 2); err != nil {
			return nil, fmt.Errorf("ngap: encoding IE %d criticality: %w", ie.ID, err)
		}

		// value: OPEN TYPE
		enc.align()
		if err := enc.PutLengthDeterminant(len(ie.Value)); err != nil {
			return nil, fmt.Errorf("ngap: encoding IE %d value length: %w", ie.ID, err)
		}
		enc.PutBytes(ie.Value)
	}

	return enc.Bytes(), nil
}
