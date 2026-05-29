package upf

import (
	"errors"
	"net"
	"sync"
	"sync/atomic"
)

var (
	ErrSessionNotFound = errors.New("upf: session not found")
	ErrTEIDNotFound    = errors.New("upf: teid not found")
)

// Session represents a UPF PFCP session (PDRs, FARs).
type Session struct {
	SEID uint64
	
	// Uplink (UE -> Internet)
	LocalTEID uint32 // UPF's GTP-U N3 TEID
	UEIP      net.IP // UE's allocated IP
	
	// Downlink (Internet -> UE)
	RemoteTEID uint32 // gNB's GTP-U N3 TEID
	RemoteIP   net.IP // gNB's IP
}

// SessionStore manages active UPF data plane sessions.
type SessionStore struct {
	nextTEID atomic.Uint32
	
	mu       sync.RWMutex
	bySEID   map[uint64]*Session
	byTEID   map[uint32]*Session // Maps LocalTEID -> Session
	byUEIP   map[string]*Session // Maps UEIP string -> Session (for downlink routing)
}

func NewSessionStore() *SessionStore {
	store := &SessionStore{
		bySEID: make(map[uint64]*Session),
		byTEID: make(map[uint32]*Session),
		byUEIP: make(map[string]*Session),
	}
	store.nextTEID.Store(1000) // Start allocating TEIDs from 1000
	return store
}

// AllocateTEID generates a new unique TEID.
func (s *SessionStore) AllocateTEID() uint32 {
	return s.nextTEID.Add(1)
}

// AddSession registers a new PFCP session and its data plane routing rules.
func (s *SessionStore) AddSession(sess *Session) {
	s.mu.Lock()
	defer s.mu.Unlock()
	
	s.bySEID[sess.SEID] = sess
	if sess.LocalTEID != 0 {
		s.byTEID[sess.LocalTEID] = sess
	}
	if sess.UEIP != nil {
		s.byUEIP[sess.UEIP.String()] = sess
	}
}

// GetBySEID finds a session by its PFCP SEID.
func (s *SessionStore) GetBySEID(seid uint64) (*Session, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	
	sess, ok := s.bySEID[seid]
	if !ok {
		return nil, ErrSessionNotFound
	}
	return sess, nil
}

// GetByTEID finds a session by its incoming GTP-U TEID.
func (s *SessionStore) GetByTEID(teid uint32) (*Session, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	
	sess, ok := s.byTEID[teid]
	if !ok {
		return nil, ErrTEIDNotFound
	}
	return sess, nil
}

// GetByUEIP finds a session by the destination IP of a downlink packet.
func (s *SessionStore) GetByUEIP(ip net.IP) (*Session, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	
	sess, ok := s.byUEIP[ip.String()]
	if !ok {
		return nil, ErrSessionNotFound
	}
	return sess, nil
}

// RemoveSession deletes a session and its routing entries.
func (s *SessionStore) RemoveSession(seid uint64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	
	sess, ok := s.bySEID[seid]
	if !ok {
		return
	}
	
	delete(s.bySEID, seid)
	if sess.LocalTEID != 0 {
		delete(s.byTEID, sess.LocalTEID)
	}
	if sess.UEIP != nil {
		delete(s.byUEIP, sess.UEIP.String())
	}
}
