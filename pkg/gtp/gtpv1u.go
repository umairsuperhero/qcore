// Package gtp implements GTPv1-U (3GPP TS 29.281): the user-plane tunneling
// protocol used on the S1-U (eNB ↔ SGW) and S5/S8-U (SGW ↔ PGW) interfaces.
//
// GTP-U carries user IP packets between the eNodeB and the PGW encapsulated
// in UDP/IP. The header identifies the bearer via a 32-bit TEID.
package gtp

import (
	"encoding/binary"
	"errors"
	"fmt"
)

// UDP port for GTP-U (well-known, TS 29.281 §4.4.2.2).
const PortU = 2152

// GTPv1-U message types (TS 29.281 §6.1).
const (
	MsgEchoRequest                        uint8 = 1
	MsgEchoResponse                       uint8 = 2
	MsgErrorIndication                    uint8 = 26
	MsgSupportedExtensionHeadersNotify    uint8 = 31
	MsgEndMarker                          uint8 = 254
	MsgTPDU                               uint8 = 255 // G-PDU (user plane IP)
)

// Flags byte layout (TS 29.281 §5.1):
//
//	bit 7..5: Version     (001 = GTPv1)
//	bit 4:    PT          (1 = GTP, 0 = GTP')
//	bit 3:    reserved    (0)
//	bit 2:    E           (Extension Header present)
//	bit 1:    S           (Sequence number present)
//	bit 0:    PN          (N-PDU number present)
const (
	flagVersion1 = 0x20 // 001 in bits 7..5
	flagPT       = 0x10
	flagE        = 0x04
	flagS        = 0x02
	flagPN       = 0x01
)

// Header is a decoded GTPv1-U header.
type Header struct {
	Flags           uint8
	MessageType     uint8
	Length          uint16 // bytes that follow the mandatory 8-byte header
	TEID            uint32
	Sequence        uint16 // present if Flags&flagS
	NPDUNumber      uint8  // present if Flags&flagPN
	NextExtHdrType  uint8  // present if Flags&flagE
	HeaderLen       int    // total header length including optional fields
}

// Version returns the protocol version (should be 1 for GTPv1-U).
func (h *Header) Version() uint8 { return (h.Flags >> 5) & 0x07 }

// HasExtension returns true if the Extension header flag is set.
func (h *Header) HasExtension() bool { return h.Flags&flagE != 0 }

// HasSequence returns true if a sequence number is present.
func (h *Header) HasSequence() bool { return h.Flags&flagS != 0 }

// Encode serialises a GTPv1-U packet: header + payload (T-PDU bytes).
// If Sequence/NPDU/Extension flags are set in hdr.Flags, the corresponding
// optional bytes must be provided on hdr and are emitted. Otherwise the
// 4-byte optional block is omitted entirely. Length in hdr is recomputed.
func Encode(hdr *Header, payload []byte) ([]byte, error) {
	if hdr == nil {
		return nil, errors.New("nil header")
	}
	// Ensure version + PT are set correctly.
	flags := hdr.Flags | flagVersion1 | flagPT

	hasOpt := flags&(flagE|flagS|flagPN) != 0
	optLen := 0
	if hasOpt {
		optLen = 4
	}

	totalLen := 8 + optLen + len(payload)
	out := make([]byte, totalLen)

	// Length field counts bytes after the mandatory 8-byte header.
	length := optLen + len(payload)
	if length > 0xFFFF {
		return nil, fmt.Errorf("payload too large: %d bytes", length)
	}

	out[0] = flags
	out[1] = hdr.MessageType
	binary.BigEndian.PutUint16(out[2:4], uint16(length))
	binary.BigEndian.PutUint32(out[4:8], hdr.TEID)

	off := 8
	if hasOpt {
		binary.BigEndian.PutUint16(out[off:off+2], hdr.Sequence)
		out[off+2] = hdr.NPDUNumber
		out[off+3] = hdr.NextExtHdrType
		off += 4
	}
	copy(out[off:], payload)
	return out, nil
}

// Decode parses a GTPv1-U packet and returns the header and the inner
// payload (the T-PDU for G-PDU messages). It validates version==1 and
// that the encoded length field agrees with the packet size.
func Decode(buf []byte) (*Header, []byte, error) {
	if len(buf) < 8 {
		return nil, nil, fmt.Errorf("buffer too short for GTP-U header: %d bytes", len(buf))
	}
	hdr := &Header{
		Flags:       buf[0],
		MessageType: buf[1],
		Length:      binary.BigEndian.Uint16(buf[2:4]),
		TEID:        binary.BigEndian.Uint32(buf[4:8]),
	}
	if v := hdr.Version(); v != 1 {
		return nil, nil, fmt.Errorf("unsupported GTP version: %d", v)
	}

	off := 8
	hasOpt := hdr.Flags&(flagE|flagS|flagPN) != 0
	if hasOpt {
		if len(buf) < off+4 {
			return nil, nil, errors.New("buffer too short for GTP-U optional header")
		}
		hdr.Sequence = binary.BigEndian.Uint16(buf[off : off+2])
		hdr.NPDUNumber = buf[off+2]
		hdr.NextExtHdrType = buf[off+3]
		off += 4

		// Walk extension-header chain (TS 29.281 §5.2). Each extension header is
		// [length-in-4-byte-units(1)][content(4*len-2)][next-ext-type(1)].
		// A value of 0 for next-ext-type terminates the chain.
		next := hdr.NextExtHdrType
		for next != 0 {
			if len(buf) < off+1 {
				return nil, nil, errors.New("buffer too short for extension header length")
			}
			extLenUnits := int(buf[off])
			if extLenUnits == 0 {
				return nil, nil, errors.New("invalid extension header length 0")
			}
			extBytes := extLenUnits * 4
			if len(buf) < off+extBytes {
				return nil, nil, errors.New("buffer too short for extension header body")
			}
			next = buf[off+extBytes-1]
			off += extBytes
		}
	}
	hdr.HeaderLen = off

	// Validate declared Length against actual buffer size.
	expectedTotal := 8 + int(hdr.Length)
	if len(buf) < expectedTotal {
		return nil, nil, fmt.Errorf("declared length %d but buffer has %d remaining after mandatory header",
			hdr.Length, len(buf)-8)
	}
	if off > expectedTotal {
		return nil, nil, errors.New("optional/extension headers exceed declared length")
	}
	payload := buf[off:expectedTotal]
	return hdr, payload, nil
}

// EncodeTPDU is a convenience for the common case: encapsulate an IP packet
// for the given downlink TEID with no sequence/extension options.
func EncodeTPDU(teid uint32, ipPacket []byte) ([]byte, error) {
	hdr := &Header{
		MessageType: MsgTPDU,
		TEID:        teid,
	}
	return Encode(hdr, ipPacket)
}

// EncodeEchoRequest builds an Echo Request (TS 29.281 §7.2.1). TEID is zero for
// the Echo Request/Response path and a Recovery IE should be appended.
func EncodeEchoRequest(seq uint16) ([]byte, error) {
	hdr := &Header{
		Flags:       flagS,
		MessageType: MsgEchoRequest,
		TEID:        0,
		Sequence:    seq,
	}
	// Recovery IE (TV): type=14, value=0 (restart counter)
	recovery := []byte{14, 0}
	return Encode(hdr, recovery)
}
