package s1ap

import (
	"bytes"
	"net"
	"testing"
)

func TestERABSetupResultRoundTrip(t *testing.T) {
	in := &InitialContextSetupResponse{
		MMEUES1APID: 42,
		ENBUES1APID: 7,
		ERABs: []ERABSetupResult{{
			ERABID:             5,
			TransportLayerAddr: net.IPv4(127, 0, 0, 1),
			GTPTEID:            [4]byte{0x11, 0x22, 0x33, 0x44},
		}},
	}
	encoded, err := EncodeInitialContextSetupResponse(in)
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	pdu, err := DecodePDU(encoded)
	if err != nil {
		t.Fatalf("decode PDU: %v", err)
	}
	if pdu.ProcedureCode != ProcInitialContextSetup {
		t.Fatalf("procedure code mismatch: got %v", pdu.ProcedureCode)
	}
	ies, err := DecodeProtocolIEContainer(pdu.Value)
	if err != nil {
		t.Fatalf("decode IEs: %v", err)
	}
	out, err := DecodeInitialContextSetupResponse(ies)
	if err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if out.MMEUES1APID != in.MMEUES1APID || out.ENBUES1APID != in.ENBUES1APID {
		t.Fatalf("id mismatch: %+v", out)
	}
	if len(out.ERABs) != 1 {
		t.Fatalf("expected 1 ERAB, got %d", len(out.ERABs))
	}
	g := out.ERABs[0]
	if g.ERABID != 5 {
		t.Fatalf("ERABID mismatch: %d", g.ERABID)
	}
	if g.TransportLayerAddr == nil || !g.TransportLayerAddr.Equal(net.IPv4(127, 0, 0, 1)) {
		t.Fatalf("addr mismatch: %v", g.TransportLayerAddr)
	}
	if !bytes.Equal(g.GTPTEID[:], []byte{0x11, 0x22, 0x33, 0x44}) {
		t.Fatalf("TEID mismatch: %x", g.GTPTEID)
	}
}
