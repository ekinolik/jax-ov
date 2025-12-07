package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"sort"
	"time"

	"github.com/scmhub/calendar"
)

type TradingDaysData struct {
	GeneratedDate string   `json:"generated_date"`
	Year          int      `json:"year"`
	TradingDays   []string `json:"trading_days"`
}

func main() {
	// Parse command-line flags
	output := flag.String("output", "trading-days.json", "Output JSON file path (default: trading-days.json)")
	load := flag.String("load", "", "Load JSON file and get past N trading days")
	past := flag.Int("past", 0, "Number of past trading days to retrieve (required if --load is used)")
	flag.Parse()

	// If --load is provided, load and filter
	if *load != "" {
		if *past <= 0 {
			log.Fatal("Error: --past must be greater than 0 when using --load")
		}
		getPastTradingDays(*load, *past)
		return
	}

	// Otherwise, fetch and create trading days JSON
	fetchTradingDays(*output)
}

// fetchTradingDays fetches trading days for current and next year and saves to JSON
func fetchTradingDays(outputFile string) {
	now := time.Now()
	currentYear := now.Year()
	nextYear := currentYear + 1

	// Initialize calendar with both years to ensure holidays are calculated correctly
	cal := calendar.XNYS(currentYear, nextYear)

	var allTradingDays []string

	// Get trading days for current year
	currentYearDays := getTradingDaysForYear(cal, currentYear)
	allTradingDays = append(allTradingDays, currentYearDays...)

	// Get trading days for next year
	nextYearDays := getTradingDaysForYear(cal, nextYear)
	allTradingDays = append(allTradingDays, nextYearDays...)

	// Sort all trading days
	sort.Strings(allTradingDays)

	// Create output structure
	output := map[string]interface{}{
		"generated_date": now.Format("2006-01-02"),
		"years": map[string]interface{}{
			fmt.Sprintf("%d", currentYear): TradingDaysData{
				GeneratedDate: now.Format("2006-01-02"),
				Year:          currentYear,
				TradingDays:   currentYearDays,
			},
			fmt.Sprintf("%d", nextYear): TradingDaysData{
				GeneratedDate: now.Format("2006-01-02"),
				Year:          nextYear,
				TradingDays:   nextYearDays,
			},
		},
		"all_trading_days": allTradingDays,
	}

	// Write to JSON file
	file, err := os.Create(outputFile)
	if err != nil {
		log.Fatalf("Failed to create output file: %v", err)
	}
	defer file.Close()

	encoder := json.NewEncoder(file)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(output); err != nil {
		log.Fatalf("Failed to encode JSON: %v", err)
	}

	fmt.Printf("Generated trading days for %d and %d\n", currentYear, nextYear)
	fmt.Printf("Total trading days: %d\n", len(allTradingDays))
	fmt.Printf("Saved to: %s\n", outputFile)
}

// getTradingDaysForYear gets all trading days for a given year using the provided calendar
func getTradingDaysForYear(cal *calendar.Calendar, year int) []string {
	var tradingDays []string

	// Start from January 1st of the year
	startDate := time.Date(year, time.January, 1, 0, 0, 0, 0, time.UTC)
	// End at December 31st of the year
	endDate := time.Date(year, time.December, 31, 23, 59, 59, 999, time.UTC)

	// Iterate through each day in the year
	currentDate := startDate
	for currentDate.Before(endDate) || currentDate.Equal(endDate) {
		// Check if it's a business day (trading day = business day that's not a holiday)
		if cal.IsBusinessDay(currentDate) {
			tradingDays = append(tradingDays, currentDate.Format("2006-01-02"))
		}
		currentDate = currentDate.AddDate(0, 0, 1)
	}

	return tradingDays
}

// getPastTradingDays loads JSON file and returns the past N trading days
func getPastTradingDays(jsonFile string, n int) {
	// Read JSON file
	data, err := os.ReadFile(jsonFile)
	if err != nil {
		log.Fatalf("Failed to read JSON file: %v", err)
	}

	// Parse JSON
	var dataMap map[string]interface{}
	if err := json.Unmarshal(data, &dataMap); err != nil {
		log.Fatalf("Failed to parse JSON: %v", err)
	}

	// Get all_trading_days array
	allTradingDaysInterface, ok := dataMap["all_trading_days"]
	if !ok {
		log.Fatal("Error: 'all_trading_days' field not found in JSON")
	}

	allTradingDays, ok := allTradingDaysInterface.([]interface{})
	if !ok {
		log.Fatal("Error: 'all_trading_days' is not an array")
	}

	// Convert to string slice
	var tradingDays []string
	for _, day := range allTradingDays {
		if dayStr, ok := day.(string); ok {
			tradingDays = append(tradingDays, dayStr)
		}
	}

	// Sort to ensure chronological order
	sort.Strings(tradingDays)

	// Get today's date
	now := time.Now()
	todayStr := now.Format("2006-01-02")

	// Find today's position in the array (or the most recent trading day before/on today)
	todayIndex := -1
	for i, day := range tradingDays {
		if day <= todayStr {
			todayIndex = i
		} else {
			break
		}
	}

	// If no trading days found up to today, error
	if todayIndex == -1 {
		log.Fatal("Error: Could not find today or a past trading day in the data")
	}

	// Get past N trading days (including today if it's a trading day)
	// We want the last N trading days ending at todayIndex
	startIndex := todayIndex - n + 1
	if startIndex < 0 {
		startIndex = 0
	}

	// Ensure we don't go beyond available data
	endIndex := todayIndex + 1
	if endIndex > len(tradingDays) {
		endIndex = len(tradingDays)
	}

	pastDays := tradingDays[startIndex:endIndex]

	// Output as JSON
	encoder := json.NewEncoder(os.Stdout)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(pastDays); err != nil {
		log.Fatalf("Failed to encode JSON: %v", err)
	}
}
