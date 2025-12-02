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
	"time"

	"github.com/ekinolik/jax-ov/internal/config"
	"github.com/ekinolik/jax-ov/internal/websocket"
	"github.com/massive-com/client-go/v2/websocket/models"
)

func main() {
	// Parse command-line flags
	ticker := flag.String("ticker", "", "Underlying stock ticker (required, e.g., AAPL)")
	mode := flag.String("mode", "all", "Subscription mode: 'all' or 'contract' (default: 'all')")
	contract := flag.String("contract", "", "Specific option contract symbol (required if mode is 'contract')")
	flag.Parse()

	// Validate flags
	if *ticker == "" {
		log.Fatal("Error: --ticker is required")
	}

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
	var filterPrefix string // Prefix to filter by when mode is "all"
	if *mode == "all" {
		// Subscribe to all options, then filter by ticker prefix
		subscriptionTicker = "*"
		filterPrefix = fmt.Sprintf("O:%s", *ticker)
	} else {
		// Use the specific contract symbol
		subscriptionTicker = *contract
		filterPrefix = "" // No filtering needed for specific contract
	}

	// Subscribe
	if err := wsClient.Subscribe(subscriptionTicker); err != nil {
		log.Fatalf("Failed to subscribe: %v", err)
	}

	if *mode == "all" {
		fmt.Printf("Subscribed to: %s (filtering for %s*)\n", subscriptionTicker, filterPrefix)
	} else {
		fmt.Printf("Subscribed to: %s\n", subscriptionTicker)
	}
	fmt.Println("Streaming options aggregate data... (Press Ctrl+C to stop)")
	fmt.Println()

	// Set up context for graceful shutdown
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Handle interrupt signal
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	go func() {
		<-sigChan
		fmt.Println("\nShutting down...")
		cancel()
	}()

	// Define handler for incoming messages
	handler := func(agg models.EquityAgg) {
		// Filter by ticker prefix if mode is "all"
		if filterPrefix != "" && !strings.HasPrefix(agg.Symbol, filterPrefix) {
			return // Skip this message, it doesn't match our filter
		}

		printAggregate(agg)
	}

	// Run the client
	if err := wsClient.Run(ctx, handler); err != nil && err != context.Canceled {
		log.Printf("Error running WebSocket client: %v", err)
	}
}

// printAggregate prints the aggregate data in a readable format
func printAggregate(agg models.EquityAgg) {
	// Note: EquityAgg is used for options aggregates as they share the same structure
	// StartTimestamp is in Unix milliseconds
	var timestamp time.Time
	if agg.StartTimestamp > 0 {
		// Convert milliseconds to time.Time
		timestamp = time.Unix(agg.StartTimestamp/1000, (agg.StartTimestamp%1000)*int64(time.Millisecond))
	} else {
		timestamp = time.Now()
	}

	fmt.Printf("[%s] Symbol: %s | Volume: %.0f | OHLC: O=%.2f H=%.2f L=%.2f C=%.2f | VWAP: %.2f\n",
		timestamp.Format("15:04:05"),
		agg.Symbol,
		agg.Volume,
		agg.Open,
		agg.High,
		agg.Low,
		agg.Close,
		agg.VWAP,
	)
}
