package server

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/ekinolik/jax-ov/internal/analysis"
)

// ReadLogFile reads a JSONL log file and returns all aggregates
func ReadLogFile(filename string) ([]analysis.Aggregate, error) {
	file, err := os.Open(filename)
	if err != nil {
		return nil, fmt.Errorf("failed to open log file: %w", err)
	}
	defer file.Close()

	var aggregates []analysis.Aggregate
	scanner := bufio.NewScanner(file)

	for scanner.Scan() {
		var agg analysis.Aggregate
		if err := json.Unmarshal(scanner.Bytes(), &agg); err != nil {
			// Skip invalid lines but continue processing
			continue
		}
		aggregates = append(aggregates, agg)
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("error reading log file: %w", err)
	}

	return aggregates, nil
}

// GetLogFileForTickerAndDate returns the log file path for a specific ticker and date
// Format: SYMBOL_YYYY-MM-DD.jsonl
func GetLogFileForTickerAndDate(logDir string, ticker string, dateStr string) string {
	filename := fmt.Sprintf("%s_%s.jsonl", ticker, dateStr)
	return filepath.Join(logDir, filename)
}

// GetLogFilesForDate returns all log file paths for a specific date
// With the new format, there are multiple files per date (one per symbol): SYMBOL_YYYY-MM-DD.jsonl
func GetLogFilesForDate(logDir string, dateStr string) ([]string, error) {
	var logFiles []string

	// Read all files in the log directory
	entries, err := os.ReadDir(logDir)
	if err != nil {
		return nil, fmt.Errorf("failed to read log directory: %w", err)
	}

	// Find all files matching the date pattern: *_YYYY-MM-DD.jsonl
	suffix := fmt.Sprintf("_%s.jsonl", dateStr)
	for _, entry := range entries {
		if !entry.IsDir() && strings.HasSuffix(entry.Name(), suffix) {
			logFiles = append(logFiles, filepath.Join(logDir, entry.Name()))
		}
	}

	return logFiles, nil
}

// ReadAllLogFilesForDate reads all log files for a specific date and returns combined aggregates
func ReadAllLogFilesForDate(logDir string, dateStr string) ([]analysis.Aggregate, error) {
	logFiles, err := GetLogFilesForDate(logDir, dateStr)
	if err != nil {
		return nil, err
	}

	var allAggregates []analysis.Aggregate

	// Read aggregates from all log files for this date
	for _, logFile := range logFiles {
		aggregates, err := ReadLogFile(logFile)
		if err != nil {
			// Log error but continue with other files
			continue
		}
		allAggregates = append(allAggregates, aggregates...)
	}

	return allAggregates, nil
}

// AnalyzeCurrentDay reads and analyzes all aggregates for the current day
func AnalyzeCurrentDay(logDir string, periodMinutes int) ([]analysis.TimePeriodSummary, error) {
	// Get current date in Pacific timezone
	pacificTZ, _ := time.LoadLocation("America/Los_Angeles")
	dateStr := time.Now().In(pacificTZ).Format("2006-01-02")

	return AnalyzeDate(logDir, dateStr, periodMinutes)
}

// AnalyzeDate reads and analyzes all aggregates for a specific date
// Reads all per-symbol log files for the date and combines them
func AnalyzeDate(logDir string, dateStr string, periodMinutes int) ([]analysis.TimePeriodSummary, error) {
	aggregates, err := ReadAllLogFilesForDate(logDir, dateStr)
	if err != nil {
		return nil, fmt.Errorf("failed to read log files: %w", err)
	}

	if len(aggregates) == 0 {
		return []analysis.TimePeriodSummary{}, nil
	}

	summaries, err := analysis.AggregatePremiums(aggregates, periodMinutes)
	if err != nil {
		return nil, fmt.Errorf("failed to aggregate premiums: %w", err)
	}

	return summaries, nil
}

// AnalyzeTickerAndDate reads and analyzes aggregates for a specific ticker and date
// Reads only the log file for that ticker: SYMBOL_YYYY-MM-DD.jsonl
func AnalyzeTickerAndDate(logDir string, ticker string, dateStr string, periodMinutes int) ([]analysis.TimePeriodSummary, error) {
	logFile := GetLogFileForTickerAndDate(logDir, ticker, dateStr)

	// Check if file exists
	if _, err := os.Stat(logFile); os.IsNotExist(err) {
		// Return empty results if no log file exists
		return []analysis.TimePeriodSummary{}, nil
	}

	aggregates, err := ReadLogFile(logFile)
	if err != nil {
		return nil, fmt.Errorf("failed to read log file: %w", err)
	}

	if len(aggregates) == 0 {
		return []analysis.TimePeriodSummary{}, nil
	}

	summaries, err := analysis.AggregatePremiums(aggregates, periodMinutes)
	if err != nil {
		return nil, fmt.Errorf("failed to aggregate premiums: %w", err)
	}

	return summaries, nil
}

// GetNewAggregatesSince reads all log files for the current day and returns aggregates with timestamps >= sinceTimestamp
func GetNewAggregatesSince(logDir string, sinceTimestamp int64) ([]analysis.Aggregate, error) {
	// Get current date in Pacific timezone
	pacificTZ, _ := time.LoadLocation("America/Los_Angeles")
	dateStr := time.Now().In(pacificTZ).Format("2006-01-02")

	aggregates, err := ReadAllLogFilesForDate(logDir, dateStr)
	if err != nil {
		return nil, fmt.Errorf("failed to read log files: %w", err)
	}

	// Filter aggregates with timestamp >= sinceTimestamp
	var newAggregates []analysis.Aggregate
	for _, agg := range aggregates {
		if agg.StartTimestamp >= sinceTimestamp {
			newAggregates = append(newAggregates, agg)
		}
	}

	return newAggregates, nil
}

// GetTransactionsForTickerAndTimePeriod reads a log file for a specific ticker and returns all transactions within a time period
func GetTransactionsForTickerAndTimePeriod(logDir string, ticker string, dateStr string, timeStr string, periodMinutes int) ([]analysis.Aggregate, error) {
	// Load Pacific timezone
	loc, err := time.LoadLocation("America/Los_Angeles")
	if err != nil {
		return nil, fmt.Errorf("failed to load timezone: %w", err)
	}

	// Parse time (HH:MM format)
	timeParts := strings.Split(timeStr, ":")
	if len(timeParts) != 2 {
		return nil, fmt.Errorf("invalid time format, expected HH:MM")
	}

	var hour, minute int
	if _, err := fmt.Sscanf(timeParts[0], "%d", &hour); err != nil {
		return nil, fmt.Errorf("invalid hour in time: %w", err)
	}
	if _, err := fmt.Sscanf(timeParts[1], "%d", &minute); err != nil {
		return nil, fmt.Errorf("invalid minute in time: %w", err)
	}

	if hour < 0 || hour > 23 {
		return nil, fmt.Errorf("hour must be between 0 and 23")
	}
	if minute < 0 || minute > 59 {
		return nil, fmt.Errorf("minute must be between 0 and 59")
	}

	// Parse date or use today
	var date time.Time
	if dateStr != "" {
		// Parse date string and interpret it in Pacific Time
		dateStrWithTime := dateStr + " 00:00:00"
		date, err = time.ParseInLocation("2006-01-02 15:04:05", dateStrWithTime, loc)
		if err != nil {
			return nil, fmt.Errorf("invalid date format, expected YYYY-MM-DD: %w", err)
		}
	} else {
		// Use today in Pacific Time
		now := time.Now().In(loc)
		date = time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, loc)
	}

	// Create start time in Pacific Time
	startTime := time.Date(date.Year(), date.Month(), date.Day(), hour, minute, 0, 0, loc)
	endTime := startTime.Add(time.Duration(periodMinutes) * time.Minute)

	// Convert to Unix milliseconds for comparison
	startTimestamp := startTime.UnixMilli()
	endTimestamp := endTime.UnixMilli()

	// Get date string if not provided
	if dateStr == "" {
		loc, _ := time.LoadLocation("America/Los_Angeles")
		now := time.Now().In(loc)
		dateStr = now.Format("2006-01-02")
	}

	// Get log file for the specific ticker and date
	logFile := GetLogFileForTickerAndDate(logDir, ticker, dateStr)

	// Check if file exists
	if _, err := os.Stat(logFile); os.IsNotExist(err) {
		return []analysis.Aggregate{}, nil
	}

	// Read aggregates from the ticker's log file
	aggregates, err := ReadLogFile(logFile)
	if err != nil {
		return nil, fmt.Errorf("failed to read log file: %w", err)
	}

	// Filter aggregates within time range
	var filtered []analysis.Aggregate
	for _, agg := range aggregates {
		// Check if aggregate's start timestamp falls within the range
		if agg.StartTimestamp >= startTimestamp && agg.StartTimestamp < endTimestamp {
			filtered = append(filtered, agg)
		}
	}

	return filtered, nil
}
