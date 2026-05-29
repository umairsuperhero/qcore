package pfcp

import (
	"bytes"
	"net"
	"testing"
	"time"
)

func TestHighLevelMessages(t *testing.T) {
	t.Run("Heartbeat Request", func(t *testing.T) {
		ts := NewRecoveryTimeStamp(time.Unix(0, 0)) // Using 0 for stable test value
		req := NewHeartbeatRequest(1, ts)
		
		b := req.Encode()
		
		msg, err := DecodeMessage(b)
		if err != nil {
			t.Fatalf("DecodeMessage failed: %v", err)
		}
		
		if msg.Header.MessageType != MsgTypeHeartbeatRequest {
			t.Errorf("Expected MsgTypeHeartbeatRequest, got %d", msg.Header.MessageType)
		}
		if msg.Header.SEIDPresent {
			t.Error("Heartbeat should not have SEID")
		}
		if len(msg.IEs) != 1 {
			t.Errorf("Expected 1 IE, got %d", len(msg.IEs))
		}
		
		parsedTs, err := ParseRecoveryTimeStamp(msg.IEs[0])
		if err != nil {
			t.Fatalf("Failed to parse ts: %v", err)
		}
		if parsedTs.Unix() != 0 {
			t.Errorf("Expected Unix 0, got %d", parsedTs.Unix())
		}
	})

	t.Run("Session Establishment Request", func(t *testing.T) {
		nodeID := NewNodeID(NodeIDTypeIPv4, net.ParseIP("1.2.3.4"), "")
		fseid := NewFSEID(0x1234567890abcdef, net.ParseIP("1.2.3.4"), nil)
		
		req := NewSessionEstablishmentRequest(2, nodeID, fseid)
		
		b := req.Encode()
		
		msg, err := DecodeMessage(b)
		if err != nil {
			t.Fatalf("DecodeMessage failed: %v", err)
		}
		
		if msg.Header.MessageType != MsgTypeSessionEstablishmentRequest {
			t.Errorf("Expected MsgTypeSessionEstablishmentRequest, got %d", msg.Header.MessageType)
		}
		if !msg.Header.SEIDPresent {
			t.Error("Session Estab Req should have SEID")
		}
		if msg.Header.SEID != 0 {
			t.Errorf("Session Estab Req SEID must be 0, got %d", msg.Header.SEID)
		}
		if len(msg.IEs) != 2 {
			t.Errorf("Expected 2 IEs, got %d", len(msg.IEs))
		}
		
		idType, ip, _, err := ParseNodeID(msg.IEs[0])
		if err != nil {
			t.Fatalf("ParseNodeID failed: %v", err)
		}
		if idType != NodeIDTypeIPv4 || !bytes.Equal(ip, net.ParseIP("1.2.3.4").To4()) {
			t.Errorf("Unexpected NodeID: type=%d ip=%v", idType, ip)
		}
	})
	
	t.Run("Session Deletion Response", func(t *testing.T) {
		cause := NewCause(CauseRequestAccepted)
		resp := NewSessionDeletionResponse(3, 0x9988776655443322, cause)
		
		b := resp.Encode()
		
		msg, err := DecodeMessage(b)
		if err != nil {
			t.Fatalf("DecodeMessage failed: %v", err)
		}
		
		if msg.Header.MessageType != MsgTypeSessionDeletionResponse {
			t.Errorf("Expected MsgTypeSessionDeletionResponse, got %d", msg.Header.MessageType)
		}
		if msg.Header.SEID != 0x9988776655443322 {
			t.Errorf("Wrong SEID: got %x", msg.Header.SEID)
		}
		
		parsedCause, err := ParseCause(msg.IEs[0])
		if err != nil {
			t.Fatalf("ParseCause failed: %v", err)
		}
		if parsedCause != CauseRequestAccepted {
			t.Errorf("Expected CauseRequestAccepted, got %d", parsedCause)
		}
	})
}
