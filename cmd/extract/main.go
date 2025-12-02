package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"github.com/ekinolik/jax-ov/internal/analysis"
)

func main() {
	// Parse command-line flags
	input := flag.String("input", "", "Input JSON file path (required)")
	timeStr := flag.String("time", "", "Start time in HH:MM format (required, e.g., 9:46)")
	period := flag.Int("period", 1, "Time period in minutes (default: 1)")
	dateStr := flag.String("date", "", "Date in YYYY-MM-DD format (optional, defaults to today)")
	flag.Parse()

	// Validate flags
	if *input == "" {
		log.Fatal("Error: --input is required")
	}

	if *timeStr == "" {
		log.Fatal("Error: --time is required (format: HH:MM, e.g., 9:46)")
	}

	if *period <= 0 {
		log.Fatal("Error: --period must be greater than 0")
	}

	// Parse time
	timeParts := strings.Split(*timeStr, ":")
	if len(timeParts) != 2 {
		log.Fatal("Error: --time must be in HH:MM format (e.g., 9:46)")
	}

	var hour, minute int
	if _, err := fmt.Sscanf(timeParts[0], "%d", &hour); err != nil {
		log.Fatalf("Error: invalid hour in time: %v", err)
	}
	if _, err := fmt.Sscanf(timeParts[1], "%d", &minute); err != nil {
		log.Fatalf("Error: invalid minute in time: %v", err)
	}

	if hour < 0 || hour > 23 {
		log.Fatal("Error: hour must be between 0 and 23")
	}
	if minute < 0 || minute > 59 {
		log.Fatal("Error: minute must be between 0 and 59")
	}

	// Load timezone (Eastern Time for market hours)
	loc, err := time.LoadLocation("America/New_York")
	if err != nil {
		log.Fatalf("Error: failed to load timezone: %v", err)
	}

	// Parse date or use today
	var date time.Time
	if *dateStr != "" {
		// Parse date string and interpret it in Eastern Time
		dateStrWithTime := *dateStr + " 00:00:00"
		date, err = time.ParseInLocation("2006-01-02 15:04:05", dateStrWithTime, loc)
		if err != nil {
			log.Fatalf("Error: invalid date format. Use YYYY-MM-DD format: %v", err)
		}
	} else {
		// Use today in Eastern Time
		now := time.Now().In(loc)
		date = time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, loc)
	}

	// Create start time in Eastern Time
	startTime := time.Date(date.Year(), date.Month(), date.Day(), hour, minute, 0, 0, loc)
	endTime := startTime.Add(time.Duration(*period) * time.Minute)

	// Convert to Unix milliseconds for comparison
	startTimestamp := startTime.UnixMilli()
	endTimestamp := endTime.UnixMilli()

	// Read input file
	data, err := os.ReadFile(*input)
	if err != nil {
		log.Fatalf("Failed to read input file: %v", err)
	}

	// Parse JSON
	var aggregates []analysis.Aggregate
	if err := json.Unmarshal(data, &aggregates); err != nil {
		log.Fatalf("Failed to parse JSON: %v", err)
	}

	// Filter aggregates within time range
	var filtered []analysis.Aggregate
	for _, agg := range aggregates {
		// Check if aggregate's start timestamp falls within the range
		if agg.StartTimestamp >= startTimestamp && agg.StartTimestamp < endTimestamp {
			filtered = append(filtered, agg)
		}
	}

	// Output filtered aggregates as JSON to stdout
	encoder := json.NewEncoder(os.Stdout)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(filtered); err != nil {
		log.Fatalf("Failed to encode JSON: %v", err)
	}
}
