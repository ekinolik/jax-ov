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

// GetCurrentDayLogFile returns the log file path for the current date
func GetCurrentDayLogFile(logDir string) string {
	date := time.Now().Format("2006-01-02")
	filename := fmt.Sprintf("%s.jsonl", date)
	return filepath.Join(logDir, filename)
}

// GetLogFileForDate returns the log file path for a specific date
func GetLogFileForDate(logDir string, dateStr string) string {
	filename := fmt.Sprintf("%s.jsonl", dateStr)
	return filepath.Join(logDir, filename)
}

// AnalyzeCurrentDay reads and analyzes all aggregates for the current day
func AnalyzeCurrentDay(logDir string, periodMinutes int) ([]analysis.TimePeriodSummary, error) {
	logFile := GetCurrentDayLogFile(logDir)

	// Check if file exists
	if _, err := os.Stat(logFile); os.IsNotExist(err) {
		// Return empty results if no log file exists yet
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

// AnalyzeDate reads and analyzes all aggregates for a specific date
func AnalyzeDate(logDir string, dateStr string, periodMinutes int) ([]analysis.TimePeriodSummary, error) {
	logFile := GetLogFileForDate(logDir, dateStr)

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

// GetNewAggregatesSince reads the log file and returns aggregates with timestamps >= sinceTimestamp
func GetNewAggregatesSince(logDir string, sinceTimestamp int64) ([]analysis.Aggregate, error) {
	logFile := GetCurrentDayLogFile(logDir)

	// Check if file exists
	if _, err := os.Stat(logFile); os.IsNotExist(err) {
		return []analysis.Aggregate{}, nil
	}

	aggregates, err := ReadLogFile(logFile)
	if err != nil {
		return nil, fmt.Errorf("failed to read log file: %w", err)
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

// GetTransactionsForTimePeriod reads a log file and returns all transactions within a time period
func GetTransactionsForTimePeriod(logDir string, dateStr string, timeStr string, periodMinutes int) ([]analysis.Aggregate, error) {
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

	// Get log file path
	var logFile string
	if dateStr != "" {
		logFile = GetLogFileForDate(logDir, dateStr)
	} else {
		logFile = GetCurrentDayLogFile(logDir)
	}

	// Check if file exists
	if _, err := os.Stat(logFile); os.IsNotExist(err) {
		return []analysis.Aggregate{}, nil
	}

	// Read and filter aggregates
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
