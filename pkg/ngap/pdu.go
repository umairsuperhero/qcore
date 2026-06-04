package ngap

import (
	"fmt"
)

// DecodePDU decodes a complete NGAP PDU from raw bytes.
func DecodePDU(data []byte) (*PDU, error) {
	dec := NewPERDecoder(data)

	// NGAP-PDU CHOICE (3 alternatives, extensible)
	pduType, err := dec.GetChoiceIndex(3, true)
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

	// NGAP-PDU CHOICE (3 alternatives, extensible)
	if err := enc.PutChoiceIndex(int(pdu.Type), 3, true); err != nil {
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
// It also reads the message-level SEQUENCE header (ext bit) that wraps the container.
func DecodeIEContainer(data []byte) ([]ProtocolIE, error) {
	dec := NewPERDecoder(data)

	// Every NGAP message is a SEQUENCE { protocolIEs ProtocolIE-Container, ... }
	// so we must read the sequence header (extensible, 0 optional root fields).
	if _, _, err := dec.GetSequenceHeader(true, 0); err != nil {
		return nil, fmt.Errorf("ngap: decoding message sequence header: %w", err)
	}

	// Count: SEQUENCE SIZE(0..65535)
	countInt, err := dec.GetConstrainedInt(0, 65535)
	if err != nil {
		return nil, fmt.Errorf("ngap: decoding IE count: %w", err)
	}
	count := int(countInt)

	ies := make([]ProtocolIE, 0, count)
	for i := 0; i < count; i++ {
		// id: ProtocolIE-ID (2 bytes)
		idInt, err := dec.GetConstrainedInt(0, 65535)
		if err != nil {
			return nil, fmt.Errorf("ngap: decoding IE %d id: %w", i, err)
		}
		id := ProtocolIEID(idInt)

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
			return nil, fmt.Errorf("ngap: decoding IE %d (id=%d) value (%d bytes): %w", i, id, valueLen, err)
		}

		ies = append(ies, ProtocolIE{
			ID:          id,
			Criticality: Criticality(crit),
			Value:       value,
		})
	}

	return ies, nil
}

// EncodeIEContainer encodes a list of ProtocolIEs into an IE container,
// wrapped in the message-level SEQUENCE header.
func EncodeIEContainer(ies []ProtocolIE) ([]byte, error) {
	enc := NewPEREncoder()

	// Message-level SEQUENCE header (extensible, 0 optional root fields)
	enc.PutSequenceHeader(true, 0, 0)

	// Count: SEQUENCE SIZE(0..65535)
	if err := enc.PutConstrainedInt(int64(len(ies)), 0, 65535); err != nil {
		return nil, fmt.Errorf("ngap: encoding IE count: %w", err)
	}

	for _, ie := range ies {
		// id: ProtocolIE-ID (2 bytes, 0..65535)
		if err := enc.PutConstrainedInt(int64(ie.ID), 0, 65535); err != nil {
			return nil, fmt.Errorf("ngap: encoding IE ID: %w", err)
		}

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
