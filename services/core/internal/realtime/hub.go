// Package realtime fans out events to WebSocket clients. Each client has a
// bounded send buffer; slow consumers are disconnected instead of blocking
// the broadcaster (back-pressure isolation).
package realtime

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gorilla/websocket"
)

// Event is the standard envelope for every realtime message.
type Event struct {
	EventID   string    `json:"event_id"`
	EventType string    `json:"event_type"`
	Source    string    `json:"source"`
	Timestamp time.Time `json:"timestamp"`
	Payload   any       `json:"payload"`
}

const (
	TrainPositions   = "TRAIN_POSITIONS_UPDATED"
	CrowdUpdated     = "CROWD_UPDATED"
	IncidentCreated  = "INCIDENT_CREATED"
	ScenarioChanged  = "SCENARIO_CHANGED"
	TicketBooked     = "TICKET_BOOKED"
	PredictionMade   = "PREDICTION_GENERATED"
)

type client struct {
	conn   *websocket.Conn
	send   chan []byte
	mu     sync.RWMutex
	topics map[string]bool // empty => everything
}

func (c *client) wants(t string) bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return len(c.topics) == 0 || c.topics[t]
}

type Hub struct {
	mu       sync.RWMutex
	clients  map[*client]struct{}
	seq      atomic.Uint64
	upgrader websocket.Upgrader
	log      *slog.Logger
	OnCount  func(n int)
}

func NewHub(allowed func(origin string) bool, log *slog.Logger) *Hub {
	return &Hub{
		clients: map[*client]struct{}{},
		log:     log,
		upgrader: websocket.Upgrader{
			ReadBufferSize: 1024, WriteBufferSize: 4096,
			CheckOrigin: func(r *http.Request) bool {
				o := r.Header.Get("Origin")
				return o == "" || allowed(o)
			},
		},
	}
}

func (h *Hub) Count() int {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return len(h.clients)
}

func (h *Hub) NewEvent(t, source string, payload any) Event {
	return Event{EventID: fmt.Sprintf("evt_%d", h.seq.Add(1)), EventType: t, Source: source, Timestamp: time.Now().UTC(), Payload: payload}
}

func (h *Hub) Broadcast(e Event) {
	b, err := json.Marshal(e)
	if err != nil {
		return
	}
	h.mu.RLock()
	var slow []*client
	for c := range h.clients {
		if !c.wants(e.EventType) {
			continue
		}
		select {
		case c.send <- b:
		default:
			slow = append(slow, c)
		}
	}
	h.mu.RUnlock()
	for _, c := range slow {
		h.remove(c)
	}
}

func (h *Hub) remove(c *client) {
	h.mu.Lock()
	if _, ok := h.clients[c]; ok {
		delete(h.clients, c)
		close(c.send)
	}
	n := len(h.clients)
	h.mu.Unlock()
	if h.OnCount != nil {
		h.OnCount(n)
	}
}

// Serve upgrades the connection. `initial` events are sent first (snapshot).
func (h *Hub) Serve(w http.ResponseWriter, r *http.Request, initial ...Event) {
	conn, err := h.upgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	c := &client{conn: conn, send: make(chan []byte, 64), topics: map[string]bool{}}
	for _, e := range initial {
		if b, err := json.Marshal(e); err == nil {
			c.send <- b
		}
	}
	h.mu.Lock()
	h.clients[c] = struct{}{}
	n := len(h.clients)
	h.mu.Unlock()
	if h.OnCount != nil {
		h.OnCount(n)
	}
	go h.writer(c)
	h.reader(c)
}

func (h *Hub) reader(c *client) {
	defer func() { h.remove(c); c.conn.Close() }()
	c.conn.SetReadLimit(4096)
	_ = c.conn.SetReadDeadline(time.Now().Add(60 * time.Second))
	c.conn.SetPongHandler(func(string) error {
		return c.conn.SetReadDeadline(time.Now().Add(60 * time.Second))
	})
	for {
		_, msg, err := c.conn.ReadMessage()
		if err != nil {
			return
		}
		var m struct {
			Subscribe []string `json:"subscribe"`
		}
		if json.Unmarshal(msg, &m) == nil && m.Subscribe != nil {
			c.mu.Lock()
			c.topics = map[string]bool{}
			for _, t := range m.Subscribe {
				c.topics[t] = true
			}
			c.mu.Unlock()
		}
	}
}

func (h *Hub) writer(c *client) {
	ping := time.NewTicker(25 * time.Second)
	defer func() { ping.Stop(); c.conn.Close() }()
	for {
		select {
		case b, ok := <-c.send:
			_ = c.conn.SetWriteDeadline(time.Now().Add(5 * time.Second))
			if !ok {
				_ = c.conn.WriteMessage(websocket.CloseMessage, nil)
				return
			}
			if err := c.conn.WriteMessage(websocket.TextMessage, b); err != nil {
				return
			}
		case <-ping.C:
			_ = c.conn.SetWriteDeadline(time.Now().Add(5 * time.Second))
			if err := c.conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		}
	}
}
