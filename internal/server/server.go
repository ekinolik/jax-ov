package server

import (
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/ekinolik/jax-ov/internal/analysis"
	"github.com/gorilla/websocket"
)

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool {
		return true // Allow all origins
	},
}

// Server manages WebSocket connections and broadcasts messages
type Server struct {
	clients    map[*websocket.Conn]bool
	broadcast  chan analysis.TimePeriodSummary
	register   chan *websocket.Conn
	unregister chan *websocket.Conn
	mu         sync.RWMutex
}

// NewServer creates a new WebSocket server
func NewServer() *Server {
	return &Server{
		clients:    make(map[*websocket.Conn]bool),
		broadcast:  make(chan analysis.TimePeriodSummary, 256),
		register:   make(chan *websocket.Conn),
		unregister: make(chan *websocket.Conn),
	}
}

// Run starts the server's connection management goroutine
func (s *Server) Run() {
	for {
		select {
		case conn := <-s.register:
			s.mu.Lock()
			s.clients[conn] = true
			s.mu.Unlock()
			log.Printf("Client connected. Total clients: %d", len(s.clients))

		case conn := <-s.unregister:
			s.mu.Lock()
			if _, ok := s.clients[conn]; ok {
				delete(s.clients, conn)
				conn.Close()
			}
			s.mu.Unlock()
			log.Printf("Client disconnected. Total clients: %d", len(s.clients))

		case message := <-s.broadcast:
			s.mu.RLock()
			for conn := range s.clients {
				err := conn.WriteJSON(message)
				if err != nil {
					log.Printf("Error writing to client: %v", err)
					conn.Close()
					delete(s.clients, conn)
				}
			}
			s.mu.RUnlock()
		}
	}
}

// Broadcast sends a summary to all connected clients
func (s *Server) Broadcast(summary analysis.TimePeriodSummary) {
	s.broadcast <- summary
}

// HandleWebSocket handles WebSocket connections
func (s *Server) HandleWebSocket(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("WebSocket upgrade error: %v", err)
		return
	}

	s.register <- conn

	// Set up ping/pong to keep connection alive
	go func() {
		ticker := time.NewTicker(54 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				if err := conn.WriteMessage(websocket.PingMessage, nil); err != nil {
					s.unregister <- conn
					return
				}
			}
		}
	}()

	// Handle client messages (if needed in future)
	go func() {
		for {
			_, _, err := conn.ReadMessage()
			if err != nil {
				s.unregister <- conn
				return
			}
		}
	}()
}

// SendHistory sends historical data to a specific client
func (s *Server) SendHistory(conn *websocket.Conn, summaries []analysis.TimePeriodSummary) error {
	// Send each summary as a separate message (just the summary object, no wrapper)
	for _, summary := range summaries {
		if err := conn.WriteJSON(summary); err != nil {
			return err
		}
	}
	return nil
}

// SendUpdate sends an update to all clients
func (s *Server) SendUpdate(summary analysis.TimePeriodSummary) {
	// Send the summary directly, not wrapped in a Message struct
	s.Broadcast(summary)
}

// Register registers a new client connection
func (s *Server) Register(conn *websocket.Conn) {
	s.register <- conn
}

// Unregister unregisters a client connection
func (s *Server) Unregister(conn *websocket.Conn) {
	s.unregister <- conn
}
