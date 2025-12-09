package main

import (
	"flag"
	"fmt"
	"log"
	"math/rand"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/ekinolik/jax-ov/internal/analysis"
	"github.com/ekinolik/jax-ov/internal/logger"
)

func main() {
	// Parse command-line flags
	logDir := flag.String("log-dir", "./logs", "Log directory path (default: ./logs)")
	flag.Parse()

	// Create file logger
	fileLogger, err := logger.NewDailyLogger(*logDir)
	if err != nil {
		log.Fatalf("Failed to create logger: %v", err)
	}

	// Generate contracts
	contracts := generateContracts()
	fmt.Printf("Mock logger started - Generating data for %d contracts\n", len(contracts))
	fmt.Printf("Logging to directory: %s\n", *logDir)
	fmt.Println("Press Ctrl+C to stop")

	// Set up signal handling for graceful shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	// Create ticker for 5-second intervals
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	// Initialize random number generator
	rng := rand.New(rand.NewSource(time.Now().UnixNano()))

	// Main loop
	done := make(chan bool)
	go func() {
		<-sigChan
		fmt.Println("\nShutting down mock logger...")
		done <- true
	}()

	for {
		select {
		case <-done:
			return
		case <-ticker.C:
			// Generate one aggregate per contract
			now := time.Now()
			for _, contract := range contracts {
				agg := generateFakeAggregate(contract, now, rng)
				if err := fileLogger.Write(agg); err != nil {
					log.Printf("Error writing to log file: %v", err)
				}
			}
			fmt.Printf("Generated aggregates for %d contracts at %s\n", len(contracts), now.Format("15:04:05"))
		}
	}
}

// generateContracts creates 200 contracts (10 expirations × 10 strikes × 2 types)
func generateContracts() []string {
	var contracts []string

	// Generate 10 expiration dates (30, 60, 90, 120, 150, 180, 210, 240, 270, 300 days from today)
	now := time.Now()
	expirationDays := []int{30, 60, 90, 120, 150, 180, 210, 240, 270, 300}

	// Generate 10 strike prices (100, 110, 120, 130, 140, 150, 160, 170, 180, 190)
	strikes := []float64{100, 110, 120, 130, 140, 150, 160, 170, 180, 190}

	// Generate contracts for each expiration × strike combination
	for _, days := range expirationDays {
		expDate := now.AddDate(0, 0, days)
		expStr := expDate.Format("060102") // YYMMDD format

		for _, strike := range strikes {
			// Format strike as 8 digits with last 3 as decimal
			// e.g., 150.000 -> 00150000
			strikeStr := fmt.Sprintf("%08d", int(strike*1000))

			// Create call contract
			callSymbol := fmt.Sprintf("O:TESTING%sC%s", expStr, strikeStr)
			contracts = append(contracts, callSymbol)

			// Create put contract
			putSymbol := fmt.Sprintf("O:TESTING%sP%s", expStr, strikeStr)
			contracts = append(contracts, putSymbol)
		}
	}

	return contracts
}

// generateFakeAggregate creates a fake aggregate with realistic random data
func generateFakeAggregate(symbol string, timestamp time.Time, rng *rand.Rand) analysis.Aggregate {
	// Base price around 150 with some variation
	basePrice := 150.0 + (rng.Float64()*40 - 20) // 130-170 range

	// Generate OHLC prices
	open := basePrice + (rng.Float64()*2 - 1)   // ±1 from base
	high := open + rng.Float64()*3              // 0-3 above open
	low := open - rng.Float64()*3               // 0-3 below open
	close := open + (rng.Float64()*2 - 1)       // ±1 from open

	// Ensure high is highest and low is lowest
	if high < open {
		high = open
	}
	if high < close {
		high = close
	}
	if low > open {
		low = open
	}
	if low > close {
		low = close
	}

	// Generate volume (100-10000)
	volume := int64(100 + rng.Intn(9900))

	// Calculate VWAP (simplified: average of OHLC)
	vwap := (open + high + low + close) / 4.0

	// Timestamps (1 second aggregate)
	endTimestamp := timestamp.UnixMilli()
	startTimestamp := endTimestamp - 1000 // 1 second earlier

	// Accumulated volume (cumulative)
	accumulatedVolume := volume + int64(rng.Intn(100000))

	// Average size (volume / number of trades, simplified)
	averageSize := volume / int64(1+rng.Intn(10))

	// Official open price (similar to open)
	officialOpenPrice := open + (rng.Float64()*0.5 - 0.25)

	// Aggregate VWAP (similar to VWAP)
	aggregateVWAP := vwap + (rng.Float64()*0.1 - 0.05)

	return analysis.Aggregate{
		EventType:         "A",
		Symbol:            symbol,
		Volume:            volume,
		AccumulatedVolume: accumulatedVolume,
		OfficialOpenPrice: officialOpenPrice,
		VWAP:              vwap,
		Open:              open,
		High:              high,
		Low:               low,
		Close:             close,
		AggregateVWAP:     aggregateVWAP,
		AverageSize:       averageSize,
		StartTimestamp:    startTimestamp,
		EndTimestamp:      endTimestamp,
	}
}

