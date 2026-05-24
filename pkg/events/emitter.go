package events

import (
	"bytes"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/qcore-project/qcore/pkg/logger"
)

// Emitter is the interface every NF uses to record structured events.
// Implementations must be safe for concurrent use and must never block
// the caller — event emission is fire-and-forget on the critical path.
type Emitter interface {
	Emit(e Event)
}

// New returns an HTTPEmitter that posts events to collectorURL, or a
// NoopEmitter when collectorURL is empty. nf is the network function
// name stamped on every event (e.g. "mme", "spgw", "hss").
func New(collectorURL, nf string, log logger.Logger) Emitter {
	if collectorURL == "" {
		return &NoopEmitter{}
	}
	e := &HTTPEmitter{
		endpoint: collectorURL + "/events",
		nf:       nf,
		log:      log,
		queue:    make(chan Event, 1024),
		client:   &http.Client{Timeout: 2 * time.Second},
	}
	go e.drain()
	return e
}

// MintJourneyID returns a new UUID-v4 string prefixed with "j-".
// Called once per UE at first contact (in the MME on AttachRequest).
func MintJourneyID() string {
	var b [16]byte
	_, _ = rand.Read(b[:])
	b[6] = (b[6] & 0x0f) | 0x40 // version 4
	b[8] = (b[8] & 0x3f) | 0x80 // variant 2
	return fmt.Sprintf("j-%08x-%04x-%04x-%04x-%012x",
		b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}

func newEventID() string {
	var b [16]byte
	_, _ = rand.Read(b[:])
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x",
		b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}

// --- HTTPEmitter ---

// HTTPEmitter posts events to the collector over HTTP. Emission is
// non-blocking: events are queued in a buffered channel and drained by a
// background goroutine. If the queue is full or the collector is unreachable,
// events are silently dropped — observability must not degrade the core path.
type HTTPEmitter struct {
	endpoint string
	nf       string
	log      logger.Logger
	queue    chan Event
	client   *http.Client
}

func (e *HTTPEmitter) Emit(ev Event) {
	if ev.ID == "" {
		ev.ID = newEventID()
	}
	if ev.Timestamp.IsZero() {
		ev.Timestamp = time.Now().UTC()
	}
	ev.NF = e.nf
	select {
	case e.queue <- ev:
	default:
		// queue full — drop silently
	}
}

func (e *HTTPEmitter) drain() {
	for ev := range e.queue {
		e.post(ev)
	}
}

func (e *HTTPEmitter) post(ev Event) {
	body, err := json.Marshal(ev)
	if err != nil {
		return
	}
	resp, err := e.client.Post(e.endpoint, "application/json", bytes.NewReader(body))
	if err != nil {
		return // collector unreachable — swallow
	}
	resp.Body.Close()
}

// --- NoopEmitter ---

// NoopEmitter discards all events. Used in tests and when no collector URL
// is configured.
type NoopEmitter struct{}

func (n *NoopEmitter) Emit(Event) {}
