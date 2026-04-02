// Package ws provides a WebSocket hub for broadcasting real-time download
// progress to connected browser clients. It uses the gorilla/websocket
// library and follows the standard Hub/Client fan-out pattern.
package ws

import (
	"log/slog"
	"net/http"
	"sync"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
)

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin: func(r *http.Request) bool {
		return true // allow all origins (dev + production)
	},
}

// client represents a single WebSocket connection.
type client struct {
	conn *websocket.Conn
	send chan any
}

// Hub manages WebSocket client connections and broadcasts messages.
// It is safe for concurrent use.
type Hub struct {
	clients    map[*client]bool
	broadcast  chan any
	register   chan *client
	unregister chan *client
	mu         sync.Mutex

	// lastStatus caches the most recent status so new clients get it immediately.
	lastStatus any
	statusMu   sync.RWMutex
}

// NewHub creates a new Hub. Call Run() to start the event loop.
func NewHub() *Hub {
	return &Hub{
		clients:    make(map[*client]bool),
		broadcast:  make(chan any, 64),
		register:   make(chan *client),
		unregister: make(chan *client),
	}
}

// Run starts the hub event loop. It blocks and should be called in a goroutine.
func (h *Hub) Run() {
	for {
		select {
		case c := <-h.register:
			h.mu.Lock()
			h.clients[c] = true
			h.mu.Unlock()
			slog.Debug("ws: client connected", "clients", len(h.clients))

			// Send current status to the newly connected client.
			h.statusMu.RLock()
			if h.lastStatus != nil {
				select {
				case c.send <- h.lastStatus:
				default:
				}
			}
			h.statusMu.RUnlock()

		case c := <-h.unregister:
			h.mu.Lock()
			if _, ok := h.clients[c]; ok {
				delete(h.clients, c)
				close(c.send)
			}
			h.mu.Unlock()
			slog.Debug("ws: client disconnected", "clients", len(h.clients))

		case msg := <-h.broadcast:
			h.statusMu.Lock()
			h.lastStatus = msg
			h.statusMu.Unlock()

			h.mu.Lock()
			for c := range h.clients {
				select {
				case c.send <- msg:
				default:
					// Client is too slow; disconnect it.
					close(c.send)
					delete(h.clients, c)
				}
			}
			h.mu.Unlock()
		}
	}
}

// Broadcast sends a message to all connected clients. Non-blocking: if the
// broadcast channel is full the message is dropped.
func (h *Hub) Broadcast(msg any) {
	select {
	case h.broadcast <- msg:
	default:
		// Drop if hub is busy.
	}
}

// HandleWebSocket is a Gin handler that upgrades the HTTP connection to WebSocket.
func (h *Hub) HandleWebSocket(c *gin.Context) {
	conn, err := upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		slog.Error("ws: upgrade failed", "error", err)
		return
	}

	cl := &client{conn: conn, send: make(chan any, 256)}
	h.register <- cl

	// Read pump: keep the connection alive and detect disconnects.
	go func() {
		defer func() {
			h.unregister <- cl
			conn.Close()
		}()
		for {
			if _, _, err := conn.ReadMessage(); err != nil {
				break
			}
		}
	}()

	// Write pump: send messages to the client.
	go func() {
		for msg := range cl.send {
			if err := conn.WriteJSON(msg); err != nil {
				slog.Debug("ws: write error", "error", err)
				break
			}
		}
	}()
}
