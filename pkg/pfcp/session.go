package pfcp

import (
	"encoding/binary"
	"net"
)

// --- Session IEs ---

// NewFSEID creates a Fully Qualified SEID IE.
// Flags (Octet 5): bit 2 = IPv4, bit 1 = IPv6.
func NewFSEID(seid uint64, ip4 net.IP, ip6 net.IP) IE {
	var b []byte
	flags := byte(0)

	if ip4 != nil {
		flags |= 0x02
	}
	if ip6 != nil {
		flags |= 0x01
	}

	b = make([]byte, 9) // 1 (flags) + 8 (seid)
	b[0] = flags
	binary.BigEndian.PutUint64(b[1:9], seid)

	if ip4 != nil {
		b = append(b, ip4.To4()...)
	}
	if ip6 != nil {
		b = append(b, ip6.To16()...)
	}

	return IE{Type: IETypeFSEID, Value: b}
}

func ParseFSEID(ie IE) (uint64, net.IP, error) {
	if ie.Type != IETypeFSEID || len(ie.Value) < 9 {
		return 0, nil, ErrMalformedIE
	}
	seid := binary.BigEndian.Uint64(ie.Value[1:9])
	if ie.Value[0]&0x02 == 0 || len(ie.Value) < 13 {
		return seid, nil, nil
	}
	return seid, net.IPv4(ie.Value[9], ie.Value[10], ie.Value[11], ie.Value[12]), nil
}

// --- Session Message Builders ---

// NewSessionEstablishmentRequest builds a Session Establishment Request.
// Note: It is sent with a SEID=0 because the session isn't established yet on the receiver side.
func NewSessionEstablishmentRequest(seq uint32, nodeID IE, fseid IE, additionalIEs ...IE) *Message {
	ies := []IE{nodeID, fseid}
	ies = append(ies, additionalIEs...)

	return &Message{
		Header: Header{
			Version:        1,
			SEIDPresent:    true,
			MessageType:    MsgTypeSessionEstablishmentRequest,
			SEID:           0, // TS 29.244 §7.2.2.4: 0 for establishment request
			SequenceNumber: seq,
		},
		IEs: ies,
	}
}

// NewSessionEstablishmentResponse builds a Session Establishment Response.
// Sent with the SEID provided by the CP function in the request.
func NewSessionEstablishmentResponse(seq uint32, seid uint64, nodeID IE, cause IE, upFSEID IE, additionalIEs ...IE) *Message {
	ies := []IE{nodeID, cause}
	if upFSEID.Type == IETypeFSEID {
		ies = append(ies, upFSEID)
	}
	ies = append(ies, additionalIEs...)

	return &Message{
		Header: Header{
			Version:        1,
			SEIDPresent:    true,
			MessageType:    MsgTypeSessionEstablishmentResponse,
			SEID:           seid,
			SequenceNumber: seq,
		},
		IEs: ies,
	}
}

// NewSessionModificationRequest builds a Session Modification Request.
func NewSessionModificationRequest(seq uint32, seid uint64, additionalIEs ...IE) *Message {
	return &Message{
		Header: Header{
			Version:        1,
			SEIDPresent:    true,
			MessageType:    MsgTypeSessionModificationRequest,
			SEID:           seid,
			SequenceNumber: seq,
		},
		IEs: additionalIEs,
	}
}

// NewSessionModificationResponse builds a Session Modification Response.
func NewSessionModificationResponse(seq uint32, seid uint64, cause IE, additionalIEs ...IE) *Message {
	ies := []IE{cause}
	ies = append(ies, additionalIEs...)

	return &Message{
		Header: Header{
			Version:        1,
			SEIDPresent:    true,
			MessageType:    MsgTypeSessionModificationResponse,
			SEID:           seid,
			SequenceNumber: seq,
		},
		IEs: ies,
	}
}

// NewSessionDeletionRequest builds a Session Deletion Request.
func NewSessionDeletionRequest(seq uint32, seid uint64) *Message {
	return &Message{
		Header: Header{
			Version:        1,
			SEIDPresent:    true,
			MessageType:    MsgTypeSessionDeletionRequest,
			SEID:           seid,
			SequenceNumber: seq,
		},
	}
}

// NewSessionDeletionResponse builds a Session Deletion Response.
func NewSessionDeletionResponse(seq uint32, seid uint64, cause IE) *Message {
	return &Message{
		Header: Header{
			Version:        1,
			SEIDPresent:    true,
			MessageType:    MsgTypeSessionDeletionResponse,
			SEID:           seid,
			SequenceNumber: seq,
		},
		IEs: []IE{cause},
	}
}
