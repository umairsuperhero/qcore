package pfcp

import (
	"encoding/binary"
	"errors"
	"net"
	"time"
)

// IE Types (TS 29.244 §8.1.2)
const (
	IETypeCause                = 19
	IETypeNodeID               = 60
	IETypeRecoveryTimeStamp    = 96
	IETypeCPFunctionFeatures   = 89
	IETypeUPFunctionFeatures   = 43
	IETypeFSEID                = 57
	IETypePDRID                = 56
	IETypeFARID                = 108
	IETypeURRID                = 81
	IETypeQERID                = 109
	IETypeOuterHeaderCreation  = 84
	IETypeNetworkInstance      = 22
	IETypeSDFFilter            = 74
	IETypeApplicationID        = 24
	IETypeGateStatus           = 25
	IETypeMBR                  = 26
	IETypeGBR                  = 27
	IETypeQCI                  = 28
)

// Cause values (TS 29.244 §8.2.1)
const (
	CauseRequestAccepted = 1
	CauseRequestRejected = 64
)

var (
	ErrMalformedIE = errors.New("pfcp: malformed IE")
)

// IE represents a generic PFCP Information Element.
type IE struct {
	Type   uint16
	Length uint16
	Value  []byte
}

// Encode formats the IE to bytes.
func (ie *IE) Encode() []byte {
	b := make([]byte, 4+len(ie.Value))
	binary.BigEndian.PutUint16(b[0:2], ie.Type)
	binary.BigEndian.PutUint16(b[2:4], uint16(len(ie.Value)))
	copy(b[4:], ie.Value)
	return b
}

// DecodeIEs parses a byte slice into a slice of IEs.
func DecodeIEs(b []byte) ([]IE, error) {
	var ies []IE
	offset := 0
	for offset+4 <= len(b) {
		t := binary.BigEndian.Uint16(b[offset : offset+2])
		l := binary.BigEndian.Uint16(b[offset+2 : offset+4])
		if offset+4+int(l) > len(b) {
			return nil, ErrMalformedIE
		}
		ies = append(ies, IE{
			Type:   t,
			Length: l,
			Value:  b[offset+4 : offset+4+int(l)],
		})
		offset += 4 + int(l)
	}
	return ies, nil
}

// --- Specific IE Encoders ---

// NewRecoveryTimeStamp creates a Recovery Time Stamp IE.
// It is encoded as an NTP timestamp (seconds since 1900-01-01).
func NewRecoveryTimeStamp(t time.Time) IE {
	// NTP epoch is Jan 1 1900. Unix epoch is Jan 1 1970.
	// Difference is 2208988800 seconds.
	ntpSecs := uint32(t.Unix() + 2208988800)
	b := make([]byte, 4)
	binary.BigEndian.PutUint32(b, ntpSecs)
	return IE{Type: IETypeRecoveryTimeStamp, Value: b}
}

// ParseRecoveryTimeStamp parses a Recovery Time Stamp IE.
func ParseRecoveryTimeStamp(ie IE) (time.Time, error) {
	if ie.Type != IETypeRecoveryTimeStamp || len(ie.Value) != 4 {
		return time.Time{}, ErrMalformedIE
	}
	ntpSecs := binary.BigEndian.Uint32(ie.Value)
	return time.Unix(int64(ntpSecs)-2208988800, 0), nil
}

// NewNodeID creates a Node ID IE.
// NodeIDType: 0=IPv4, 1=IPv6, 2=FQDN.
func NewNodeID(idType uint8, ip net.IP, fqdn string) IE {
	var b []byte
	switch idType {
	case NodeIDTypeIPv4:
		b = make([]byte, 5)
		b[0] = idType
		copy(b[1:], ip.To4())
	case NodeIDTypeIPv6:
		b = make([]byte, 17)
		b[0] = idType
		copy(b[1:], ip.To16())
	case NodeIDTypeFQDN:
		b = make([]byte, 1+len(fqdn))
		b[0] = idType
		copy(b[1:], []byte(fqdn))
	}
	return IE{Type: IETypeNodeID, Value: b}
}

// ParseNodeID parses a Node ID IE, returning (type, ip, fqdn).
func ParseNodeID(ie IE) (uint8, net.IP, string, error) {
	if ie.Type != IETypeNodeID || len(ie.Value) < 1 {
		return 0, nil, "", ErrMalformedIE
	}
	idType := ie.Value[0]
	switch idType {
	case NodeIDTypeIPv4:
		if len(ie.Value) != 5 {
			return 0, nil, "", ErrMalformedIE
		}
		return idType, net.IP(ie.Value[1:5]), "", nil
	case NodeIDTypeIPv6:
		if len(ie.Value) != 17 {
			return 0, nil, "", ErrMalformedIE
		}
		return idType, net.IP(ie.Value[1:17]), "", nil
	case NodeIDTypeFQDN:
		return idType, nil, string(ie.Value[1:]), nil
	default:
		return 0, nil, "", ErrMalformedIE
	}
}

// NewCause creates a Cause IE.
func NewCause(cause uint8) IE {
	return IE{Type: IETypeCause, Value: []byte{cause}}
}

// ParseCause parses a Cause IE.
func ParseCause(ie IE) (uint8, error) {
	if ie.Type != IETypeCause || len(ie.Value) != 1 {
		return 0, ErrMalformedIE
	}
	return ie.Value[0], nil
}

// NewUPFunctionFeatures creates a UP Function Features IE (flags).
func NewUPFunctionFeatures(flags uint16) IE {
	b := make([]byte, 2)
	binary.BigEndian.PutUint16(b, flags)
	return IE{Type: IETypeUPFunctionFeatures, Value: b}
}

// NewCPFunctionFeatures creates a CP Function Features IE (flags).
func NewCPFunctionFeatures(flags uint8) IE {
	return IE{Type: IETypeCPFunctionFeatures, Value: []byte{flags}}
}
