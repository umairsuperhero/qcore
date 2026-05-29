package pfcp

// Message represents a generic PFCP message (Header + IEs).
type Message struct {
	Header Header
	IEs    []IE
}

// Encode serializes the entire PFCP message.
func (m *Message) Encode() []byte {
	var payload []byte
	for _, ie := range m.IEs {
		payload = append(payload, ie.Encode()...)
	}
	
	m.Header.MessageLength = uint16(len(payload))
	// If SEID is present, header has 8 extra bytes. Sequence number is always 4 bytes.
	if m.Header.SEIDPresent {
		m.Header.MessageLength += 12
	} else {
		m.Header.MessageLength += 4
	}
	
	b := EncodeHeader(m.Header)
	b = append(b, payload...)
	return b
}

// DecodeMessage parses a full PFCP message.
func DecodeMessage(b []byte) (*Message, error) {
	h, offset, err := DecodeHeader(b)
	if err != nil {
		return nil, err
	}
	
	expectedPayloadLen := int(h.MessageLength)
	if h.SEIDPresent {
		expectedPayloadLen -= 12
	} else {
		expectedPayloadLen -= 4
	}
	
	if len(b[offset:]) < expectedPayloadLen {
		return nil, ErrBufferTooSmall
	}
	
	ies, err := DecodeIEs(b[offset : offset+expectedPayloadLen])
	if err != nil {
		return nil, err
	}
	
	return &Message{
		Header: h,
		IEs:    ies,
	}, nil
}

// --- High-level Builders ---

// NewHeartbeatRequest builds a Heartbeat Request message (TS 29.244 §7.4.2).
func NewHeartbeatRequest(seq uint32, ts IE) *Message {
	return &Message{
		Header: Header{
			Version:        1,
			MessageType:    MsgTypeHeartbeatRequest,
			SequenceNumber: seq,
		},
		IEs: []IE{ts},
	}
}

// NewHeartbeatResponse builds a Heartbeat Response message (TS 29.244 §7.4.3).
func NewHeartbeatResponse(seq uint32, ts IE) *Message {
	return &Message{
		Header: Header{
			Version:        1,
			MessageType:    MsgTypeHeartbeatResponse,
			SequenceNumber: seq,
		},
		IEs: []IE{ts},
	}
}

// NewAssociationSetupRequest builds an Association Setup Request (TS 29.244 §7.4.4).
func NewAssociationSetupRequest(seq uint32, nodeID IE, ts IE) *Message {
	// Usually also includes CP/UP Function Features depending on sender.
	return &Message{
		Header: Header{
			Version:        1,
			MessageType:    MsgTypeAssociationSetupRequest,
			SequenceNumber: seq,
		},
		IEs: []IE{nodeID, ts},
	}
}

// NewAssociationSetupResponse builds an Association Setup Response (TS 29.244 §7.4.5).
func NewAssociationSetupResponse(seq uint32, nodeID IE, cause IE, ts IE) *Message {
	return &Message{
		Header: Header{
			Version:        1,
			MessageType:    MsgTypeAssociationSetupResponse,
			SequenceNumber: seq,
		},
		IEs: []IE{nodeID, cause, ts},
	}
}
