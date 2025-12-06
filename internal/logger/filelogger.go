package logger

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
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

// ExtractUnderlyingSymbol extracts the underlying ticker from an option contract symbol
// Format: O:{UNDERLYING}{EXPIRATION}{C|P}{STRIKE}
// Example: O:AAPL230616C00150000 -> AAPL
func ExtractUnderlyingSymbol(symbol string) (string, error) {
	// Remove "O:" prefix if present
	symbol = strings.TrimPrefix(symbol, "O:")

	if len(symbol) < 7 {
		return "", fmt.Errorf("invalid symbol format: %s", symbol)
	}

	// Find the C or P that indicates call/put
	// It should be followed by digits (strike price)
	var callPutIndex int = -1

	for i := len(symbol) - 1; i >= 0; i-- {
		if symbol[i] == 'C' {
			if i+1 < len(symbol) && symbol[i+1] >= '0' && symbol[i+1] <= '9' {
				callPutIndex = i
				break
			}
		}
		if symbol[i] == 'P' {
			if i+1 < len(symbol) && symbol[i+1] >= '0' && symbol[i+1] <= '9' {
				callPutIndex = i
				break
			}
		}
	}

	if callPutIndex == -1 {
		return "", fmt.Errorf("could not find call/put indicator in: %s", symbol)
	}

	// Extract components
	// Everything before callPutIndex-6 is the underlying (expiration is 6 digits: YYMMDD)
	expirationStart := callPutIndex - 6
	if expirationStart < 0 {
		return "", fmt.Errorf("invalid symbol format: %s", symbol)
	}

	underlying := symbol[:expirationStart]
	return underlying, nil
}

// getLogFilePath returns the log file path for a specific underlying symbol and current date
func (l *DailyLogger) getLogFilePath(underlyingSymbol string) string {
	date := time.Now().Format("2006-01-02")
	filename := fmt.Sprintf("%s_%s.jsonl", underlyingSymbol, date)
	return filepath.Join(l.logDir, filename)
}

// Write writes an aggregate to the log file for the underlying symbol and current date
// Opens, appends, and closes the file for each write
func (l *DailyLogger) Write(agg analysis.Aggregate) error {
	// Extract underlying symbol from the aggregate
	underlyingSymbol, err := ExtractUnderlyingSymbol(agg.Symbol)
	if err != nil {
		return fmt.Errorf("failed to extract underlying symbol from %s: %w", agg.Symbol, err)
	}

	filePath := l.getLogFilePath(underlyingSymbol)

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
