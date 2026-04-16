package s1ap

import (
	"encoding/binary"
	"fmt"
)

// PERDecoder handles ASN.1 ALIGNED PER decoding (ITU-T X.691).
type PERDecoder struct {
	data   []byte
	off    int  // byte offset
	bitOff uint // bit offset within current byte (0-7)
}

// NewPERDecoder creates a decoder from raw bytes.
func NewPERDecoder(data []byte) *PERDecoder {
	return &PERDecoder{data: data}
}

// Remaining returns how many bytes are left.
func (d *PERDecoder) Remaining() int {
	r := len(d.data) - d.off
	if d.bitOff > 0 {
		r-- // current byte is partially consumed
	}
	if r < 0 {
		return 0
	}
	return r
}

// align advances to the next byte boundary.
func (d *PERDecoder) align() {
	if d.bitOff != 0 {
		d.off++
		d.bitOff = 0
	}
}

// Align is the exported form of align (for cross-package callers that compose PER values).
func (d *PERDecoder) Align() { d.align() }

// GetBits is the exported form of getBits.
func (d *PERDecoder) GetBits(n uint) (uint8, error) { return d.getBits(n) }

// GetBitString decodes a BIT STRING with a length determinant (number of bits).
// It returns the bit string aligned into bytes with any trailing unused bits
// in the last byte set to 0. This is sufficient for S1AP TransportLayerAddress
// where we interpret 32-bit bit strings as IPv4 addresses.
func (d *PERDecoder) GetBitString() ([]byte, error) {
	length, err := d.GetLengthDeterminant()
	if err != nil {
		return nil, err
	}
	d.align()
	byteLen := (length + 7) / 8
	if byteLen == 0 {
		return nil, nil
	}
	out := make([]byte, byteLen)
	b, err := d.GetBytes(byteLen)
	if err != nil {
		return nil, err
	}
	copy(out, b)
	// Mask any padding bits in the last byte
	if rem := length % 8; rem != 0 {
		out[byteLen-1] &= byte(0xFF) << uint(8-rem)
	}
	return out, nil
}

// getBits reads n bits (1-8) and returns them in the low bits.
func (d *PERDecoder) getBits(n uint) (uint8, error) {
	if n == 0 || n > 8 {
		return 0, fmt.Errorf("invalid bit count: %d", n)
	}
	if d.off >= len(d.data) {
		return 0, fmt.Errorf("unexpected end of data at byte %d (need %d bits)", d.off, n)
	}

	room := 8 - d.bitOff
	if n <= room {
		// All bits from current byte
		shift := room - n
		val := (d.data[d.off] >> shift) & ((1 << n) - 1)
		d.bitOff += n
		if d.bitOff == 8 {
			d.off++
			d.bitOff = 0
		}
		return val, nil
	}

	// Split across two bytes
	hi := d.data[d.off] & ((1 << room) - 1) // remaining bits from current byte
	d.off++
	d.bitOff = 0

	if d.off >= len(d.data) {
		return 0, fmt.Errorf("unexpected end of data at byte %d", d.off)
	}

	remaining := n - room
	lo := (d.data[d.off] >> (8 - remaining)) & ((1 << remaining) - 1)
	d.bitOff = remaining

	return (hi << remaining) | lo, nil
}

// GetBool decodes a BOOLEAN (1 bit).
func (d *PERDecoder) GetBool() (bool, error) {
	v, err := d.getBits(1)
	return v == 1, err
}

// GetConstrainedInt decodes a constrained whole number.
func (d *PERDecoder) GetConstrainedInt(lb, ub int64) (int64, error) {
	rng := ub - lb + 1

	var v int64
	switch {
	case rng == 1:
		return lb, nil
	case rng <= 2:
		b, err := d.getBits(1)
		if err != nil {
			return 0, err
		}
		v = int64(b)
	case rng <= 4:
		b, err := d.getBits(2)
		if err != nil {
			return 0, err
		}
		v = int64(b)
	case rng <= 8:
		b, err := d.getBits(3)
		if err != nil {
			return 0, err
		}
		v = int64(b)
	case rng <= 16:
		b, err := d.getBits(4)
		if err != nil {
			return 0, err
		}
		v = int64(b)
	case rng <= 32:
		b, err := d.getBits(5)
		if err != nil {
			return 0, err
		}
		v = int64(b)
	case rng <= 64:
		b, err := d.getBits(6)
		if err != nil {
			return 0, err
		}
		v = int64(b)
	case rng <= 128:
		b, err := d.getBits(7)
		if err != nil {
			return 0, err
		}
		v = int64(b)
	case rng <= 256:
		d.align()
		b, err := d.GetBytes(1)
		if err != nil {
			return 0, err
		}
		v = int64(b[0])
	case rng <= 65536:
		d.align()
		b, err := d.GetBytes(2)
		if err != nil {
			return 0, err
		}
		v = int64(binary.BigEndian.Uint16(b))
	default:
		return d.GetUnconstrainedInt()
	}

	return v + lb, nil
}

// GetUnconstrainedInt decodes an unconstrained integer (length + value).
func (d *PERDecoder) GetUnconstrainedInt() (int64, error) {
	d.align()
	lenByte, err := d.GetBytes(1)
	if err != nil {
		return 0, err
	}
	n := int(lenByte[0])
	if n > 8 {
		return 0, fmt.Errorf("unconstrained integer too large: %d bytes", n)
	}
	b, err := d.GetBytes(n)
	if err != nil {
		return 0, err
	}
	var val int64
	for _, x := range b {
		val = (val << 8) | int64(x)
	}
	return val, nil
}

// GetLengthDeterminant decodes a length determinant (X.691 Section 10.9).
func (d *PERDecoder) GetLengthDeterminant() (int, error) {
	d.align()
	if d.off >= len(d.data) {
		return 0, fmt.Errorf("unexpected end of data reading length")
	}

	first := d.data[d.off]
	if first&0x80 == 0 {
		// Short form: 0xxxxxxx
		d.off++
		return int(first & 0x7F), nil
	}

	// Long form: 10xxxxxx xxxxxxxx (14-bit)
	if d.off+1 >= len(d.data) {
		return 0, fmt.Errorf("unexpected end of data reading long length")
	}
	val := binary.BigEndian.Uint16(d.data[d.off:]) & 0x3FFF
	d.off += 2
	return int(val), nil
}

// GetOctetString decodes an OCTET STRING with a length determinant.
func (d *PERDecoder) GetOctetString() ([]byte, error) {
	length, err := d.GetLengthDeterminant()
	if err != nil {
		return nil, err
	}
	return d.GetBytes(length)
}

// GetFixedOctetString decodes a fixed-length OCTET STRING (no length prefix).
func (d *PERDecoder) GetFixedOctetString(n int) ([]byte, error) {
	d.align()
	return d.GetBytes(n)
}

// GetBytes reads n raw bytes from the current position.
func (d *PERDecoder) GetBytes(n int) ([]byte, error) {
	if d.bitOff != 0 {
		d.align()
	}
	if d.off+n > len(d.data) {
		return nil, fmt.Errorf("need %d bytes at offset %d, have %d", n, d.off, len(d.data)-d.off)
	}
	result := make([]byte, n)
	copy(result, d.data[d.off:d.off+n])
	d.off += n
	return result, nil
}

// GetChoiceIndex decodes a CHOICE index.
func (d *PERDecoder) GetChoiceIndex(numChoices int) (int, error) {
	v, err := d.GetConstrainedInt(0, int64(numChoices-1))
	return int(v), err
}

// GetSequenceHeader decodes extension marker and optional bitmap.
func (d *PERDecoder) GetSequenceHeader(extensible bool, numOptional int) (extended bool, optionalBits uint64, err error) {
	if extensible {
		ext, e := d.GetBool()
		if e != nil {
			return false, 0, e
		}
		if ext {
			return true, 0, nil // extension handling not yet implemented
		}
	}

	var bits uint64
	for i := 0; i < numOptional; i++ {
		b, e := d.getBits(1)
		if e != nil {
			return false, 0, e
		}
		if b == 1 {
			bits |= 1 << uint(i)
		}
	}
	return false, bits, nil
}

// DecodePDU decodes a complete S1AP PDU from raw bytes.
func DecodePDU(data []byte) (*PDU, error) {
	dec := NewPERDecoder(data)

	// S1AP-PDU CHOICE (3 alternatives)
	pduType, err := dec.GetChoiceIndex(3)
	if err != nil {
		return nil, fmt.Errorf("decoding PDU type: %w", err)
	}

	// procedureCode: INTEGER (0..255)
	dec.align()
	pcBytes, err := dec.GetBytes(1)
	if err != nil {
		return nil, fmt.Errorf("decoding procedure code: %w", err)
	}

	// criticality: ENUMERATED (0..2)
	crit, err := dec.GetConstrainedInt(0, 2)
	if err != nil {
		return nil, fmt.Errorf("decoding criticality: %w", err)
	}

	// value: OPEN TYPE
	dec.align()
	valueLen, err := dec.GetLengthDeterminant()
	if err != nil {
		return nil, fmt.Errorf("decoding value length: %w", err)
	}

	value, err := dec.GetBytes(valueLen)
	if err != nil {
		return nil, fmt.Errorf("decoding value (%d bytes): %w", valueLen, err)
	}

	return &PDU{
		Type:          PDUType(pduType),
		ProcedureCode: ProcedureCode(pcBytes[0]),
		Criticality:   Criticality(crit),
		Value:         value,
	}, nil
}

// DecodeProtocolIEContainer decodes a list of ProtocolIEs from the value field.
func DecodeProtocolIEContainer(data []byte) ([]ProtocolIE, error) {
	dec := NewPERDecoder(data)

	// Count: 2 bytes
	countBytes, err := dec.GetBytes(2)
	if err != nil {
		return nil, fmt.Errorf("decoding IE count: %w", err)
	}
	count := int(binary.BigEndian.Uint16(countBytes))

	ies := make([]ProtocolIE, 0, count)
	for i := 0; i < count; i++ {
		// id: ProtocolIE-ID (2 bytes)
		idBytes, err := dec.GetBytes(2)
		if err != nil {
			return nil, fmt.Errorf("decoding IE %d id: %w", i, err)
		}
		id := ProtocolIEID(binary.BigEndian.Uint16(idBytes))

		// criticality: ENUMERATED (0..2)
		crit, err := dec.GetConstrainedInt(0, 2)
		if err != nil {
			return nil, fmt.Errorf("decoding IE %d (id=%d) criticality: %w", i, id, err)
		}

		// value: OPEN TYPE
		dec.align()
		valueLen, err := dec.GetLengthDeterminant()
		if err != nil {
			return nil, fmt.Errorf("decoding IE %d (id=%d) value length: %w", i, id, err)
		}
		value, err := dec.GetBytes(valueLen)
		if err != nil {
			return nil, fmt.Errorf("decoding IE %d (id=%d) value (%d bytes): %w", i, id, valueLen, err)
		}

		ies = append(ies, ProtocolIE{
			ID:          id,
			Criticality: Criticality(crit),
			Value:       value,
		})
	}

	return ies, nil
}
