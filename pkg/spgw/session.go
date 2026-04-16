// Package spgw implements QCore's collapsed Serving/Packet Data Network
// Gateway — a single user-plane anchor that terminates the GTP-U tunnel from
// the eNodeB and forwards the inner IP packets to its egress (TUN device,
// log-only sink, etc.).
//
// Phase 3 scope: one default bearer per UE, one APN, IPv4 only.
package spgw

import (
	"fmt"
	"net"
	"sync"
	"time"
)

// Bearer represents one EPS bearer (default bearer in Phase 3).
type Bearer struct {
	IMSI string
	// S1-U: eNB ↔ SGW. The SGW allocates the DL TEID; the eNB allocates the UL.
	SGWTEID  uint32 // DL TEID (eNB encapsulates with this on UL; SGW receives and decapsulates)
	SGWAddr  net.IP // SGW S1-U bind address (what we advertise to the MME)
	ENBTEID  uint32 // UL TEID (SGW encapsulates with this on DL; eNB receives)
	ENBAddr  net.IP // eNB S1-U address
	UEIP     net.IP // allocated UE IP
	EBI      uint8  // EPS Bearer ID (e.g. 5 for default bearer)
	APN      string
	CreatedAt time.Time
}

// SessionStore holds all active bearers, indexed by every key we need to look
// them up by. It's safe for concurrent use.
type SessionStore struct {
	mu       sync.RWMutex
	byIMSI   map[string]*Bearer
	bySGWTEID map[uint32]*Bearer
	byUEIP   map[string]*Bearer
}

// NewSessionStore creates an empty session store.
func NewSessionStore() *SessionStore {
	return &SessionStore{
		byIMSI:    make(map[string]*Bearer),
		bySGWTEID: make(map[uint32]*Bearer),
		byUEIP:    make(map[string]*Bearer),
	}
}

// Add indexes a new bearer. Returns an error if a session for this IMSI
// already exists — callers should delete before replacing.
func (s *SessionStore) Add(b *Bearer) error {
	if b == nil || b.IMSI == "" {
		return fmt.Errorf("invalid bearer")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, exists := s.byIMSI[b.IMSI]; exists {
		return fmt.Errorf("session already exists for IMSI %s", b.IMSI)
	}
	s.byIMSI[b.IMSI] = b
	s.bySGWTEID[b.SGWTEID] = b
	if b.UEIP != nil {
		s.byUEIP[b.UEIP.String()] = b
	}
	return nil
}

// UpdateENB records the eNB's uplink TEID + address after Modify Bearer.
func (s *SessionStore) UpdateENB(imsi string, enbTEID uint32, enbAddr net.IP) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	b, ok := s.byIMSI[imsi]
	if !ok {
		return fmt.Errorf("no session for IMSI %s", imsi)
	}
	b.ENBTEID = enbTEID
	b.ENBAddr = enbAddr
	return nil
}

// Delete removes a session by IMSI and returns its freed IP + TEID so callers
// can recycle them in their pools. Returns (nil, 0, nil) if the IMSI is unknown.
func (s *SessionStore) Delete(imsi string) (net.IP, uint32, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	b, ok := s.byIMSI[imsi]
	if !ok {
		return nil, 0, nil
	}
	delete(s.byIMSI, imsi)
	delete(s.bySGWTEID, b.SGWTEID)
	if b.UEIP != nil {
		delete(s.byUEIP, b.UEIP.String())
	}
	return b.UEIP, b.SGWTEID, nil
}

// GetByIMSI looks up a bearer by its IMSI.
func (s *SessionStore) GetByIMSI(imsi string) (*Bearer, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	b, ok := s.byIMSI[imsi]
	return b, ok
}

// GetBySGWTEID looks up the bearer bound to an SGW (DL) TEID. Hot path —
// called on every uplink GTP-U packet.
func (s *SessionStore) GetBySGWTEID(teid uint32) (*Bearer, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	b, ok := s.bySGWTEID[teid]
	return b, ok
}

// GetByUEIP looks up a bearer by its assigned UE IP. Hot path — called on
// every downlink packet received from egress to pick the tunnel to put it in.
func (s *SessionStore) GetByUEIP(ip net.IP) (*Bearer, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	b, ok := s.byUEIP[ip.String()]
	return b, ok
}

// Count returns the number of active sessions.
func (s *SessionStore) Count() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.byIMSI)
}

// Snapshot returns a copy of all active bearers (safe to serialise).
func (s *SessionStore) Snapshot() []*Bearer {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]*Bearer, 0, len(s.byIMSI))
	for _, b := range s.byIMSI {
		bc := *b
		out = append(out, &bc)
	}
	return out
}
