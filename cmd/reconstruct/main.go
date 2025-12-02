package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"sort"
	"sync"
	"time"

	"github.com/ekinolik/jax-ov/internal/config"
	"github.com/ekinolik/jax-ov/internal/rest"
)

func main() {
	// Parse command-line flags
	ticker := flag.String("ticker", "", "Underlying stock ticker (required, e.g., AAPL)")
	dateStr := flag.String("date", "", "Date in YYYY-MM-DD format (required, e.g., 2025-11-30)")
	output := flag.String("output", "", "Output JSON file path (default: {ticker}_options_{date}.json)")
	workers := flag.Int("workers", 10, "Number of concurrent workers for fetching aggregates")
	flag.Parse()

	// Validate flags
	if *ticker == "" {
		log.Fatal("Error: --ticker is required")
	}

	if *dateStr == "" {
		log.Fatal("Error: --date is required")
	}

	// Parse date
	date, err := time.Parse("2006-01-02", *dateStr)
	if err != nil {
		log.Fatalf("Error: invalid date format. Use YYYY-MM-DD format: %v", err)
	}

	// Set default output filename if not provided
	if *output == "" {
		*output = fmt.Sprintf("%s_options_%s.json", *ticker, *dateStr)
	}

	// Load configuration
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("Failed to load configuration: %v", err)
	}

	// Create REST client
	restClient := rest.NewClient(cfg.APIKey)
	ctx := context.Background()

	fmt.Printf("Fetching option contracts for %s...\n", *ticker)

	// Fetch all option contracts
	contracts, err := restClient.ListOptionContracts(ctx, *ticker)
	if err != nil {
		log.Fatalf("Failed to list option contracts: %v", err)
	}

	fmt.Printf("Found %d option contracts\n", len(contracts))
	fmt.Printf("Fetching per-second aggregates for %s on %s...\n", *ticker, *dateStr)
	fmt.Printf("Using %d concurrent workers\n", *workers)

	// Channel for aggregates
	aggregatesChan := make(chan []rest.Aggregate, *workers)
	errorChan := make(chan error, *workers)

	// Worker pool to fetch aggregates concurrently
	var wg sync.WaitGroup
	semaphore := make(chan struct{}, *workers) // Limit concurrent requests

	// Process contracts in batches
	for i, contract := range contracts {
		wg.Add(1)
		go func(c rest.OptionContract, idx int) {
			defer wg.Done()
			semaphore <- struct{}{}        // Acquire semaphore
			defer func() { <-semaphore }() // Release semaphore

			if idx%100 == 0 && idx > 0 {
				fmt.Printf("Processing contract %d/%d...\n", idx, len(contracts))
			}

			aggs, err := restClient.GetOptionAggregates(ctx, c.Ticker, date)
			if err != nil {
				errorChan <- fmt.Errorf("error fetching aggregates for %s: %w", c.Ticker, err)
				return
			}

			if len(aggs) > 0 {
				aggregatesChan <- aggs
			}
		}(contract, i)
	}

	// Close channels when all workers are done
	go func() {
		wg.Wait()
		close(aggregatesChan)
		close(errorChan)
	}()

	// Collect all aggregates
	var allAggregates []rest.Aggregate
	errorCount := 0

	// Collect from channels
	go func() {
		for err := range errorChan {
			if err != nil {
				log.Printf("Warning: %v", err)
				errorCount++
			}
		}
	}()

	for aggs := range aggregatesChan {
		allAggregates = append(allAggregates, aggs...)
	}

	fmt.Printf("\nCollected %d aggregates from %d contracts", len(allAggregates), len(contracts))
	if errorCount > 0 {
		fmt.Printf(" (%d errors)", errorCount)
	}
	fmt.Println()

	// Sort aggregates by start timestamp
	fmt.Println("Sorting aggregates by timestamp...")
	sort.Slice(allAggregates, func(i, j int) bool {
		return allAggregates[i].StartTimestamp < allAggregates[j].StartTimestamp
	})

	// Write to JSON file
	fmt.Printf("Writing to %s...\n", *output)
	file, err := os.Create(*output)
	if err != nil {
		log.Fatalf("Failed to create output file: %v", err)
	}
	defer file.Close()

	encoder := json.NewEncoder(file)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(allAggregates); err != nil {
		log.Fatalf("Failed to write JSON: %v", err)
	}

	fmt.Printf("Successfully wrote %d aggregates to %s\n", len(allAggregates), *output)
}
