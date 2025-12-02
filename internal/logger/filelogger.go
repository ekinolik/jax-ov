package logger

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/ekinolik/jax-ov/internal/analysis"
)

// DailyLogger logs aggregates to daily rotating files
type DailyLogger struct {
	logDir string
}

// NewDailyLogger creates a new daily logger
func NewDailyLogger(logDir string) (*DailyLogger, error) {
	// Create log directory if it doesn't exist
	if err := os.MkdirAll(logDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create log directory: %w", err)
	}

	return &DailyLogger{
		logDir: logDir,
	}, nil
}

// getLogFilePath returns the log file path for the current date
func (l *DailyLogger) getLogFilePath() string {
	date := time.Now().Format("2006-01-02")
	filename := fmt.Sprintf("%s.jsonl", date)
	return filepath.Join(l.logDir, filename)
}

// Write writes an aggregate to the log file for the current date
// Opens, appends, and closes the file for each write
func (l *DailyLogger) Write(agg analysis.Aggregate) error {
	filePath := l.getLogFilePath()

	// Open file in append mode, create if doesn't exist
	file, err := os.OpenFile(filePath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return fmt.Errorf("failed to open log file: %w", err)
	}
	defer file.Close()

	// Encode aggregate as JSON
	encoder := json.NewEncoder(file)
	if err := encoder.Encode(agg); err != nil {
		return fmt.Errorf("failed to encode aggregate: %w", err)
	}

	return nil
}
