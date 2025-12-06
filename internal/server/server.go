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

// ClientInfo stores information about a connected client
type ClientInfo struct {
	Ticker string
}

// Server manages WebSocket connections and broadcasts messages
type Server struct {
	clients    map[*websocket.Conn]*ClientInfo
	broadcast  chan analysis.TimePeriodSummary
	register   chan *websocket.Conn
	unregister chan *websocket.Conn
	mu         sync.RWMutex
}

// NewServer creates a new WebSocket server
func NewServer() *Server {
	return &Server{
		clients:    make(map[*websocket.Conn]*ClientInfo),
		broadcast:  make(chan analysis.TimePeriodSummary, 256),
		register:   make(chan *websocket.Conn),
		unregister: make(chan *websocket.Conn),
	}
}

// Run starts the server's connection management goroutine
func (s *Server) Run() {
	for {
		select {
		case <-s.register:
			s.mu.RLock()
			clientCount := len(s.clients)
			s.mu.RUnlock()
			log.Printf("Client connected. Total clients: %d", clientCount)

		case conn := <-s.unregister:
			s.mu.Lock()
			if _, ok := s.clients[conn]; ok {
				delete(s.clients, conn)
				conn.Close()
			}
			s.mu.Unlock()
			log.Printf("Client disconnected. Total clients: %d", len(s.clients))

		case message := <-s.broadcast:
			// Broadcast is now handled per-ticker in SendUpdateForTicker
			// This channel is kept for backward compatibility but won't be used
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

// SendUpdate sends an update to all clients subscribed to a specific ticker
func (s *Server) SendUpdateForTicker(ticker string, summary analysis.TimePeriodSummary) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	for conn, info := range s.clients {
		if info != nil && info.Ticker == ticker {
			err := conn.WriteJSON(summary)
			if err != nil {
				log.Printf("Error writing to client: %v", err)
				conn.Close()
				s.mu.RUnlock()
				s.mu.Lock()
				delete(s.clients, conn)
				s.mu.Unlock()
				s.mu.RLock()
			}
		}
	}
}

// GetSubscribedTickers returns a map of all tickers that have active subscriptions
func (s *Server) GetSubscribedTickers() map[string]bool {
	s.mu.RLock()
	defer s.mu.RUnlock()

	tickers := make(map[string]bool)
	for _, info := range s.clients {
		if info != nil && info.Ticker != "" {
			tickers[info.Ticker] = true
		}
	}
	return tickers
}

// Register registers a new client connection with a ticker
func (s *Server) Register(conn *websocket.Conn, ticker string) {
	s.mu.Lock()
	s.clients[conn] = &ClientInfo{Ticker: ticker}
	clientCount := len(s.clients)
	s.mu.Unlock()
	log.Printf("Client connected for ticker %s. Total clients: %d", ticker, clientCount)
	// Send to register channel to trigger any other handlers
	select {
	case s.register <- conn:
	default:
	}
}

// Unregister unregisters a client connection
func (s *Server) Unregister(conn *websocket.Conn) {
	s.unregister <- conn
}
