package collector

import (
	"sync"

	"github.com/qcore-project/qcore/pkg/events"
)

const recentCap = 500

// Store holds events in memory. Events are indexed by journey ID for
// targeted query (Phase C AI) and also kept in a bounded recent list
// for SSE broadcast replay.
type Store struct {
	mu       sync.RWMutex
	journeys map[string]*journeyBuf
	recent   []events.Event
}

type journeyBuf struct {
	mu     sync.RWMutex
	events []events.Event
}

func newStore() *Store {
	return &Store{journeys: make(map[string]*journeyBuf)}
}

// Append records an event. Events with a JourneyID are indexed; all events
// are appended to the recent ring.
func (s *Store) Append(ev events.Event) {
	if ev.JourneyID != "" {
		s.mu.Lock()
		jb, ok := s.journeys[ev.JourneyID]
		if !ok {
			jb = &journeyBuf{}
			s.journeys[ev.JourneyID] = jb
		}
		s.mu.Unlock()

		jb.mu.Lock()
		jb.events = append(jb.events, ev)
		jb.mu.Unlock()
	}

	s.mu.Lock()
	if len(s.recent) >= recentCap {
		copy(s.recent, s.recent[1:])
		s.recent = s.recent[:recentCap-1]
	}
	s.recent = append(s.recent, ev)
	s.mu.Unlock()
}

// Journey returns a copy of all events for a journey ID, in arrival order.
// Returns nil if the ID is unknown.
func (s *Store) Journey(id string) []events.Event {
	s.mu.RLock()
	jb, ok := s.journeys[id]
	s.mu.RUnlock()
	if !ok {
		return nil
	}
	jb.mu.RLock()
	defer jb.mu.RUnlock()
	cp := make([]events.Event, len(jb.events))
	copy(cp, jb.events)
	return cp
}

// JourneyIDs returns all known journey IDs.
func (s *Store) JourneyIDs() []string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	ids := make([]string, 0, len(s.journeys))
	for id := range s.journeys {
		ids = append(ids, id)
	}
	return ids
}
