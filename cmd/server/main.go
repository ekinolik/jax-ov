package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"time"

	"github.com/ekinolik/jax-ov/internal/analysis"
	"github.com/ekinolik/jax-ov/internal/server"
	"github.com/gorilla/websocket"
)

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool {
		return true // Allow all origins
	},
}

func main() {
	// Parse command-line flags
	logDir := flag.String("log-dir", "./logs", "Log directory path (default: ./logs)")
	period := flag.Int("period", 5, "Analysis period in minutes (default: 5)")
	port := flag.String("port", "8080", "WebSocket server port (default: 8080)")
	host := flag.String("host", "localhost", "Bind address (default: localhost)")
	flag.Parse()

	// Create WebSocket server
	wsServer := server.NewServer()
	go wsServer.Run()

	// HTTP handler for WebSocket connections
	http.HandleFunc("/analyze", func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			log.Printf("WebSocket upgrade error: %v", err)
			return
		}

		// Register connection
		wsServer.Register(conn)

		// Get date from query parameter, default to current date
		dateStr := r.URL.Query().Get("date")
		if dateStr == "" {
			// Use current date in Pacific timezone
			pacificTZ, _ := time.LoadLocation("America/Los_Angeles")
			dateStr = time.Now().In(pacificTZ).Format("2006-01-02")
		}

		// Validate date format (YYYY-MM-DD)
		_, err = time.Parse("2006-01-02", dateStr)
		if err != nil {
			log.Printf("Invalid date format: %s, using current date", dateStr)
			pacificTZ, _ := time.LoadLocation("America/Los_Angeles")
			dateStr = time.Now().In(pacificTZ).Format("2006-01-02")
		}

		// Send historical data immediately for the specified date
		summaries, err := server.AnalyzeDate(*logDir, dateStr, *period)
		if err != nil {
			log.Printf("Error getting historical data for date %s: %v", dateStr, err)
		} else {
			if err := wsServer.SendHistory(conn, summaries); err != nil {
				log.Printf("Error sending history: %v", err)
			} else {
				log.Printf("Sent %d historical periods to new client for date %s", len(summaries), dateStr)
			}
		}

		// Handle connection (ping/pong, cleanup on disconnect)
		go func() {
			defer func() {
				wsServer.Unregister(conn)
				conn.Close()
			}()

			ticker := time.NewTicker(54 * time.Second)
			defer ticker.Stop()

			for {
				select {
				case <-ticker.C:
					if err := conn.WriteMessage(websocket.PingMessage, nil); err != nil {
						return
					}
				}
			}
		}()
	})

	// HTTP GET handler for transactions endpoint
	http.HandleFunc("/transactions", func(w http.ResponseWriter, r *http.Request) {
		// Only allow GET requests
		if r.Method != http.MethodGet {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		// Get query parameters
		dateStr := r.URL.Query().Get("date")
		timeStr := r.URL.Query().Get("time")
		periodStr := r.URL.Query().Get("period")

		// Time is required
		if timeStr == "" {
			http.Error(w, "time parameter is required (format: HH:MM)", http.StatusBadRequest)
			return
		}

		// Default period to 1 minute if not provided
		periodMinutes := 1
		if periodStr != "" {
			period, err := strconv.Atoi(periodStr)
			if err != nil || period <= 0 {
				http.Error(w, "invalid period, must be a positive integer", http.StatusBadRequest)
				return
			}
			periodMinutes = period
		}

		// Get transactions for the time period
		transactions, err := server.GetTransactionsForTimePeriod(*logDir, dateStr, timeStr, periodMinutes)
		if err != nil {
			log.Printf("Error getting transactions: %v", err)
			http.Error(w, fmt.Sprintf("Error getting transactions: %v", err), http.StatusInternalServerError)
			return
		}

		// Set content type and return JSON array
		w.Header().Set("Content-Type", "application/json")
		encoder := json.NewEncoder(w)
		encoder.SetIndent("", "  ")
		if err := encoder.Encode(transactions); err != nil {
			log.Printf("Error encoding JSON: %v", err)
			http.Error(w, "Error encoding response", http.StatusInternalServerError)
			return
		}
	})

	// Root handler
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/" {
			w.Header().Set("Content-Type", "text/html")
			w.Write([]byte(`<html><body><h1>Options Analysis WebSocket Server</h1><p>Connect to ws://` + *host + `:` + *port + `/analyze</p><p>Get transactions: GET http://` + *host + `:` + *port + `/transactions?date=YYYY-MM-DD&time=HH:MM&period=N</p></body></html>`))
		}
	})

	// Start analysis loop
	go func() {
		ticker := time.NewTicker(1 * time.Minute)
		defer ticker.Stop()

		// Track last processed period end timestamp
		var lastPeriodEnd int64 = 0

		// Initial analysis on startup
		summaries, err := server.AnalyzeCurrentDay(*logDir, *period)
		if err != nil {
			log.Printf("Error analyzing current day: %v", err)
		} else {
			log.Printf("Initial analysis: %d time periods", len(summaries))
			if len(summaries) > 0 {
				lastSummary := summaries[len(summaries)-1]
				lastPeriodEnd = lastSummary.PeriodEnd.UnixMilli()
			}
		}

		// Analyze and broadcast every minute
		for range ticker.C {
			summaries, err := server.AnalyzeCurrentDay(*logDir, *period)
			if err != nil {
				log.Printf("Error analyzing current day: %v", err)
				continue
			}

			if len(summaries) == 0 {
				continue
			}

			// Find the latest complete period
			now := time.Now()
			periodDuration := time.Duration(*period) * time.Minute

			var latestCompleteSummary *analysis.TimePeriodSummary
			for i := len(summaries) - 1; i >= 0; i-- {
				if now.Sub(summaries[i].PeriodEnd) >= periodDuration {
					latestCompleteSummary = &summaries[i]
					break
				}
			}

			// Send update if we have a new complete period
			if latestCompleteSummary != nil {
				periodEnd := latestCompleteSummary.PeriodEnd.UnixMilli()
				if periodEnd > lastPeriodEnd {
					wsServer.SendUpdate(*latestCompleteSummary)
					lastPeriodEnd = periodEnd
					log.Printf("Sent update for period ending at %s", latestCompleteSummary.PeriodEnd.Format("15:04:05"))
				}
			}
		}
	}()

	// Start HTTP server
	addr := fmt.Sprintf("%s:%s", *host, *port)
	log.Printf("Starting server on %s", addr)
	log.Printf("WebSocket endpoint: ws://%s/analyze", addr)
	log.Printf("Transactions endpoint: http://%s/transactions?date=YYYY-MM-DD&time=HH:MM&period=N", addr)
	log.Fatal(http.ListenAndServe(addr, nil))
}
