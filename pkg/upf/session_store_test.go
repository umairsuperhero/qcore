package upf

import (
	"net"
	"testing"
)

func TestUpdateRemoteTunnel(t *testing.T) {
	store := NewSessionStore()
	sess := &Session{
		SEID:      1001,
		LocalTEID: 2001,
		UEIP:      net.IPv4(10, 45, 0, 2),
	}
	store.AddSession(sess)

	if err := store.UpdateRemoteTunnel(1001, 3001, net.IPv4(172, 18, 0, 14)); err != nil {
		t.Fatalf("UpdateRemoteTunnel failed: %v", err)
	}
	got, err := store.GetByUEIP(net.IPv4(10, 45, 0, 2))
	if err != nil {
		t.Fatalf("GetByUEIP failed: %v", err)
	}
	if got.RemoteTEID != 3001 || !got.RemoteIP.Equal(net.IPv4(172, 18, 0, 14)) {
		t.Fatalf("remote tunnel not updated: teid=%d ip=%v", got.RemoteTEID, got.RemoteIP)
	}
}
