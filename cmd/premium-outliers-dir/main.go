package main

import (
	"bufio"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/ekinolik/jax-ov/internal/analysis"
)

func main() {
	// Parse command-line flags
	logDir := flag.String("log-dir", "", "Log directory path (required)")
	percentileFlag := flag.Float64("percentile", 90.0, "Percentile to use for outlier detection (0-100, default: 90.0)")
	multipleFlag := flag.Float64("multiple", 10.0, "Multiple of percentile to use as outlier threshold (default: 10.0)")
	flag.Parse()

	// Validate flags
	if *logDir == "" {
		log.Fatal("Error: --log-dir is required")
	}

	if *percentileFlag < 0 || *percentileFlag > 100 {
		log.Fatal("Error: --percentile must be between 0 and 100")
	}

	if *multipleFlag <= 0 {
		log.Fatal("Error: --multiple must be greater than 0")
	}

	// Convert percentile from 0-100 range to 0.0-1.0 range
	percentileValue := *percentileFlag / 100.0

	// Read all JSONL files in the directory
	files, err := os.ReadDir(*logDir)
	if err != nil {
		log.Fatalf("Failed to read log directory: %v", err)
	}

	headerPrinted := false

	// Process each file
	for _, file := range files {
		if file.IsDir() || !strings.HasSuffix(file.Name(), ".jsonl") {
			continue
		}

		filePath := filepath.Join(*logDir, file.Name())

		// Extract ticker from filename (format: TICKER_YYYY-MM-DD.jsonl)
		ticker := extractTickerFromFilename(file.Name())
		if ticker == "" {
			continue
		}

		// Read and process the file, printing findings as they're found
		findings := processFile(filePath, ticker, percentileValue, *multipleFlag)

		// Print header only once, when we have our first finding
		if len(findings) > 0 && !headerPrinted {
			printFindingsHeader()
			headerPrinted = true
		}

		// Print findings immediately
		for _, finding := range findings {
			printFinding(finding)
		}
	}
}

// Finding represents an outlier transaction finding
type Finding struct {
	Ticker     string
	Type       string // "CALL" or "PUT"
	Expiration string // "YYYY-MM-DD"
	Strike     string // Formatted strike price
	Premium    float64
	Volume     int64
	Date       string // "YYYY-MM-DD"
	Time       string // "HH:MM:SS"
	Multiple   float64
}

// extractTickerFromFilename extracts the ticker from a filename like "AAPL_2025-12-06.jsonl"
func extractTickerFromFilename(filename string) string {
	// Remove .jsonl extension
	name := strings.TrimSuffix(filename, ".jsonl")

	// Find the last underscore (separates ticker from date)
	lastUnderscore := strings.LastIndex(name, "_")
	if lastUnderscore == -1 {
		return ""
	}

	return name[:lastUnderscore]
}

// processFile processes a single log file and returns findings
func processFile(filePath, ticker string, percentileValue, multiple float64) []Finding {
	// Read JSONL file
	aggregates, err := readJSONLFile(filePath)
	if err != nil {
		// Skip files that can't be read
		return nil
	}

	if len(aggregates) == 0 {
		return nil
	}

	// Separate call and put transactions with premiums
	var callPremiums []float64
	var putPremiums []float64
	var callTransactions []TransactionWithPremium
	var putTransactions []TransactionWithPremium

	for _, agg := range aggregates {
		// Determine option type
		optionType, err := analysis.ParseOptionType(agg.Symbol)
		if err != nil {
			// Skip aggregates we can't parse
			continue
		}

		// Calculate premium
		premium := analysis.CalculatePremium(agg.Volume, agg.VWAP)

		tx := TransactionWithPremium{
			Aggregate: agg,
			Premium:   premium,
		}

		if optionType == "call" {
			callPremiums = append(callPremiums, premium)
			callTransactions = append(callTransactions, tx)
		} else if optionType == "put" {
			putPremiums = append(putPremiums, premium)
			putTransactions = append(putTransactions, tx)
		}
	}

	var findings []Finding

	// Calculate percentile and find outliers for calls
	if len(callPremiums) > 0 {
		callP := calculatePercentile(callPremiums, percentileValue)
		callOutliers := findOutliers(callTransactions, callP, multiple)
		findings = append(findings, convertToFindings(callOutliers, ticker, callP)...)
	}

	// Calculate percentile and find outliers for puts
	if len(putPremiums) > 0 {
		putP := calculatePercentile(putPremiums, percentileValue)
		putOutliers := findOutliers(putTransactions, putP, multiple)
		findings = append(findings, convertToFindings(putOutliers, ticker, putP)...)
	}

	return findings
}

// TransactionWithPremium holds an aggregate transaction with its calculated premium
type TransactionWithPremium struct {
	Aggregate analysis.Aggregate
	Premium   float64
}

// readJSONLFile reads a JSONL log file and returns all aggregates
func readJSONLFile(filename string) ([]analysis.Aggregate, error) {
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

// calculatePercentile calculates a single percentile value for a slice of premiums
func calculatePercentile(premiums []float64, p float64) float64 {
	if len(premiums) == 0 {
		return 0
	}

	// Create a copy and sort
	sorted := make([]float64, len(premiums))
	copy(sorted, premiums)
	sort.Float64s(sorted)

	return percentile(sorted, p)
}

// percentile calculates the value at the given percentile (0.0 to 1.0)
func percentile(sorted []float64, p float64) float64 {
	if len(sorted) == 0 {
		return 0
	}

	index := p * float64(len(sorted)-1)
	lower := int(index)
	upper := lower + 1

	if upper >= len(sorted) {
		return sorted[len(sorted)-1]
	}

	weight := index - float64(lower)
	return sorted[lower]*(1-weight) + sorted[upper]*weight
}

// findOutliers finds transactions where premium is >= multiplier times the threshold value
func findOutliers(transactions []TransactionWithPremium, threshold float64, multiplier float64) []TransactionWithPremium {
	if threshold == 0 {
		return nil
	}

	cutoff := threshold * multiplier
	var outliers []TransactionWithPremium

	for _, tx := range transactions {
		if tx.Premium >= cutoff {
			outliers = append(outliers, tx)
		}
	}

	return outliers
}

// OptionDetails holds parsed option contract details
type OptionDetails struct {
	Type       string // "CALL" or "PUT"
	Expiration string // "YYYY-MM-DD"
	Strike     string // Formatted strike price
}

// parseOptionSymbol parses an option contract symbol into its components
// Format: O:{UNDERLYING}{EXPIRATION}{C|P}{STRIKE}
// Example: O:AAPL230616C00150000 -> CALL, 2023-06-16, 150.00
func parseOptionSymbol(symbol string) (OptionDetails, error) {
	// Remove "O:" prefix if present
	symbol = strings.TrimPrefix(symbol, "O:")

	if len(symbol) < 7 {
		return OptionDetails{}, fmt.Errorf("invalid symbol format: %s", symbol)
	}

	// Find the C or P that indicates call/put
	var callPutIndex int = -1
	var optionType string

	for i := len(symbol) - 1; i >= 0; i-- {
		if symbol[i] == 'C' {
			if i+1 < len(symbol) && symbol[i+1] >= '0' && symbol[i+1] <= '9' {
				callPutIndex = i
				optionType = "CALL"
				break
			}
		}
		if symbol[i] == 'P' {
			if i+1 < len(symbol) && symbol[i+1] >= '0' && symbol[i+1] <= '9' {
				callPutIndex = i
				optionType = "PUT"
				break
			}
		}
	}

	if callPutIndex == -1 {
		return OptionDetails{}, fmt.Errorf("could not find call/put indicator in: %s", symbol)
	}

	// Extract components
	// Everything before callPutIndex-6 is the underlying (expiration is 6 digits: YYMMDD)
	expirationStart := callPutIndex - 6
	if expirationStart < 0 {
		return OptionDetails{}, fmt.Errorf("invalid symbol format: %s", symbol)
	}

	expirationStr := symbol[expirationStart:callPutIndex]
	strikeStr := symbol[callPutIndex+1:]

	// Parse expiration (YYMMDD -> YYYY-MM-DD)
	if len(expirationStr) != 6 {
		return OptionDetails{}, fmt.Errorf("invalid expiration format: %s", expirationStr)
	}

	year := "20" + expirationStr[0:2]
	month := expirationStr[2:4]
	day := expirationStr[4:6]
	expiration := fmt.Sprintf("%s-%s-%s", year, month, day)

	// Parse strike (option strikes are stored with last 3 digits as decimal part)
	// Example: "00150000" -> 150.000, "220500" -> 220.500
	strike := strings.TrimLeft(strikeStr, "0")
	if strike == "" {
		strike = "0"
	}

	// Pad with zeros to ensure we have at least 3 digits for decimal part
	for len(strike) < 3 {
		strike = "0" + strike
	}

	// Insert decimal point 3 digits from the right
	strike = strike[:len(strike)-3] + "." + strike[len(strike)-3:]

	// Ensure exactly 3 decimal places
	parts := strings.Split(strike, ".")
	if len(parts) == 2 {
		for len(parts[1]) < 3 {
			parts[1] += "0"
		}
		strike = parts[0] + "." + parts[1]
	}

	return OptionDetails{
		Type:       optionType,
		Expiration: expiration,
		Strike:     strike,
	}, nil
}

// convertToFindings converts transactions to Finding structs
func convertToFindings(transactions []TransactionWithPremium, ticker string, threshold float64) []Finding {
	var findings []Finding

	for _, tx := range transactions {
		// Parse option symbol
		details, err := parseOptionSymbol(tx.Aggregate.Symbol)
		if err != nil {
			// Skip if we can't parse
			continue
		}

		// Extract date and time from timestamp
		timestamp := time.Unix(0, tx.Aggregate.StartTimestamp*int64(time.Millisecond))
		date := timestamp.Format("2006-01-02")
		timeStr := timestamp.Format("15:04:05")

		// Calculate multiple
		multipleValue := tx.Premium / threshold

		findings = append(findings, Finding{
			Ticker:     ticker,
			Type:       details.Type,
			Expiration: details.Expiration,
			Strike:     details.Strike,
			Premium:    tx.Premium,
			Volume:     tx.Aggregate.Volume,
			Date:       date,
			Time:       timeStr,
			Multiple:   multipleValue,
		})
	}

	return findings
}

// printFindingsHeader prints the header for the findings table
func printFindingsHeader() {
	fmt.Printf("%-10s %-6s %-12s %-12s %-15s %-12s %-12s %-10s %-10s\n",
		"Ticker", "Type", "Expiration", "Strike", "Premium", "Volume", "Date", "Time", "Multiple")
	fmt.Printf("%s\n", strings.Repeat("-", 109))
}

// printFinding prints a single finding
func printFinding(f Finding) {
	fmt.Printf("%-10s %-6s %-12s %-12s %-15s %-12d %-12s %-10s %-10.2fx\n",
		f.Ticker,
		f.Type,
		f.Expiration,
		f.Strike,
		"$"+formatCurrency(f.Premium),
		f.Volume,
		f.Date,
		f.Time,
		f.Multiple)
}

// formatCurrency formats a float64 as currency with thousands separators
func formatCurrency(amount float64) string {
	// Format to 2 decimal places
	formatted := fmt.Sprintf("%.2f", amount)

	// Split into integer and decimal parts
	parts := strings.Split(formatted, ".")
	integerPart := parts[0]
	decimalPart := parts[1]

	// Add thousands separators
	var result strings.Builder
	length := len(integerPart)

	// Handle negative sign if present
	start := 0
	if length > 0 && integerPart[0] == '-' {
		result.WriteByte('-')
		start = 1
	}

	// Add commas every 3 digits from right to left
	for i := start; i < length; i++ {
		if i > start && (length-i)%3 == 0 {
			result.WriteByte(',')
		}
		result.WriteByte(integerPart[i])
	}

	// Add decimal part
	result.WriteByte('.')
	result.WriteString(decimalPart)

	return result.String()
}
