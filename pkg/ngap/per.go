package ngap

import (
	"encoding/binary"
	"fmt"
)

// PERDecoder handles ASN.1 ALIGNED PER decoding (ITU-T X.691).
type PERDecoder struct {
	data   []byte
	off    int
	bitOff uint
}

func NewPERDecoder(data []byte) *PERDecoder { return &PERDecoder{data: data} }

func (d *PERDecoder) Remaining() int {
	r := len(d.data) - d.off
	if d.bitOff > 0 {
		r--
	}
	if r < 0 {
		return 0
	}
	return r
}

func (d *PERDecoder) align() {
	if d.bitOff != 0 {
		d.off++
		d.bitOff = 0
	}
}

func (d *PERDecoder) Align() { d.align() }

func (d *PERDecoder) GetBits(n uint) (uint8, error) { return d.getBits(n) }

func (d *PERDecoder) getBits(n uint) (uint8, error) {
	if n == 0 || n > 8 {
		return 0, fmt.Errorf("invalid bit count: %d", n)
	}
	if d.off >= len(d.data) {
		return 0, fmt.Errorf("unexpected end of data at byte %d (need %d bits)", d.off, n)
	}
	room := 8 - d.bitOff
	if n <= room {
		shift := room - n
		val := (d.data[d.off] >> shift) & ((1 << n) - 1)
		d.bitOff += n
		if d.bitOff == 8 {
			d.off++
			d.bitOff = 0
		}
		return val, nil
	}
	hi := d.data[d.off] & ((1 << room) - 1)
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

func (d *PERDecoder) GetBool() (bool, error) {
	v, err := d.getBits(1)
	return v == 1, err
}

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
	case rng <= 255:
		b, err := d.getBits(bitsNeeded(uint64(rng)))
		if err != nil {
			return 0, err
		}
		v = int64(b)
	case rng == 256:
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

// bitsNeeded returns the minimum number of bits required to represent values 0..(range-1).
func bitsNeeded(rng uint64) uint {
	if rng <= 1 {
		return 0
	}
	r := rng - 1
	var bits uint
	for r > 0 {
		bits++
		r >>= 1
	}
	return bits
}

// GetUnconstrainedInt decodes a length-prefixed unconstrained integer (up to 8 bytes).
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

func (d *PERDecoder) GetLengthDeterminant() (int, error) {
	d.align()
	if d.off >= len(d.data) {
		return 0, fmt.Errorf("unexpected end of data reading length")
	}
	first := d.data[d.off]
	if first&0x80 == 0 {
		d.off++
		return int(first & 0x7F), nil
	}
	if d.off+1 >= len(d.data) {
		return 0, fmt.Errorf("unexpected end of data reading long length")
	}
	val := binary.BigEndian.Uint16(d.data[d.off:]) & 0x3FFF
	d.off += 2
	return int(val), nil
}

// GetOctetString decodes an unconstrained OCTET STRING.
func (d *PERDecoder) GetOctetString() ([]byte, error) {
	d.align()
	l, err := d.GetLengthDeterminant()
	if err != nil {
		return nil, err
	}
	return d.GetBytes(int(l))
}

// GetPrintableString decodes a PrintableString with a known size constraint.
func (d *PERDecoder) GetPrintableString(lb, ub int, ext bool) (string, error) {
	isExt := false
	var err error
	if ext {
		v, e := d.getBits(1)
		if e != nil {
			return "", e
		}
		isExt = (v == 1)
	}
	var length int
	if !isExt {
		if ub >= lb {
			// constrained length
			l, err := d.GetConstrainedInt(int64(lb), int64(ub))
			if err != nil {
				return "", err
			}
			length = int(l)
		} else {
			d.align()
			length, err = d.GetLengthDeterminant()
			if err != nil {
				return "", err
			}
		}
	} else {
		// unconstrained length
		d.align()
		length, err = d.GetLengthDeterminant()
		if err != nil {
			return "", err
		}
	}

	// Characters are 8 bits each, so align before reading
	d.align()
	b, err := d.GetBytes(length)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

func (d *PERDecoder) GetFixedOctetString(n int) ([]byte, error) {
	if n <= 2 {
		b := make([]byte, n)
		for i := 0; i < n; i++ {
			val, err := d.getBits(8)
			if err != nil {
				return nil, err
			}
			b[i] = uint8(val)
		}
		return b, nil
	}
	d.align()
	return d.GetBytes(n)
}

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

func (d *PERDecoder) GetChoiceIndex(numChoices int, ext bool) (int, error) {
	if ext {
		extended, err := d.GetBool()
		if err != nil {
			return 0, err
		}
		if extended {
			return 0, fmt.Errorf("extended choice not supported")
		}
	}
	v, err := d.GetConstrainedInt(0, int64(numChoices-1))
	return int(v), err
}

func (d *PERDecoder) GetSequenceHeader(extensible bool, numOptional int) (extended bool, optionalBits uint64, err error) {
	if extensible {
		ext, e := d.GetBool()
		if e != nil {
			return false, 0, e
		}
		if ext {
			return true, 0, nil
		}
	}
	var bits uint64
	for i := 0; i < numOptional; i++ {
		b, e := d.getBits(1)
		if e != nil {
			return false, 0, e
		}
		if b == 1 {
			bits |= 1 << uint(numOptional-1-i)
		}
	}
	return false, bits, nil
}

// PEREncoder handles ASN.1 ALIGNED PER encoding (ITU-T X.691).
type PEREncoder struct {
	buf    []byte
	bitOff uint
}

func NewPEREncoder() *PEREncoder {
	return &PEREncoder{buf: make([]byte, 0, 256)}
}

func (e *PEREncoder) Bytes() []byte { return e.buf }

func (e *PEREncoder) align() {
	if e.bitOff != 0 {
		e.bitOff = 0
	}
}

func (e *PEREncoder) putBits(val uint8, n uint) {
	if n == 0 || n > 8 {
		return
	}
	val &= (1 << n) - 1
	if e.bitOff == 0 {
		e.buf = append(e.buf, 0)
	}
	room := 8 - e.bitOff
	if n <= room {
		e.buf[len(e.buf)-1] |= val << (room - n)
		e.bitOff += n
		if e.bitOff == 8 {
			e.bitOff = 0
		}
	} else {
		hi := val >> (n - room)
		e.buf[len(e.buf)-1] |= hi
		e.bitOff = 0
		remaining := n - room
		e.buf = append(e.buf, 0)
		e.buf[len(e.buf)-1] = val << (8 - remaining)
		e.bitOff = remaining
	}
}

func (e *PEREncoder) PutBool(v bool) {
	if v {
		e.putBits(1, 1)
	} else {
		e.putBits(0, 1)
	}
}

func (e *PEREncoder) PutConstrainedInt(val, lb, ub int64) error {
	if val < lb || val > ub {
		return fmt.Errorf("value %d out of range [%d, %d]", val, lb, ub)
	}
	rng := ub - lb + 1
	v := val - lb
	switch {
	case rng == 1:
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
	case rng <= 255:
		e.putBits(uint8(v), bitsNeeded(uint64(rng)))
	case rng == 256:
		e.align()
		e.PutBytes([]byte{uint8(v)})
	case rng <= 65536:
		e.align()
		b := make([]byte, 2)
		binary.BigEndian.PutUint16(b, uint16(v))
		e.PutBytes(b)
	default:
		return e.PutUnconstrainedInt(v)
	}
	return nil
}

// PutUnconstrainedInt encodes a non-negative integer with a 1-byte length prefix.
// Handles values up to 5 bytes (covers AMF-UE-NGAP-ID range 0..1099511627775).
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
	case v <= 0xFFFFFF:
		b := make([]byte, 4)
		b[0] = 3
		b[1] = uint8(v >> 16)
		b[2] = uint8(v >> 8)
		b[3] = uint8(v)
		e.PutBytes(b)
	case v <= 0xFFFFFFFF:
		b := make([]byte, 5)
		b[0] = 4
		binary.BigEndian.PutUint32(b[1:], uint32(v))
		e.PutBytes(b)
	case v <= 0xFFFFFFFFFF:
		b := make([]byte, 6)
		b[0] = 5
		b[1] = uint8(v >> 32)
		binary.BigEndian.PutUint32(b[2:], uint32(v))
		e.PutBytes(b)
	default:
		return fmt.Errorf("integer too large: %d", v)
	}
	return nil
}

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

func (e *PEREncoder) PutOctetString(data []byte) error {
	e.align()
	if err := e.PutLengthDeterminant(len(data)); err != nil {
		return err
	}
	e.PutBytes(data)
	return nil
}

// PutPrintableString encodes a PrintableString with a known size constraint.
func (e *PEREncoder) PutPrintableString(s string, lb, ub int, ext bool) error {
	b := []byte(s)
	l := len(b)

	if ext {
		isExt := l < lb || l > ub
		isExtBit := uint8(0)
		if isExt {
			isExtBit = 1
		}
		e.putBits(isExtBit, 1)
		if isExt {
			e.align()
			if err := e.PutLengthDeterminant(l); err != nil {
				return err
			}
		} else {
			if err := e.PutConstrainedInt(int64(l), int64(lb), int64(ub)); err != nil {
				return err
			}
		}
	} else {
		if ub >= lb {
			if err := e.PutConstrainedInt(int64(l), int64(lb), int64(ub)); err != nil {
				return err
			}
		} else {
			e.align()
			if err := e.PutLengthDeterminant(l); err != nil {
				return err
			}
		}
	}

	e.align()
	e.PutBytes(b)
	return nil
}

func (e *PEREncoder) PutFixedOctetString(data []byte) {
	n := len(data)
	if n <= 2 {
		for i := 0; i < n; i++ {
			e.putBits(data[i], 8)
		}
		return
	}
	e.align()
	e.PutBytes(data)
}

// PutBitString encodes a fixed-length BIT STRING. For BIT STRINGs > 16 bits,
// this aligns to a byte boundary first per X.691 §15.4.
func (e *PEREncoder) PutBitString(data []byte, numBits int) {
	if numBits > 16 {
		e.align()
	}
	e.PutBytes(data)
}

func (e *PEREncoder) PutBytes(data []byte) {
	if e.bitOff != 0 {
		e.align()
	}
	e.buf = append(e.buf, data...)
}

func (e *PEREncoder) PutChoiceIndex(index, numChoices int, ext bool) error {
	if index < 0 || index >= numChoices {
		return fmt.Errorf("choice index %d out of range [0, %d)", index, numChoices)
	}
	if ext {
		e.putBits(0, 1) // not extended
	}
	return e.PutConstrainedInt(int64(index), 0, int64(numChoices-1))
}

func (e *PEREncoder) PutSequenceHeader(extensible bool, optionalBits uint64, numOptional int) {
	if extensible {
		e.putBits(0, 1)
	}
	for i := 0; i < numOptional; i++ {
		if optionalBits&(1<<uint(numOptional-1-i)) != 0 {
			e.putBits(1, 1)
		} else {
			e.putBits(0, 1)
		}
	}
}
