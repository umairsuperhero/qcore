package s1ap

import (
	"encoding/binary"
	"fmt"
)

// PEREncoder handles ASN.1 ALIGNED PER encoding (ITU-T X.691).
// S1AP uses a subset of PER: no unaligned, no real/enumerated extensions.
// We implement exactly what S1AP needs, nothing more.
type PEREncoder struct {
	buf     []byte
	bitOff  uint // current bit offset within current byte (0-7)
}

// NewPEREncoder creates a new PER encoder.
func NewPEREncoder() *PEREncoder {
	return &PEREncoder{
		buf: make([]byte, 0, 256),
	}
}

// Bytes returns the encoded bytes. Must be called after all encoding is done.
func (e *PEREncoder) Bytes() []byte {
	// If we have partial bits, they're already in the last byte
	return e.buf
}

// align moves to the next byte boundary (pad with zero bits).
func (e *PEREncoder) align() {
	if e.bitOff != 0 {
		e.bitOff = 0
		// The partial byte is already appended; just reset bit offset
	}
}

// putBits writes n bits (1-8) from the low bits of val.
func (e *PEREncoder) putBits(val uint8, n uint) {
	if n == 0 || n > 8 {
		return
	}
	// mask to n bits
	val &= (1 << n) - 1

	if e.bitOff == 0 {
		// Start a new byte
		e.buf = append(e.buf, 0)
	}

	room := 8 - e.bitOff // bits remaining in current byte
	if n <= room {
		// Fits in current byte
		e.buf[len(e.buf)-1] |= val << (room - n)
		e.bitOff += n
		if e.bitOff == 8 {
			e.bitOff = 0
		}
	} else {
		// Split across two bytes
		hi := val >> (n - room)
		e.buf[len(e.buf)-1] |= hi
		e.bitOff = 0
		// Start new byte with remaining bits
		remaining := n - room
		e.buf = append(e.buf, 0)
		e.buf[len(e.buf)-1] = val << (8 - remaining)
		e.bitOff = remaining
	}
}

// PutBool encodes a BOOLEAN (1 bit).
func (e *PEREncoder) PutBool(v bool) {
	if v {
		e.putBits(1, 1)
	} else {
		e.putBits(0, 1)
	}
}

// PutConstrainedInt encodes a constrained whole number (X.691 Section 10.5).
// range = ub - lb + 1. For range <= 255, uses minimal bits; for range <= 65536,
// uses 1 or 2 octets aligned.
func (e *PEREncoder) PutConstrainedInt(val, lb, ub int64) error {
	if val < lb || val > ub {
		return fmt.Errorf("value %d out of range [%d, %d]", val, lb, ub)
	}
	rng := ub - lb + 1
	v := val - lb

	switch {
	case rng == 1:
		// No encoding needed
		return nil
	case rng <= 2:
		e.putBits(uint8(v), 1)
	case rng <= 4:
		e.putBits(uint8(v), 2)
	case rng <= 8:
		e.putBits(uint8(v), 3)
	case rng <= 16:
		e.putBits(uint8(v), 4)
	case rng <= 32:
		e.putBits(uint8(v), 5)
	case rng <= 64:
		e.putBits(uint8(v), 6)
	case rng <= 128:
		e.putBits(uint8(v), 7)
	case rng <= 256:
		e.align()
		e.PutBytes([]byte{uint8(v)})
	case rng <= 65536:
		e.align()
		b := make([]byte, 2)
		binary.BigEndian.PutUint16(b, uint16(v))
		e.PutBytes(b)
	default:
		// Unconstrained or semi-constrained — use length-determinant + bytes
		return e.PutUnconstrainedInt(v)
	}
	return nil
}

// PutUnconstrainedInt encodes an unconstrained integer (length + value).
func (e *PEREncoder) PutUnconstrainedInt(v int64) error {
	e.align()
	switch {
	case v <= 0xFF:
		e.PutBytes([]byte{1, uint8(v)})
	case v <= 0xFFFF:
		b := make([]byte, 3)
		b[0] = 2
		binary.BigEndian.PutUint16(b[1:], uint16(v))
		e.PutBytes(b)
	case v <= 0xFFFFFFFF:
		b := make([]byte, 5)
		b[0] = 4
		binary.BigEndian.PutUint32(b[1:], uint32(v))
		e.PutBytes(b)
	default:
		return fmt.Errorf("integer too large: %d", v)
	}
	return nil
}

// PutLengthDeterminant encodes a normally-small or constrained length (X.691 Section 10.9).
// For S1AP, lengths are typically < 16384 (no fragmentation needed).
func (e *PEREncoder) PutLengthDeterminant(length int) error {
	e.align()
	if length < 128 {
		e.PutBytes([]byte{uint8(length)})
	} else if length < 16384 {
		b := make([]byte, 2)
		binary.BigEndian.PutUint16(b, uint16(length)|0x8000)
		e.PutBytes(b)
	} else {
		return fmt.Errorf("length %d too large for non-fragmented encoding", length)
	}
	return nil
}

// PutOctetString encodes an OCTET STRING with a length determinant.
func (e *PEREncoder) PutOctetString(data []byte) error {
	if err := e.PutLengthDeterminant(len(data)); err != nil {
		return err
	}
	e.PutBytes(data)
	return nil
}

// PutFixedOctetString encodes a fixed-length OCTET STRING (no length prefix).
func (e *PEREncoder) PutFixedOctetString(data []byte) {
	e.align()
	e.PutBytes(data)
}

// PutBitString encodes a BIT STRING with known fixed length in bits.
// The data is byte-aligned; unused bits in the last byte are high-order.
func (e *PEREncoder) PutBitString(data []byte, numBits int) {
	e.align()
	e.PutBytes(data)
	_ = numBits // for documentation; data already padded
}

// PutBytes appends raw bytes at the current position.
// Caller must ensure alignment if needed.
func (e *PEREncoder) PutBytes(data []byte) {
	if e.bitOff != 0 {
		// We're mid-byte — should have called align() first for octet-aligned data
		// Force alignment
		e.align()
	}
	e.buf = append(e.buf, data...)
}

// PutChoiceIndex encodes a CHOICE index for a type with numChoices alternatives.
func (e *PEREncoder) PutChoiceIndex(index, numChoices int) error {
	if index < 0 || index >= numChoices {
		return fmt.Errorf("choice index %d out of range [0, %d)", index, numChoices)
	}
	return e.PutConstrainedInt(int64(index), 0, int64(numChoices-1))
}

// PutSequenceHeader encodes the extension bit and optional bitmap for a SEQUENCE.
// extensible: whether the type has "..." in its definition
// optionalBits: bitmap of present optional elements (bit 0 = first optional)
// numOptional: how many optional elements the SEQUENCE has
func (e *PEREncoder) PutSequenceHeader(extensible bool, optionalBits uint64, numOptional int) {
	if extensible {
		e.putBits(0, 1) // extension marker = false (no extensions present)
	}
	for i := 0; i < numOptional; i++ {
		if optionalBits&(1<<uint(i)) != 0 {
			e.putBits(1, 1)
		} else {
			e.putBits(0, 1)
		}
	}
}

// EncodePDU encodes a complete S1AP PDU (top-level container).
func EncodePDU(pdu *PDU) ([]byte, error) {
	enc := NewPEREncoder()

	// S1AP-PDU is a CHOICE { initiatingMessage, successfulOutcome, unsuccessfulOutcome }
	// 3 choices → 2 bits
	if err := enc.PutChoiceIndex(int(pdu.Type), 3); err != nil {
		return nil, fmt.Errorf("encoding PDU type: %w", err)
	}

	// Each alternative is a SEQUENCE { procedureCode, criticality, value }
	// procedureCode: INTEGER (0..255)
	enc.align()
	enc.PutBytes([]byte{uint8(pdu.ProcedureCode)})

	// criticality: ENUMERATED { reject, ignore, notify } — 2 bits
	if err := enc.PutConstrainedInt(int64(pdu.Criticality), 0, 2); err != nil {
		return nil, fmt.Errorf("encoding criticality: %w", err)
	}

	// value: OPEN TYPE (length + content)
	enc.align()
	if err := enc.PutLengthDeterminant(len(pdu.Value)); err != nil {
		return nil, fmt.Errorf("encoding value length: %w", err)
	}
	enc.PutBytes(pdu.Value)

	return enc.Bytes(), nil
}

// EncodeProtocolIEContainer encodes a list of ProtocolIEs.
// This is the value inside most S1AP messages.
func EncodeProtocolIEContainer(ies []ProtocolIE) ([]byte, error) {
	enc := NewPEREncoder()

	// protocolIEs is a SEQUENCE (SIZE (0..maxProtocolIEs)) OF ProtocolIE-Field
	// The count is constrained; we encode it as a length
	// S1AP uses 0..65535 for the list count
	countBytes := make([]byte, 2)
	binary.BigEndian.PutUint16(countBytes, uint16(len(ies)))
	enc.PutBytes(countBytes)

	for _, ie := range ies {
		// ProtocolIE-Field ::= SEQUENCE { id, criticality, value }
		// id: ProtocolIE-ID (INTEGER 0..65535)
		idBytes := make([]byte, 2)
		binary.BigEndian.PutUint16(idBytes, uint16(ie.ID))
		enc.PutBytes(idBytes)

		// criticality: ENUMERATED { reject, ignore, notify }
		if err := enc.PutConstrainedInt(int64(ie.Criticality), 0, 2); err != nil {
			return nil, fmt.Errorf("encoding IE %d criticality: %w", ie.ID, err)
		}

		// value: OPEN TYPE (length + content)
		enc.align()
		if err := enc.PutLengthDeterminant(len(ie.Value)); err != nil {
			return nil, fmt.Errorf("encoding IE %d value length: %w", ie.ID, err)
		}
		enc.PutBytes(ie.Value)
	}

	return enc.Bytes(), nil
}
