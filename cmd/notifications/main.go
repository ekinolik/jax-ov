package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/ekinolik/jax-ov/internal/analysis"
	"github.com/ekinolik/jax-ov/internal/config"
	"github.com/ekinolik/jax-ov/internal/notifications"
	"github.com/ekinolik/jax-ov/internal/server"
	"github.com/fsnotify/fsnotify"
	apns2 "github.com/sideshow/apns2"
	"github.com/sideshow/apns2/token"
)

// formatNumberWithCommas formats a number with thousands separators
func formatNumberWithCommas(num float64) string {
	// Convert to integer for formatting (premiums are typically whole numbers)
	intNum := int64(num)
	str := strconv.FormatInt(intNum, 10)

	// Add commas every 3 digits from right to left
	n := len(str)
	if n <= 3 {
		return str
	}

	var result strings.Builder
	for i, char := range str {
		if i > 0 && (n-i)%3 == 0 {
			result.WriteRune(',')
		}
		result.WriteRune(char)
	}
	return result.String()
}

func main() {
	// Parse command-line flags
	logDir := flag.String("log-dir", "./logs", "Log directory path (default: ./logs)")
	notificationsDir := flag.String("notifications-dir", "./notifications", "Notifications config directory (default: ./notifications)")
	devicesDir := flag.String("devices-dir", "./devices", "Devices directory path (default: ./devices)")
	period := flag.Int("period", 5, "Analysis period in minutes (default: 5)")
	flag.Parse()

	// Load APNS configuration
	apnsConfig, err := config.LoadAPNS()
	if err != nil {
		log.Fatalf("Failed to load APNS configuration: %v", err)
	}
	log.Printf("APNS configuration loaded (topic: %s, environment: %s)", apnsConfig.Topic, apnsConfig.Environment)

	// Load APNS private key and create client
	authKey, err := token.AuthKeyFromFile(apnsConfig.KeyPath)
	if err != nil {
		log.Fatalf("Failed to load APNS key: %v", err)
	}

	apnsToken := &token.Token{
		AuthKey: authKey,
		KeyID:   apnsConfig.KeyID,
		TeamID:  apnsConfig.TeamID,
	}

	// Create APNS client
	var apnsClient *apns2.Client
	if apnsConfig.Environment == "production" {
		apnsClient = apns2.NewTokenClient(apnsToken).Production()
	} else {
		apnsClient = apns2.NewTokenClient(apnsToken).Development()
	}

	// TickerState tracks monitoring state for each ticker
	type TickerState struct {
		LastFilePosition       int64                                 // Position at end of last completed period
		NotifiedPeriods        map[string]map[int64]bool             // Map: userID -> map[periodEnd]bool (deduplication)
		MonitoringStartTime    time.Time                             // When we started monitoring this ticker
		LastProcessedPeriodEnd time.Time                             // Last period end time we processed
		CurrentPeriods         map[int64]*analysis.TimePeriodSummary // Map: periodStart -> summary (for in-progress periods)
		mu                     sync.Mutex
	}

	// State management
	tickerStates := make(map[string]*TickerState)
	statesMu := sync.RWMutex{}

	// Load all notifications and build ticker map
	loadNotifications := func() (map[string][]notifications.UserNotification, error) {
		return notifications.LoadAllNotifications(*notificationsDir)
	}

	// Get or create ticker state
	getTickerState := func(ticker string) *TickerState {
		statesMu.Lock()
		defer statesMu.Unlock()

		state, exists := tickerStates[ticker]
		if !exists {
			state = &TickerState{
				LastFilePosition:       0,
				NotifiedPeriods:        make(map[string]map[int64]bool),
				MonitoringStartTime:    time.Now(),
				LastProcessedPeriodEnd: time.Time{}, // Zero time means no period processed yet
				CurrentPeriods:         make(map[int64]*analysis.TimePeriodSummary),
			}
			tickerStates[ticker] = state
		}
		return state
	}

	// Initialize: load notifications and set up initial file positions
	allNotifications, err := loadNotifications()
	if err != nil {
		log.Fatalf("Failed to load notifications: %v", err)
	}

	log.Printf("Loaded notifications for %d tickers", len(allNotifications))

	// Initialize file positions for each ticker with notifications
	pacificTZ, _ := time.LoadLocation("America/Los_Angeles")
	dateStr := time.Now().In(pacificTZ).Format("2006-01-02")
	now := time.Now()
	periodDuration := time.Duration(*period) * time.Minute

	for ticker := range allNotifications {
		logFile := server.GetLogFileForTickerAndDate(*logDir, ticker, dateStr)
		state := getTickerState(ticker)

		// Check if file exists
		if fileInfo, err := os.Stat(logFile); err == nil {
			// Read file to find position at end of last completed period
			summaries, err := server.AnalyzeTickerAndDate(*logDir, ticker, dateStr, *period)
			if err == nil && len(summaries) > 0 {
				// Find the last completed period
				var lastCompletedPeriod *analysis.TimePeriodSummary
				for i := len(summaries) - 1; i >= 0; i-- {
					if now.Sub(summaries[i].PeriodEnd) >= periodDuration {
						lastCompletedPeriod = &summaries[i]
						break
					}
				}

				if lastCompletedPeriod != nil {
					// Find file position at end of this period
					// We'll approximate by reading the file and finding where this period ends
					// For now, set to file size (we'll refine this when processing)
					state.mu.Lock()
					state.LastFilePosition = fileInfo.Size()
					state.mu.Unlock()
					log.Printf("Initialized ticker %s: file position at %d (end of last completed period)", ticker, state.LastFilePosition)
				} else {
					// No completed periods yet, start from beginning of current period
					// Read all data to find current period start
					state.mu.Lock()
					state.LastFilePosition = 0
					state.mu.Unlock()
					log.Printf("Initialized ticker %s: no completed periods yet, starting from beginning", ticker)
				}
			} else {
				state.mu.Lock()
				state.LastFilePosition = fileInfo.Size()
				state.mu.Unlock()
			}
		}
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

	log.Printf("Watching log directory: %s", *logDir)

	// Reload notifications periodically
	go func() {
		reloadTicker := time.NewTicker(30 * time.Second)
		defer reloadTicker.Stop()

		for range reloadTicker.C {
			newNotifications, err := loadNotifications()
			if err != nil {
				log.Printf("Error reloading notifications: %v", err)
				continue
			}

			// Update ticker states (add new tickers, remove tickers with no notifications)
			statesMu.Lock()
			newTickerSet := make(map[string]bool)
			for ticker := range newNotifications {
				newTickerSet[ticker] = true
				if _, exists := tickerStates[ticker]; !exists {
					tickerStates[ticker] = &TickerState{
						LastFilePosition:    0,
						NotifiedPeriods:     make(map[string]map[int64]bool),
						MonitoringStartTime: time.Now(),
					}
					log.Printf("Started monitoring ticker %s (reload)", ticker)
				}
			}

			// Remove tickers that no longer have notifications
			for ticker := range tickerStates {
				if !newTickerSet[ticker] {
					delete(tickerStates, ticker)
					log.Printf("Stopped monitoring ticker %s (no notifications)", ticker)
				}
			}

			log.Printf("Reloaded notifications: %d tickers being monitored", len(newNotifications))
			statesMu.Unlock()
		}
	}()

	// Debounce file events to avoid processing the same file multiple times in quick succession
	type pendingFile struct {
		path      string
		ticker    string
		lastEvent time.Time
	}
	pendingFiles := make(map[string]*pendingFile)
	pendingMu := sync.Mutex{}

	// Process file events with debouncing
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

					// Debounce: only process if we haven't seen this file recently
					pendingMu.Lock()
					now := time.Now()
					pending, exists := pendingFiles[event.Name]
					if !exists {
						pending = &pendingFile{
							path:      event.Name,
							ticker:    ticker,
							lastEvent: now,
						}
						pendingFiles[event.Name] = pending
					} else {
						pending.lastEvent = now
					}
					pendingMu.Unlock()

					// Process after a short delay to batch multiple rapid writes
					go func(filePath string, fileTicker string) {
						time.Sleep(500 * time.Millisecond) // Wait 500ms to batch writes

						pendingMu.Lock()
						pending, exists := pendingFiles[filePath]
						if !exists {
							pendingMu.Unlock()
							return
						}

						// Check if this event is still recent (within last 1 second)
						if time.Since(pending.lastEvent) > 1*time.Second {
							// Too old, probably already processed
							delete(pendingFiles, filePath)
							pendingMu.Unlock()
							return
						}

						// Remove from pending so we don't process it again
						delete(pendingFiles, filePath)
						pendingMu.Unlock()

						// Check if this ticker has active notifications (reload fresh each time)
						allNotifications, err := loadNotifications()
						if err != nil {
							log.Printf("Error loading notifications in file handler: %v", err)
							return
						}
						userNotifications, hasNotifications := allNotifications[fileTicker]
						if !hasNotifications || len(userNotifications) == 0 {
							// No notifications for this ticker, skip
							return
						}

						// Get or create state for this ticker
						state := getTickerState(fileTicker)

						// Process new data
						state.mu.Lock()
						aggregates, newPosition, err := server.ReadLogFileIncremental(filePath, state.LastFilePosition)
						if err != nil {
							log.Printf("Error reading incremental data for ticker %s: %v", fileTicker, err)
							state.mu.Unlock()
							return
						}

						if len(aggregates) == 0 {
							// No new complete lines
							log.Printf("Ticker %s: No new aggregates read (position: %d -> %d)", fileTicker, state.LastFilePosition, newPosition)
							state.mu.Unlock()
							return
						}

						// Update file position
						state.LastFilePosition = newPosition

						// Process new aggregates and update period summaries incrementally
						// We need to maintain state for in-progress periods and accumulate data
						now := time.Now()

						// Process each new aggregate and add it to the appropriate period
						for _, agg := range aggregates {
							periodStart := analysis.RoundDownToPeriod(agg.StartTimestamp, *period)
							periodEnd := periodStart + int64(*period*60*1000)
							periodEndTime := time.Unix(0, periodEnd*int64(time.Millisecond))

							// Get or create period summary
							summary, exists := state.CurrentPeriods[periodStart]
							if !exists {
								// Create new period summary
								summary = &analysis.TimePeriodSummary{
									PeriodStart: time.Unix(0, periodStart*int64(time.Millisecond)),
									PeriodEnd:   periodEndTime,
								}
								state.CurrentPeriods[periodStart] = summary
							}

							// Update summary with this aggregate
							server.UpdatePeriodSummaryIncremental(summary, []analysis.Aggregate{agg}, *period)
						}

						// Convert current periods map to slice for processing
						var summaries []analysis.TimePeriodSummary
						for _, summary := range state.CurrentPeriods {
							summaries = append(summaries, *summary)
						}

						// Clean up completed periods that are old (keep only recent periods)
						// Remove periods that completed more than 2 periods ago
						cutoffTime := now.Add(-time.Duration(*period*2) * time.Minute)
						for periodStart, summary := range state.CurrentPeriods {
							if summary.PeriodEnd.Before(cutoffTime) {
								delete(state.CurrentPeriods, periodStart)
							}
						}

						// Process each period summary
						monitoringStartTime := state.MonitoringStartTime

						processedCount := 0
						evaluatedCount := 0
						triggeredCount := 0

						for _, summary := range summaries {
							periodEnd := summary.PeriodEnd.UnixMilli()
							periodEndTime := summary.PeriodEnd
							isComplete := now.After(periodEndTime) || now.Equal(periodEndTime)

							// Process both completed and in-progress periods
							// For in-progress periods, we check thresholds immediately
							// For completed periods, we also check thresholds

							// Only skip periods that completed BEFORE we started monitoring
							// This prevents sending notifications for historical periods on initial load
							if isComplete && periodEndTime.Before(monitoringStartTime) {
								continue
							}

							// For completed periods, check if we've already processed it
							// For in-progress periods, we process them every time to check for threshold changes
							if isComplete {
								if !state.LastProcessedPeriodEnd.IsZero() && !periodEndTime.After(state.LastProcessedPeriodEnd) {
									continue
								}
							}

							processedCount++
							periodStatus := "completed"
							if !isComplete {
								periodStatus = "in-progress"
							}

							// Check notifications for this period (both completed and in-progress)
							for _, userNotif := range userNotifications {
								evaluatedCount++
								// Check deduplication
								// For completed periods, we only notify once
								// For in-progress periods, we can notify multiple times if thresholds are met again
								// Use a more granular key that includes a time window for in-progress periods
								userPeriods, exists := state.NotifiedPeriods[userNotif.UserID]
								if !exists {
									userPeriods = make(map[int64]bool)
									state.NotifiedPeriods[userNotif.UserID] = userPeriods
								}

								// Determine the notification key for deduplication
								var notificationKey int64
								if isComplete {
									// For completed periods, use the period end timestamp
									notificationKey = periodEnd
									if userPeriods[notificationKey] {
										continue
									}
								} else {
									// For in-progress periods, use a 30-second window to avoid spam
									// Round down to nearest 30 seconds for the notification key
									notificationWindow := (now.Unix() / 30) * 30
									notificationKey = periodEnd + notificationWindow // Combine period end with time window
									if userPeriods[notificationKey] {
										continue
									}
								}

								// Evaluate thresholds
								thresholdsMet := notifications.EvaluateThresholds(summary, userNotif.Config)

								if thresholdsMet {
									triggeredCount++

									// Send push notification via APNS
									err := sendPushNotification(apnsClient, apnsConfig, *devicesDir, userNotif.UserID, fileTicker, periodStatus, summary)
									if err != nil {
										log.Printf("ERROR: Failed to send push notification to user %s for ticker %s: %v", userNotif.UserID, fileTicker, err)
									} else {
										log.Printf("Notification sent: User %s, Ticker %s, %s Period %s", userNotif.UserID, fileTicker, periodStatus, summary.PeriodEnd.Format("15:04:05"))
									}

									// Mark as notified using the appropriate key
									userPeriods[notificationKey] = true
								}
							}

							// Update last processed period end (only for completed periods)
							if isComplete {
								if state.LastProcessedPeriodEnd.IsZero() || periodEndTime.After(state.LastProcessedPeriodEnd) {
									state.LastProcessedPeriodEnd = periodEndTime
								}
							}
						}

						state.mu.Unlock()
					}(event.Name, ticker)
				}

			case err, ok := <-watcher.Errors:
				if !ok {
					return
				}
				log.Printf("File watcher error: %v", err)
			}
		}
	}()

	// Keep service running
	log.Printf("Notifications service started. Press Ctrl+C to stop.")
	select {} // Block forever
}

// sendPushNotification sends a push notification via APNS
func sendPushNotification(apnsClient *apns2.Client, apnsConfig *config.APNSConfig, devicesDir string, userID string, ticker string, periodStatus string, summary analysis.TimePeriodSummary) error {
	// Load user devices
	devices, err := notifications.LoadUserDevices(userID, devicesDir)
	if err != nil {
		return fmt.Errorf("failed to load devices for user %s: %w", userID, err)
	}

	// Get all active device tokens
	deviceTokens := notifications.GetActiveDeviceTokens(devices)
	if len(deviceTokens) == 0 {
		return fmt.Errorf("no active devices found for user %s", userID)
	}

	// Create notification payload with full details
	payload := map[string]interface{}{
		"aps": map[string]interface{}{
			"alert": map[string]interface{}{
				"title": fmt.Sprintf("Options Alert: %s", ticker),
				"body":  fmt.Sprintf("%s period - Call: $%.2f, Put: $%.2f, Ratio: %.2f", periodStatus, summary.CallPremium, summary.PutPremium, summary.CallPutRatio),
			},
			"sound": "default",
			"badge": 1,
		},
		"ticker":         ticker,
		"period_status":  periodStatus,
		"period_end":     summary.PeriodEnd.Format(time.RFC3339),
		"call_premium":   summary.CallPremium,
		"put_premium":    summary.PutPremium,
		"total_premium":  summary.TotalPremium,
		"call_put_ratio": summary.CallPutRatio,
		"call_volume":    summary.CallVolume,
		"put_volume":     summary.PutVolume,
	}

	payloadJSON, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to marshal notification payload: %w", err)
	}

	// Send notification to all active devices
	successCount := 0

	for _, deviceToken := range deviceTokens {
		notification := &apns2.Notification{}
		notification.DeviceToken = deviceToken
		notification.Topic = apnsConfig.Topic
		notification.Payload = payloadJSON
		notification.Priority = apns2.PriorityHigh

		// Send notification
		res, err := apnsClient.Push(notification)
		if err != nil {
			log.Printf("ERROR: Failed to send push notification to user %s: %v", userID, err)
			continue
		}

		if res.Sent() {
			successCount++
		} else {
			log.Printf("ERROR: APNS rejected notification for user %s: StatusCode=%d, Reason=%s", userID, res.StatusCode, res.Reason)
		}
	}

	// Return error if no devices were successfully notified
	if successCount == 0 {
		return fmt.Errorf("failed to send notification to any device for user %s", userID)
	}

	return nil
}
