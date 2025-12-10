package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/ekinolik/jax-ov/internal/analysis"
	"github.com/ekinolik/jax-ov/internal/auth"
	"github.com/ekinolik/jax-ov/internal/config"
	"github.com/ekinolik/jax-ov/internal/notifications"
	"github.com/ekinolik/jax-ov/internal/server"
	"github.com/fsnotify/fsnotify"
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
	notificationsDir := flag.String("notifications-dir", "./notifications", "Notifications config directory (default: ./notifications)")
	period := flag.Int("period", 5, "Analysis period in minutes (default: 5)")
	port := flag.String("port", "8080", "WebSocket server port (default: 8080)")
	host := flag.String("host", "localhost", "Bind address (default: localhost)")
	flag.Parse()

	// Load authentication configuration
	authConfig, err := config.LoadAuth()
	if err != nil {
		log.Fatalf("Failed to load auth configuration: %v", err)
	}

	// Create WebSocket server
	wsServer := server.NewServer()
	go wsServer.Run()

	// Device registration endpoint (protected by JWT)
	devicesDir := flag.String("devices-dir", "./devices", "Devices directory path (default: ./devices)")

	http.Handle("/auth/register", auth.JWTMiddleware(authConfig.JWTSecret, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		// Extract user sub from JWT token
		authHeader := r.Header.Get("Authorization")
		if authHeader == "" {
			http.Error(w, "Authorization header required", http.StatusUnauthorized)
			return
		}

		parts := strings.SplitN(authHeader, " ", 2)
		if len(parts) != 2 || parts[0] != "Bearer" {
			http.Error(w, "Invalid authorization header format", http.StatusUnauthorized)
			return
		}

		sub, _, err := auth.ValidateSessionToken(parts[1], authConfig.JWTSecret)
		if err != nil {
			http.Error(w, "Invalid or expired token", http.StatusUnauthorized)
			return
		}

		// Parse request body
		var registerRequest struct {
			DeviceToken string `json:"device_token"`
		}

		if err := json.NewDecoder(r.Body).Decode(&registerRequest); err != nil {
			http.Error(w, "Invalid request body", http.StatusBadRequest)
			return
		}

		if registerRequest.DeviceToken == "" {
			http.Error(w, "device_token is required", http.StatusBadRequest)
			return
		}

		// Load existing devices for user
		devices, err := notifications.LoadUserDevices(sub, *devicesDir)
		if err != nil {
			log.Printf("Error loading devices for user %s: %v", sub, err)
			http.Error(w, "Error loading devices", http.StatusInternalServerError)
			return
		}

		// Add or update device token
		notifications.AddOrUpdateDevice(devices, registerRequest.DeviceToken)

		// Save devices back to file
		if err := notifications.SaveUserDevices(sub, *devicesDir, devices); err != nil {
			log.Printf("Error saving devices for user %s: %v", sub, err)
			http.Error(w, "Error saving device", http.StatusInternalServerError)
			return
		}

		// Return success response
		w.Header().Set("Content-Type", "application/json")
		response := map[string]interface{}{
			"success": true,
			"message": "Device registered",
		}
		if err := json.NewEncoder(w).Encode(response); err != nil {
			log.Printf("Failed to encode response: %v", err)
		}
	})))

	// Auth login endpoint (no JWT required)
	http.HandleFunc("/auth/login", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		// Parse request body
		var loginRequest struct {
			IdentityToken     string `json:"identity_token"`
			AuthorizationCode string `json:"authorization_code"`
		}

		if err := json.NewDecoder(r.Body).Decode(&loginRequest); err != nil {
			http.Error(w, "Invalid request body", http.StatusBadRequest)
			return
		}

		if loginRequest.IdentityToken == "" {
			http.Error(w, "identity_token is required", http.StatusBadRequest)
			return
		}

		// Validate Apple identity token
		sub, err := auth.ValidateAppleIdentityToken(loginRequest.IdentityToken, authConfig.AppleClientID)
		if err != nil {
			log.Printf("Apple identity token validation failed: %v", err)
			http.Error(w, "Invalid identity token", http.StatusUnauthorized)
			return
		}

		// Create session JWT
		sessionToken, err := auth.CreateSessionToken(sub, authConfig.JWTSecret, authConfig.JWTExpiryDuration())
		if err != nil {
			log.Printf("Failed to create session token: %v", err)
			http.Error(w, "Failed to create session", http.StatusInternalServerError)
			return
		}

		// Return session token
		w.Header().Set("Content-Type", "application/json")
		response := map[string]interface{}{
			"token":      sessionToken,
			"expires_in": int(authConfig.JWTExpiryDuration().Seconds()),
		}
		if err := json.NewEncoder(w).Encode(response); err != nil {
			log.Printf("Failed to encode response: %v", err)
		}
	})

	// HTTP handler for WebSocket connections (protected by JWT)
	http.HandleFunc("/analyze", func(w http.ResponseWriter, r *http.Request) {
		// Validate JWT before upgrading to WebSocket
		authHeader := r.Header.Get("Authorization")
		if authHeader == "" {
			http.Error(w, "Authorization header required", http.StatusUnauthorized)
			return
		}

		parts := strings.SplitN(authHeader, " ", 2)
		if len(parts) != 2 || parts[0] != "Bearer" {
			http.Error(w, "Invalid authorization header format", http.StatusUnauthorized)
			return
		}

		_, _, err := auth.ValidateSessionToken(parts[1], authConfig.JWTSecret)
		if err != nil {
			http.Error(w, "Invalid or expired token", http.StatusUnauthorized)
			return
		}
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

	// HTTP GET handler for transactions endpoint (protected by JWT)
	transactionsHandler := func(w http.ResponseWriter, r *http.Request) {
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
	}
	http.Handle("/transactions", auth.JWTMiddleware(authConfig.JWTSecret, http.HandlerFunc(transactionsHandler)))

	// GET /notifications endpoint (protected by JWT)
	getNotificationsHandler := func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		// Extract user sub from JWT (already validated by middleware)
		// We need to get it from the request context or re-validate
		authHeader := r.Header.Get("Authorization")
		if authHeader == "" {
			http.Error(w, "Authorization header required", http.StatusUnauthorized)
			return
		}

		parts := strings.SplitN(authHeader, " ", 2)
		if len(parts) != 2 || parts[0] != "Bearer" {
			http.Error(w, "Invalid authorization header format", http.StatusUnauthorized)
			return
		}

		sub, _, err := auth.ValidateSessionToken(parts[1], authConfig.JWTSecret)
		if err != nil {
			http.Error(w, "Invalid or expired token", http.StatusUnauthorized)
			return
		}

		// Load user notifications
		userConfig, err := notifications.LoadUserNotifications(sub, *notificationsDir)
		if err != nil {
			log.Printf("Error loading notifications for user %s: %v", sub, err)
			http.Error(w, "Error loading notifications", http.StatusInternalServerError)
			return
		}

		// Return response
		w.Header().Set("Content-Type", "application/json")
		response := map[string]interface{}{
			"notifications": userConfig.Notifications,
		}
		if err := json.NewEncoder(w).Encode(response); err != nil {
			log.Printf("Error encoding response: %v", err)
		}
	}

	// PUT /notifications endpoint (protected by JWT)
	putNotificationsHandler := func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPut {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		// Extract user sub from JWT
		authHeader := r.Header.Get("Authorization")
		if authHeader == "" {
			http.Error(w, "Authorization header required", http.StatusUnauthorized)
			return
		}

		parts := strings.SplitN(authHeader, " ", 2)
		if len(parts) != 2 || parts[0] != "Bearer" {
			http.Error(w, "Invalid authorization header format", http.StatusUnauthorized)
			return
		}

		sub, _, err := auth.ValidateSessionToken(parts[1], authConfig.JWTSecret)
		if err != nil {
			http.Error(w, "Invalid or expired token", http.StatusUnauthorized)
			return
		}

		// Parse request body
		var newConfig notifications.NotificationConfig
		if err := json.NewDecoder(r.Body).Decode(&newConfig); err != nil {
			http.Error(w, "Invalid request body", http.StatusBadRequest)
			return
		}

		// Validate required fields
		if newConfig.Ticker == "" {
			http.Error(w, "ticker is required", http.StatusBadRequest)
			return
		}
		newConfig.Ticker = strings.ToUpper(newConfig.Ticker)

		// Disabled defaults to false (active) if not provided (Go's zero value)

		// Load existing user notifications
		userConfig, err := notifications.LoadUserNotifications(sub, *notificationsDir)
		if err != nil {
			log.Printf("Error loading notifications for user %s: %v", sub, err)
			http.Error(w, "Error loading notifications", http.StatusInternalServerError)
			return
		}

		// Ensure notifications map exists
		if userConfig.Notifications == nil {
			userConfig.Notifications = make(map[string]notifications.NotificationConfig)
		}

		// Overwrite notification for this ticker (only one per ticker)
		userConfig.Notifications[newConfig.Ticker] = newConfig

		// Save user notifications
		if err := notifications.SaveUserNotifications(sub, *notificationsDir, userConfig); err != nil {
			log.Printf("Error saving notifications for user %s: %v", sub, err)
			http.Error(w, "Error saving notifications", http.StatusInternalServerError)
			return
		}

		// Return success
		w.Header().Set("Content-Type", "application/json")
		response := map[string]interface{}{
			"success": true,
		}
		if err := json.NewEncoder(w).Encode(response); err != nil {
			log.Printf("Error encoding response: %v", err)
		}
	}

	http.Handle("/notifications", auth.JWTMiddleware(authConfig.JWTSecret, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			getNotificationsHandler(w, r)
		} else if r.Method == http.MethodPut {
			putNotificationsHandler(w, r)
		} else {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		}
	})))

	// Root handler
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/" {
			w.Header().Set("Content-Type", "text/html")
			w.Write([]byte(`<html><body><h1>Options Analysis WebSocket Server</h1><p>Connect to ws://` + *host + `:` + *port + `/analyze?ticker=SYMBOL&date=YYYY-MM-DD</p><p>Get transactions: GET http://` + *host + `:` + *port + `/transactions?ticker=SYMBOL&date=YYYY-MM-DD&time=HH:MM&period=N</p></body></html>`))
		}
	})

	// TickerState tracks the state for each ticker being monitored
	type TickerState struct {
		LastFilePosition int64                       // Position of last complete line read
		CurrentPeriod    *analysis.TimePeriodSummary // Current in-progress period
		LastPeriodEnd    int64                       // Last completed period end timestamp
		WatchedFile      string                      // Path to the log file being watched
		mu               sync.Mutex                  // Mutex for thread-safe access
	}

	// State management
	tickerStates := make(map[string]*TickerState)
	statesMu := sync.RWMutex{}

	// Helper to get or create ticker state
	getTickerState := func(ticker string, dateStr string) *TickerState {
		statesMu.Lock()
		defer statesMu.Unlock()

		state, exists := tickerStates[ticker]
		if !exists {
			// Initialize state
			logFile := server.GetLogFileForTickerAndDate(*logDir, ticker, dateStr)
			state = &TickerState{
				LastFilePosition: 0,
				CurrentPeriod:    nil,
				LastPeriodEnd:    0,
				WatchedFile:      logFile,
			}
			tickerStates[ticker] = state
			log.Printf("Started monitoring log file for ticker %s: %s", ticker, logFile)

			// Do initial load to establish baseline
			go func() {
				summaries, err := server.AnalyzeTickerAndDate(*logDir, ticker, dateStr, *period)
				if err != nil {
					log.Printf("Error in initial load for ticker %s: %v", ticker, err)
					return
				}

				state.mu.Lock()
				defer state.mu.Unlock()

				// Get file size to set last position
				if fileInfo, err := os.Stat(logFile); err == nil {
					state.LastFilePosition = fileInfo.Size()
				}

				// Set up current period
				if len(summaries) > 0 {
					now := time.Now()
					periodDuration := time.Duration(*period) * time.Minute
					latestSummary := summaries[len(summaries)-1]

					if now.Sub(latestSummary.PeriodEnd) < periodDuration {
						// It's the current period
						state.CurrentPeriod = &latestSummary
					}

					// Find last completed period
					for i := len(summaries) - 1; i >= 0; i-- {
						if now.Sub(summaries[i].PeriodEnd) >= periodDuration {
							state.LastPeriodEnd = summaries[i].PeriodEnd.UnixMilli()
							break
						}
					}
				}
			}()
		}
		return state
	}

	// Create file watcher
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		log.Fatalf("Failed to create file watcher: %v", err)
	}
	defer watcher.Close()

	// Watch the log directory
	if err := watcher.Add(*logDir); err != nil {
		log.Fatalf("Failed to watch log directory: %v", err)
	}

	// Process file events
	go func() {
		for {
			select {
			case event, ok := <-watcher.Events:
				if !ok {
					return
				}

				// Only process write events
				if event.Op&fsnotify.Write == fsnotify.Write {
					// Extract ticker from filename: SYMBOL_YYYY-MM-DD.jsonl
					filename := filepath.Base(event.Name)
					if !strings.HasSuffix(filename, ".jsonl") {
						continue
					}

					// Parse ticker from filename
					parts := strings.Split(filename, "_")
					if len(parts) < 2 {
						continue
					}
					ticker := strings.ToUpper(parts[0])

					// Check if this ticker is subscribed
					subscribedTickers := wsServer.GetSubscribedTickers()
					if !subscribedTickers[ticker] {
						continue
					}

					// Get current date
					pacificTZ, _ := time.LoadLocation("America/Los_Angeles")
					dateStr := time.Now().In(pacificTZ).Format("2006-01-02")

					// Get or create state for this ticker
					state := getTickerState(ticker, dateStr)

					// Process new data
					state.mu.Lock()
					aggregates, newPosition, err := server.ReadLogFileIncremental(event.Name, state.LastFilePosition)
					if err != nil {
						log.Printf("Error reading incremental data for ticker %s: %v", ticker, err)
						state.mu.Unlock()
						continue
					}

					if len(aggregates) == 0 {
						// No new complete lines
						state.mu.Unlock()
						continue
					}

					// Update file position
					state.LastFilePosition = newPosition

					// Process aggregates
					now := time.Now()
					periodDuration := time.Duration(*period) * time.Minute

					for _, agg := range aggregates {
						// Determine which period this aggregate belongs to
						periodStart := analysis.RoundDownToPeriod(agg.StartTimestamp, *period)
						periodEnd := periodStart + int64(*period*60*1000)

						// Check if this is the current period
						periodEndTime := time.Unix(0, periodEnd*int64(time.Millisecond))
						isCurrentPeriod := now.Sub(periodEndTime) < periodDuration

						if isCurrentPeriod {
							// Update or create current period
							if state.CurrentPeriod == nil {
								// Create new current period
								state.CurrentPeriod = &analysis.TimePeriodSummary{
									PeriodStart: time.Unix(0, periodStart*int64(time.Millisecond)),
									PeriodEnd:   periodEndTime,
								}
							}

							// Check if aggregate belongs to current period
							if state.CurrentPeriod.PeriodStart.UnixMilli() == periodStart {
								// Update current period incrementally
								server.UpdatePeriodSummaryIncremental(state.CurrentPeriod, []analysis.Aggregate{agg}, *period)

								// Send update
								wsServer.SendUpdateForTicker(ticker, *state.CurrentPeriod)
							} else {
								// New period started - check if old one is complete
								oldPeriodEnd := state.CurrentPeriod.PeriodEnd.UnixMilli()
								if now.Sub(state.CurrentPeriod.PeriodEnd) >= periodDuration {
									// Old period is complete, send it
									if oldPeriodEnd > state.LastPeriodEnd {
										wsServer.SendUpdateForTicker(ticker, *state.CurrentPeriod)
										state.LastPeriodEnd = oldPeriodEnd
									}
								}

								// Start new current period
								state.CurrentPeriod = &analysis.TimePeriodSummary{
									PeriodStart: time.Unix(0, periodStart*int64(time.Millisecond)),
									PeriodEnd:   periodEndTime,
								}
								server.UpdatePeriodSummaryIncremental(state.CurrentPeriod, []analysis.Aggregate{agg}, *period)
								wsServer.SendUpdateForTicker(ticker, *state.CurrentPeriod)
							}
						} else {
							// This is a completed period - check if we need to send it
							if periodEnd > state.LastPeriodEnd {
								// Need to aggregate this period (might have multiple aggregates)
								// For now, we'll need to re-read or cache - simplified: just send if it's new
								// In a full implementation, we'd track completed periods better
								summaries, _ := server.AnalyzeTickerAndDate(*logDir, ticker, dateStr, *period)
								for i := len(summaries) - 1; i >= 0; i-- {
									if summaries[i].PeriodEnd.UnixMilli() == periodEnd {
										wsServer.SendUpdateForTicker(ticker, summaries[i])
										state.LastPeriodEnd = periodEnd
										break
									}
								}
							}
						}
					}

					state.mu.Unlock()
				}

			case err, ok := <-watcher.Errors:
				if !ok {
					return
				}
				log.Printf("File watcher error: %v", err)
			}
		}
	}()

	// Cleanup: remove ticker states when clients disconnect
	go func() {
		cleanupTicker := time.NewTicker(30 * time.Second)
		defer cleanupTicker.Stop()

		for range cleanupTicker.C {
			subscribedTickers := wsServer.GetSubscribedTickers()
			statesMu.Lock()
			for ticker := range tickerStates {
				if !subscribedTickers[ticker] {
					state := tickerStates[ticker]
					logFile := state.WatchedFile
					delete(tickerStates, ticker)
					log.Printf("Stopped monitoring log file for ticker %s: %s", ticker, logFile)
				}
			}
			statesMu.Unlock()
		}
	}()

	// Start HTTP server
	addr := fmt.Sprintf("%s:%s", *host, *port)
	log.Printf("Starting server on %s", addr)
	log.Printf("WebSocket endpoint: ws://%s/analyze", addr)
	log.Printf("Transactions endpoint: http://%s/transactions?ticker=SYMBOL&date=YYYY-MM-DD&time=HH:MM&period=N", addr)
	log.Fatal(http.ListenAndServe(addr, nil))
}
