package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/ekinolik/jax-ov/internal/analysis"
	"github.com/ekinolik/jax-ov/internal/config"
	"github.com/ekinolik/jax-ov/internal/logger"
	"github.com/ekinolik/jax-ov/internal/websocket"
	"github.com/massive-com/client-go/v2/websocket/models"
)

func main() {
	// Parse command-line flags
	ticker := flag.String("ticker", "", "Underlying stock ticker (optional, e.g., AAPL). If not provided, logs all symbols")
	mode := flag.String("mode", "all", "Subscription mode: 'all' or 'contract' (default: 'all')")
	contract := flag.String("contract", "", "Specific option contract symbol (required if mode is 'contract')")
	logDir := flag.String("log-dir", "./logs", "Log directory path (default: ./logs)")
	flag.Parse()

	// Validate flags
	if *mode != "all" && *mode != "contract" {
		log.Fatal("Error: --mode must be either 'all' or 'contract'")
	}

	if *mode == "contract" && *contract == "" {
		log.Fatal("Error: --contract is required when --mode is 'contract'")
	}

	// Load configuration
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("Failed to load configuration: %v", err)
	}

	// Create file logger
	fileLogger, err := logger.NewDailyLogger(*logDir)
	if err != nil {
		log.Fatalf("Failed to create logger: %v", err)
	}

	// Create WebSocket client
	wsClient, err := websocket.NewClient(cfg.APIKey)
	if err != nil {
		log.Fatalf("Failed to create WebSocket client: %v", err)
	}
	defer wsClient.Close()

	// Connect to WebSocket
	if err := wsClient.Connect(); err != nil {
		log.Fatalf("Failed to connect: %v", err)
	}

	// Determine subscription ticker
	var subscriptionTicker string
	var filterTicker string // Underlying ticker to filter by (empty means log all)
	if *mode == "all" {
		// Always subscribe to all options
		subscriptionTicker = "*"
		// If ticker is provided, filter to that underlying symbol
		if *ticker != "" {
			filterTicker = strings.ToUpper(*ticker)
		}
	} else {
		// Use the specific contract symbol
		subscriptionTicker = *contract
		filterTicker = "" // No filtering needed for specific contract
	}

	// Subscribe
	if err := wsClient.Subscribe(subscriptionTicker); err != nil {
		log.Fatalf("Failed to subscribe: %v", err)
	}

	if *mode == "all" {
		if filterTicker != "" {
			fmt.Printf("Logger started - Subscribed to: %s (filtering for %s options)\n", subscriptionTicker, filterTicker)
		} else {
			fmt.Printf("Logger started - Subscribed to: %s (logging all symbols)\n", subscriptionTicker)
		}
	} else {
		fmt.Printf("Logger started - Subscribed to: %s\n", subscriptionTicker)
	}
	fmt.Printf("Logging to directory: %s\n", *logDir)
	fmt.Println("Press Ctrl+C to stop")

	// Set up context for graceful shutdown
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Handle interrupt signal
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	go func() {
		<-sigChan
		fmt.Println("\nShutting down logger...")
		cancel()
	}()

	// Define handler for incoming messages
	handler := func(agg models.EquityAgg) {
		// Convert to analysis.Aggregate format
		analysisAgg := convertToAnalysisAggregate(agg)

		// Extract underlying symbol for filtering
		if *mode == "all" && filterTicker != "" {
			underlyingSymbol, err := logger.ExtractUnderlyingSymbol(agg.Symbol)
			if err != nil {
				// Skip aggregates we can't parse
				return
			}
			// Filter by underlying ticker if specified
			if strings.ToUpper(underlyingSymbol) != filterTicker {
				return // Skip this message, it doesn't match our filter
			}
		}

		// Write to log file (will automatically route to correct symbol file)
		if err := fileLogger.Write(analysisAgg); err != nil {
			log.Printf("Error writing to log file: %v", err)
		}
	}

	// Run the client
	if err := wsClient.Run(ctx, handler); err != nil && err != context.Canceled {
		log.Printf("Error running WebSocket client: %v", err)
	}
}

// convertToAnalysisAggregate converts websocket EquityAgg to analysis.Aggregate
func convertToAnalysisAggregate(agg models.EquityAgg) analysis.Aggregate {
	return analysis.Aggregate{
		EventType:         "A",
		Symbol:            agg.Symbol,
		Volume:            int64(agg.Volume),
		AccumulatedVolume: int64(agg.AccumulatedVolume),
		OfficialOpenPrice: agg.OfficialOpenPrice,
		VWAP:              agg.VWAP,
		Open:              agg.Open,
		High:              agg.High,
		Low:               agg.Low,
		Close:             agg.Close,
		AggregateVWAP:     agg.AggregateVWAP,
		AverageSize:       int64(agg.AverageSize),
		StartTimestamp:    agg.StartTimestamp,
		EndTimestamp:      agg.EndTimestamp,
	}
}
