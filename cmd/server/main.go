package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"
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
		// Get ticker from query parameter (required)
		ticker := r.URL.Query().Get("ticker")
		if ticker == "" {
			log.Printf("ticker parameter is required, closing connection")
			http.Error(w, "ticker parameter is required", http.StatusBadRequest)
			return
		}
		ticker = strings.ToUpper(ticker)

		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			log.Printf("WebSocket upgrade error: %v", err)
			return
		}

		// Register connection with ticker
		wsServer.Register(conn, ticker)

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

		// Send historical data immediately for the specified ticker and date
		summaries, err := server.AnalyzeTickerAndDate(*logDir, ticker, dateStr, *period)
		if err != nil {
			log.Printf("Error getting historical data for ticker %s, date %s: %v", ticker, dateStr, err)
		} else {
			if err := wsServer.SendHistory(conn, summaries); err != nil {
				log.Printf("Error sending history: %v", err)
			} else {
				log.Printf("Sent %d historical periods to new client for ticker %s, date %s", len(summaries), ticker, dateStr)
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
		ticker := r.URL.Query().Get("ticker")
		dateStr := r.URL.Query().Get("date")
		timeStr := r.URL.Query().Get("time")
		periodStr := r.URL.Query().Get("period")

		// Ticker is required
		if ticker == "" {
			http.Error(w, "ticker parameter is required", http.StatusBadRequest)
			return
		}
		ticker = strings.ToUpper(ticker)

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

		// Get transactions for the time period and ticker
		transactions, err := server.GetTransactionsForTickerAndTimePeriod(*logDir, ticker, dateStr, timeStr, periodMinutes)
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
			w.Write([]byte(`<html><body><h1>Options Analysis WebSocket Server</h1><p>Connect to ws://` + *host + `:` + *port + `/analyze?ticker=SYMBOL&date=YYYY-MM-DD</p><p>Get transactions: GET http://` + *host + `:` + *port + `/transactions?ticker=SYMBOL&date=YYYY-MM-DD&time=HH:MM&period=N</p></body></html>`))
		}
	})

	// Start analysis loop - analyze per ticker and send updates to subscribed clients
	go func() {
		updateTicker := time.NewTicker(1 * time.Minute)
		defer updateTicker.Stop()

		// Track last processed period end timestamp per ticker
		lastPeriodEnds := make(map[string]int64)

		// Analyze and broadcast every minute
		for range updateTicker.C {
			// Get current date in Pacific timezone
			pacificTZ, _ := time.LoadLocation("America/Los_Angeles")
			dateStr := time.Now().In(pacificTZ).Format("2006-01-02")

			// Get all unique tickers from connected clients
			tickers := wsServer.GetSubscribedTickers()

			// Analyze and send updates for each ticker
			for ticker := range tickers {
				summaries, err := server.AnalyzeTickerAndDate(*logDir, ticker, dateStr, *period)
				if err != nil {
					log.Printf("Error analyzing ticker %s: %v", ticker, err)
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

				// Send update if we have a new complete period for this ticker
				if latestCompleteSummary != nil {
					periodEnd := latestCompleteSummary.PeriodEnd.UnixMilli()
					lastPeriodEnd, exists := lastPeriodEnds[ticker]
					if !exists || periodEnd > lastPeriodEnd {
						wsServer.SendUpdateForTicker(ticker, *latestCompleteSummary)
						lastPeriodEnds[ticker] = periodEnd
						log.Printf("Sent update for ticker %s, period ending at %s", ticker, latestCompleteSummary.PeriodEnd.Format("15:04:05"))
					}
				}
			}
		}
	}()

	// Start HTTP server
	addr := fmt.Sprintf("%s:%s", *host, *port)
	log.Printf("Starting server on %s", addr)
	log.Printf("WebSocket endpoint: ws://%s/analyze", addr)
	log.Printf("Transactions endpoint: http://%s/transactions?ticker=SYMBOL&date=YYYY-MM-DD&time=HH:MM&period=N", addr)
	log.Fatal(http.ListenAndServe(addr, nil))
}
