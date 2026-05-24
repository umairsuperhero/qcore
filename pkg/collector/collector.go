// Package collector receives structured events from all QCore network functions,
// stores them by journey ID, and serves them via:
//   - POST /events              — ingest one event (called by NF emitters)
//   - GET  /journeys/{id}/events — full trace for one journey (for Phase C AI)
//   - GET  /journeys             — list all known journey IDs
//   - GET  /events/stream        — SSE stream of live events (for Phase B UI)
package collector

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/mux"
	"github.com/qcore-project/qcore/pkg/events"
	"github.com/qcore-project/qcore/pkg/logger"
)

// Collector is the central event hub. It is designed to run as a separate
// process (cmd/qcore-collector) so NFs can be started independently.
type Collector struct {
	store  *Store
	log    logger.Logger
	router *mux.Router

	subsMu sync.Mutex
	subs   map[chan events.Event]struct{}
}

// New creates a Collector and wires its HTTP routes.
func New(log logger.Logger) *Collector {
	c := &Collector{
		store: newStore(),
		log:   log.WithField("component", "collector"),
		subs:  make(map[chan events.Event]struct{}),
	}
	r := mux.NewRouter()
	r.HandleFunc("/events", c.handleIngest).Methods(http.MethodPost)
	r.HandleFunc("/events/stream", c.handleStream).Methods(http.MethodGet)
	r.HandleFunc("/journeys", c.handleListJourneys).Methods(http.MethodGet)
	r.HandleFunc("/journeys/{id}/events", c.handleJourneyQuery).Methods(http.MethodGet)
	r.HandleFunc("/health", c.handleHealth).Methods(http.MethodGet)
	c.router = r
	return c
}

// Router returns the HTTP handler for use with http.Server.
func (c *Collector) Router() http.Handler { return c.router }

// --- HTTP handlers ---

func (c *Collector) handleIngest(w http.ResponseWriter, r *http.Request) {
	var ev events.Event
	if err := json.NewDecoder(r.Body).Decode(&ev); err != nil {
		http.Error(w, "invalid event JSON", http.StatusBadRequest)
		return
	}
	if ev.Timestamp.IsZero() {
		ev.Timestamp = time.Now().UTC()
	}
	c.store.Append(ev)
	c.broadcast(ev)
	w.WriteHeader(http.StatusNoContent)
}

func (c *Collector) handleJourneyQuery(w http.ResponseWriter, r *http.Request) {
	id := mux.Vars(r)["id"]
	evts := c.store.Journey(id)
	if evts == nil {
		http.Error(w, "journey not found", http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(evts)
}

func (c *Collector) handleListJourneys(w http.ResponseWriter, r *http.Request) {
	ids := c.store.JourneyIDs()
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(ids)
}

func (c *Collector) handleHealth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	fmt.Fprintf(w, `{"status":"ok"}`)
}

// handleStream serves a Server-Sent Events stream of all live events.
// Each event is encoded as a JSON data line: "data: {...}\n\n".
func (c *Collector) handleStream(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming unsupported", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("Access-Control-Allow-Origin", "*")

	ch := make(chan events.Event, 64)
	c.subsMu.Lock()
	c.subs[ch] = struct{}{}
	c.subsMu.Unlock()
	defer func() {
		c.subsMu.Lock()
		delete(c.subs, ch)
		c.subsMu.Unlock()
	}()

	for {
		select {
		case ev := <-ch:
			data, err := json.Marshal(ev)
			if err != nil {
				continue
			}
			fmt.Fprintf(w, "data: %s\n\n", data)
			flusher.Flush()
		case <-r.Context().Done():
			return
		}
	}
}

func (c *Collector) broadcast(ev events.Event) {
	c.subsMu.Lock()
	defer c.subsMu.Unlock()
	for ch := range c.subs {
		select {
		case ch <- ev:
		default:
			// slow subscriber — drop rather than block
		}
	}
}
