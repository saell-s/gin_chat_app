package realtime

import (
	"encoding/json"
	"log"
	"net/http"
	"net/url"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

const (
	writeWait      = 10 * time.Second
	pongWait       = 60 * time.Second
	pingPeriod     = 30 * time.Second
	maxMessageSize = 64 * 1024
	sendBuffer     = 64
)

// Event is the wire format for every server pushed message.
type Event struct {
	Type string         `json:"type"`
	Data map[string]any `json:"data,omitempty"`
}

// NewEvent builds an event envelope.
func NewEvent(typ string, data map[string]any) Event {
	return Event{Type: typ, Data: data}
}

// Client is one authenticated websocket connection.
type Client struct {
	hub    *Hub
	userID int64
	conn   *websocket.Conn
	send   chan []byte
	done   chan struct{}

	mu     sync.Mutex
	closed bool
}

// UserID returns the account owning this connection.
func (c *Client) UserID() int64 { return c.userID }

// Send delivers a single event to this connection.
func (c *Client) Send(ev Event) bool {
	payload, err := json.Marshal(ev)
	if err != nil {
		return false
	}
	return c.trySend(payload)
}

func (c *Client) close() {
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return
	}
	c.closed = true
	close(c.done)
	c.mu.Unlock()
	_ = c.conn.Close()
}

// Hub tracks every live connection and routes inbound client frames.
type Hub struct {
	mu      sync.RWMutex
	clients map[int64]map[*Client]bool

	// Router receives raw client frames (excluding protocol pings).
	Router func(userID int64, raw []byte)

	// OnConnect runs right after a client is registered (e.g. to send a snapshot).
	OnConnect func(c *Client)
}

// New creates an empty hub.
func New() *Hub {
	return &Hub{clients: make(map[int64]map[*Client]bool)}
}

var upgrader = websocket.Upgrader{
	ReadBufferSize:  4096,
	WriteBufferSize: 4096,
	CheckOrigin:     checkOrigin,
}

func checkOrigin(r *http.Request) bool {
	origin := r.Header.Get("Origin")
	if origin == "" {
		return true // non browser clients
	}
	u, err := url.Parse(origin)
	if err != nil {
		return false
	}
	return u.Host == r.Host
}

// Serve upgrades the request and blocks until the connection goes away.
func (h *Hub) Serve(w http.ResponseWriter, r *http.Request, userID int64) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		return // upgrade already wrote an error response
	}
	client := &Client{
		hub:    h,
		userID: userID,
		conn:   conn,
		send:   make(chan []byte, sendBuffer),
		done:   make(chan struct{}),
	}

	h.register(client)
	if h.OnConnect != nil {
		h.OnConnect(client)
	}
	h.broadcastPresence(userID, true)

	go h.writePump(client)
	h.readPump(client)

	h.unregister(client)
	h.broadcastPresence(userID, !h.Online(userID))
}

func (h *Hub) register(c *Client) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.clients[c.userID] == nil {
		h.clients[c.userID] = make(map[*Client]bool)
	}
	h.clients[c.userID][c] = true
}

func (h *Hub) unregister(c *Client) {
	h.mu.Lock()
	clients := h.clients[c.userID]
	delete(clients, c)
	empty := len(clients) == 0
	if empty {
		delete(h.clients, c.userID)
	}
	h.mu.Unlock()
	c.close()
}

// Online reports whether the user has at least one live connection.
func (h *Hub) Online(userID int64) bool {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return len(h.clients[userID]) > 0
}

// OnlineIDs lists every connected user id.
func (h *Hub) OnlineIDs() []int64 {
	h.mu.RLock()
	defer h.mu.RUnlock()
	out := make([]int64, 0, len(h.clients))
	for id, set := range h.clients {
		if len(set) > 0 {
			out = append(out, id)
		}
	}
	return out
}

// ClientCount counts live connections.
func (h *Hub) ClientCount() int {
	h.mu.RLock()
	defer h.mu.RUnlock()
	total := 0
	for _, set := range h.clients {
		total += len(set)
	}
	return total
}

// SendToUser queues an event for every connection of that user.
// It reports whether at least one connection accepted the frame.
func (h *Hub) SendToUser(userID int64, ev Event) bool {
	payload, err := json.Marshal(ev)
	if err != nil {
		return false
	}
	h.mu.RLock()
	clients := h.clients[userID]
	targets := make([]*Client, 0, len(clients))
	for c := range clients {
		targets = append(targets, c)
	}
	h.mu.RUnlock()

	sent := false
	for _, c := range targets {
		if c.trySend(payload) {
			sent = true
		}
	}
	return sent
}

// SendToUsers delivers an event to several users, returning who is online.
func (h *Hub) SendToUsers(userIDs []int64, ev Event) map[int64]bool {
	delivered := make(map[int64]bool, len(userIDs))
	for _, id := range userIDs {
		if h.SendToUser(id, ev) {
			delivered[id] = true
		}
	}
	return delivered
}

// Broadcast delivers an event to every connected client.
func (h *Hub) Broadcast(ev Event) {
	payload, err := json.Marshal(ev)
	if err != nil {
		return
	}
	h.mu.RLock()
	targets := make([]*Client, 0, 64)
	for _, set := range h.clients {
		for c := range set {
			targets = append(targets, c)
		}
	}
	h.mu.RUnlock()
	for _, c := range targets {
		c.trySend(payload)
	}
}

func (c *Client) trySend(payload []byte) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed {
		return false
	}
	select {
	case c.send <- payload:
		return true
	default:
		// Backpressure: drop the frame rather than the whole hub.
		return false
	}
}

func (h *Hub) broadcastPresence(userID int64, online bool) {
	h.Broadcast(NewEvent("presence.update", map[string]any{
		"user_id": userID,
		"online":  online,
	}))
}

func (h *Hub) readPump(c *Client) {
	defer func() {
		_ = c.conn.Close()
	}()
	c.conn.SetReadLimit(maxMessageSize)
	_ = c.conn.SetReadDeadline(time.Now().Add(pongWait))
	c.conn.SetPongHandler(func(string) error {
		return c.conn.SetReadDeadline(time.Now().Add(pongWait))
	})

	for {
		_, raw, err := c.conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseNormalClosure) {
				log.Printf("ws: read error for user %d: %v", c.userID, err)
			}
			return
		}
		_ = c.conn.SetReadDeadline(time.Now().Add(pongWait))

		var probe struct {
			Type string `json:"type"`
		}
		if err := json.Unmarshal(raw, &probe); err != nil {
			c.trySendPayload(NewEvent("error", map[string]any{"message": "invalid json"}))
			continue
		}
		switch probe.Type {
		case "ping":
			c.trySendPayload(NewEvent("pong", nil))
		default:
			if h.Router != nil {
				h.Router(c.userID, raw)
			}
		}
	}
}

func (c *Client) trySendPayload(ev Event) {
	if payload, err := json.Marshal(ev); err == nil {
		c.trySend(payload)
	}
}

func (h *Hub) writePump(c *Client) {
	ticker := time.NewTicker(pingPeriod)
	defer func() {
		ticker.Stop()
		_ = c.conn.Close()
	}()
	for {
		select {
		case <-c.done:
			return
		case payload, ok := <-c.send:
			_ = c.conn.SetWriteDeadline(time.Now().Add(writeWait))
			if !ok {
				_ = c.conn.WriteMessage(websocket.CloseMessage, nil)
				return
			}
			if err := c.conn.WriteMessage(websocket.TextMessage, payload); err != nil {
				return
			}
		case <-ticker.C:
			_ = c.conn.SetWriteDeadline(time.Now().Add(writeWait))
			if err := c.conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		}
	}
}
