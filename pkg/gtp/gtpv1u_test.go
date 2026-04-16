package gtp

import (
	"bytes"
	"testing"
)

func TestEncodeDecodeTPDU(t *testing.T) {
	// A minimal IPv4 "ping"-like payload. GTP-U doesn't care about the contents,
	// it's just octets to us; we're exercising the header round-trip.
	inner := []byte{0x45, 0x00, 0x00, 0x1c, 0xde, 0xad, 0x00, 0x00, 0x40, 0x01,
		0x00, 0x00, 0x0a, 0x2d, 0x00, 0x02, 0x08, 0x08, 0x08, 0x08,
		0x08, 0x00, 0xf7, 0xff, 0x00, 0x01, 0x00, 0x01}

	encoded, err := EncodeTPDU(0xDEADBEEF, inner)
	if err != nil {
		t.Fatalf("EncodeTPDU: %v", err)
	}
	if len(encoded) != 8+len(inner) {
		t.Fatalf("expected %d encoded bytes, got %d", 8+len(inner), len(encoded))
	}

	hdr, payload, err := Decode(encoded)
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}
	if hdr.Version() != 1 {
		t.Fatalf("expected version 1, got %d", hdr.Version())
	}
	if hdr.MessageType != MsgTPDU {
		t.Fatalf("expected T-PDU (%d), got %d", MsgTPDU, hdr.MessageType)
	}
	if hdr.TEID != 0xDEADBEEF {
		t.Fatalf("TEID mismatch: got 0x%x", hdr.TEID)
	}
	if !bytes.Equal(payload, inner) {
		t.Fatalf("payload round-trip mismatch")
	}
}

func TestEncodeDecodeWithSequence(t *testing.T) {
	hdr := &Header{
		Flags:       flagS,
		MessageType: MsgTPDU,
		TEID:        42,
		Sequence:    0x1234,
	}
	payload := []byte{0xaa, 0xbb, 0xcc}
	encoded, err := Encode(hdr, payload)
	if err != nil {
		t.Fatalf("Encode: %v", err)
	}

	dec, got, err := Decode(encoded)
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}
	if !dec.HasSequence() {
		t.Fatalf("sequence flag not preserved")
	}
	if dec.Sequence != 0x1234 {
		t.Fatalf("sequence mismatch: got 0x%x", dec.Sequence)
	}
	if !bytes.Equal(got, payload) {
		t.Fatalf("payload mismatch: got %x", got)
	}
}

func TestDecodeRejectsShortBuffer(t *testing.T) {
	if _, _, err := Decode([]byte{1, 2, 3}); err == nil {
		t.Fatalf("expected error for short buffer")
	}
}

func TestDecodeRejectsWrongVersion(t *testing.T) {
	// A GTPv2 header has version=010 in bits 7..5 → 0x40 base
	buf := []byte{0x40, 1, 0, 0, 0, 0, 0, 0}
	if _, _, err := Decode(buf); err == nil {
		t.Fatalf("expected error for GTPv2 header")
	}
}

func TestEncodeEchoRequest(t *testing.T) {
	buf, err := EncodeEchoRequest(0x0100)
	if err != nil {
		t.Fatalf("EncodeEchoRequest: %v", err)
	}
	hdr, body, err := Decode(buf)
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}
	if hdr.MessageType != MsgEchoRequest {
		t.Fatalf("expected Echo Request, got %d", hdr.MessageType)
	}
	if !hdr.HasSequence() || hdr.Sequence != 0x0100 {
		t.Fatalf("sequence not preserved: %+v", hdr)
	}
	if len(body) != 2 || body[0] != 14 {
		t.Fatalf("Recovery IE missing: %x", body)
	}
}
