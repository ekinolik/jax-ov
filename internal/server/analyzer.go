package server

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
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
