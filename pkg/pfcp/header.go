package pfcp

import (
	"encoding/binary"
	"errors"
)

// PFCP Message Types (TS 29.244 §7.3)
const (
	MsgTypeHeartbeatRequest            = 1
	MsgTypeHeartbeatResponse           = 2
	MsgTypeAssociationSetupRequest     = 5
	MsgTypeAssociationSetupResponse    = 6
	MsgTypeSessionEstablishmentRequest = 50
	MsgTypeSessionEstablishmentResponse= 51
	MsgTypeSessionModificationRequest  = 52
	MsgTypeSessionModificationResponse = 53
	MsgTypeSessionDeletionRequest      = 54
	MsgTypeSessionDeletionResponse     = 55
)

var (
	ErrBufferTooSmall = errors.New("pfcp: buffer too small")
	ErrInvalidVersion = errors.New("pfcp: invalid version")
)

// Header represents a PFCP message header (TS 29.244 §7.2).
type Header struct {
	Version         uint8
	FollowOn        bool
	MessagePriority bool
	SEIDPresent     bool
	MessageType     uint8
	MessageLength   uint16
	SEID            uint64
	SequenceNumber  uint32 // 24-bit on the wire
	Priority        uint8  // 4-bit on the wire
}

// NodeID type values (TS 29.244 §8.2.38)
const (
	NodeIDTypeIPv4 = 0
	NodeIDTypeIPv6 = 1
	NodeIDTypeFQDN = 2
)

// DecodeHeader decodes a PFCP header from a byte slice.
func DecodeHeader(b []byte) (Header, int, error) {
	if len(b) < 8 {
		return Header{}, 0, ErrBufferTooSmall
	}

	h := Header{}
	
	// Octet 1: Version (3), Spare (2), FO (1), MP (1), S (1)
	h.Version = (b[0] >> 5) & 0x07
	if h.Version != 1 {
		return h, 0, ErrInvalidVersion
	}
	
	h.FollowOn = (b[0] & 0x04) != 0
	h.MessagePriority = (b[0] & 0x02) != 0
	h.SEIDPresent = (b[0] & 0x01) != 0
	
	// Octet 2: Message Type
	h.MessageType = b[1]
	
	// Octet 3-4: Message Length
	h.MessageLength = binary.BigEndian.Uint16(b[2:4])
	
	offset := 4
	if h.SEIDPresent {
		if len(b) < 16 {
			return h, 0, ErrBufferTooSmall
		}
		h.SEID = binary.BigEndian.Uint64(b[4:12])
		offset = 12
	}
	
	// Sequence Number (24 bits)
	if len(b) < offset+4 {
		return h, 0, ErrBufferTooSmall
	}
	
	h.SequenceNumber = uint32(b[offset])<<16 | uint32(b[offset+1])<<8 | uint32(b[offset+2])
	
	// Spare (4) + Message Priority (4)
	if h.MessagePriority {
		h.Priority = b[offset+3] & 0x0F
	}
	
	return h, offset + 4, nil
}

// EncodeHeader encodes a PFCP header to a byte slice.
func EncodeHeader(h Header) []byte {
	// Base size is 8 without SEID, 16 with SEID
	size := 8
	if h.SEIDPresent {
		size = 16
	}
	
	b := make([]byte, size)
	
	// Octet 1
	b[0] = (h.Version & 0x07) << 5
	if h.FollowOn {
		b[0] |= 0x04
	}
	if h.MessagePriority {
		b[0] |= 0x02
	}
	if h.SEIDPresent {
		b[0] |= 0x01
	}
	
	// Octet 2
	b[1] = h.MessageType
	
	// Octet 3-4 (Length of payload + optional fields)
	// We assume MessageLength is correctly pre-calculated by the caller.
	// Typically: payload length + 4 (seq) + 8 (if SEID)
	binary.BigEndian.PutUint16(b[2:4], h.MessageLength)
	
	offset := 4
	if h.SEIDPresent {
		binary.BigEndian.PutUint64(b[4:12], h.SEID)
		offset = 12
	}
	
	// Octet 5-7 or 13-15: Sequence Number (24 bits)
	b[offset] = byte(h.SequenceNumber >> 16)
	b[offset+1] = byte(h.SequenceNumber >> 8)
	b[offset+2] = byte(h.SequenceNumber)
	
	// Octet 8 or 16: Message Priority
	if h.MessagePriority {
		b[offset+3] = h.Priority & 0x0F
	}
	
	return b
}
