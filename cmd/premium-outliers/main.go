package main

import (
	"bufio"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/ekinolik/jax-ov/internal/analysis"
)

func main() {
	// Parse command-line flags
	input := flag.String("input", "", "Input JSONL log file path (required)")
	percentileFlag := flag.Float64("percentile", 90.0, "Percentile to use for outlier detection (0-100, default: 90.0)")
	multipleFlag := flag.Float64("multiple", 10.0, "Multiple of percentile to use as outlier threshold (default: 10.0)")
	flag.Parse()

	// Validate flags
	if *input == "" {
		log.Fatal("Error: --input is required")
	}

	if *percentileFlag < 0 || *percentileFlag > 100 {
		log.Fatal("Error: --percentile must be between 0 and 100")
	}

	if *multipleFlag <= 0 {
		log.Fatal("Error: --multiple must be greater than 0")
	}

	// Convert percentile from 0-100 range to 0.0-1.0 range
	percentileValue := *percentileFlag / 100.0

	// Read JSONL file
	fmt.Printf("Reading log file: %s\n", *input)
	aggregates, err := readJSONLFile(*input)
	if err != nil {
		log.Fatalf("Failed to read log file: %v", err)
	}

	fmt.Printf("Loaded %d aggregates\n", len(aggregates))

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

	// Calculate standard percentiles (p25, p50, p75, p90, p99)
	callP25, callP50, callP75, callP90, callP99 := calculatePercentiles(callPremiums)
	putP25, putP50, putP75, putP90, putP99 := calculatePercentiles(putPremiums)

	// Calculate the requested percentile for outlier detection
	callRequestedP := calculatePercentile(callPremiums, percentileValue)
	putRequestedP := calculatePercentile(putPremiums, percentileValue)

	// Print statistics
	fmt.Printf("\n=== Premium Statistics ===\n")
	fmt.Printf("Call Premiums:\n")
	fmt.Printf("  P25: $%s\n", formatCurrency(callP25))
	fmt.Printf("  P50 (Median): $%s\n", formatCurrency(callP50))
	fmt.Printf("  P75: $%s\n", formatCurrency(callP75))
	fmt.Printf("  P90: $%s\n", formatCurrency(callP90))
	fmt.Printf("  P99: $%s\n", formatCurrency(callP99))
	fmt.Printf("  P%.1f: $%s\n", *percentileFlag, formatCurrency(callRequestedP))
	fmt.Printf("  Total Transactions: %d\n", len(callPremiums))

	fmt.Printf("\nPut Premiums:\n")
	fmt.Printf("  P25: $%s\n", formatCurrency(putP25))
	fmt.Printf("  P50 (Median): $%s\n", formatCurrency(putP50))
	fmt.Printf("  P75: $%s\n", formatCurrency(putP75))
	fmt.Printf("  P90: $%s\n", formatCurrency(putP90))
	fmt.Printf("  P99: $%s\n", formatCurrency(putP99))
	fmt.Printf("  P%.1f: $%s\n", *percentileFlag, formatCurrency(putRequestedP))
	fmt.Printf("  Total Transactions: %d\n", len(putPremiums))

	// Find outliers using requested percentile and multiple
	fmt.Printf("\n=== Outliers (%.1fx P%.1f) ===\n", *multipleFlag, *percentileFlag)
	callOutliers := findOutliers(callTransactions, callRequestedP, *multipleFlag)
	putOutliers := findOutliers(putTransactions, putRequestedP, *multipleFlag)

	if len(callOutliers) > 0 {
		fmt.Printf("\nCall Premium Outliers (≥%.1fx P%.1f):\n", *multipleFlag, *percentileFlag)
		printOutliers(callOutliers, callRequestedP)
	} else {
		fmt.Printf("\nNo call premium outliers found (≥%.1fx P%.1f)\n", *multipleFlag, *percentileFlag)
	}

	if len(putOutliers) > 0 {
		fmt.Printf("\nPut Premium Outliers (≥%.1fx P%.1f):\n", *multipleFlag, *percentileFlag)
		printOutliers(putOutliers, putRequestedP)
	} else {
		fmt.Printf("\nNo put premium outliers found (≥%.1fx P%.1f)\n", *multipleFlag, *percentileFlag)
	}
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

// calculatePercentiles calculates p25, p50 (median), p75, p90, and p99 for a slice of premiums
func calculatePercentiles(premiums []float64) (p25, p50, p75, p90, p99 float64) {
	if len(premiums) == 0 {
		return 0, 0, 0, 0, 0
	}

	// Create a copy and sort
	sorted := make([]float64, len(premiums))
	copy(sorted, premiums)
	sort.Float64s(sorted)

	// Calculate percentiles
	p25 = percentile(sorted, 0.25)
	p50 = percentile(sorted, 0.50)
	p75 = percentile(sorted, 0.75)
	p90 = percentile(sorted, 0.90)
	p99 = percentile(sorted, 0.99)

	return p25, p50, p75, p90, p99
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

// printOutliers prints outlier transactions in a formatted table
func printOutliers(outliers []TransactionWithPremium, threshold float64) {
	// Sort by premium descending
	sort.Slice(outliers, func(i, j int) bool {
		return outliers[i].Premium > outliers[j].Premium
	})

	fmt.Printf("  %-6s %-12s %-12s %-15s %-12s %-10s %-12s %-10s\n",
		"Type", "Expiration", "Strike", "Premium", "Volume", "VWAP", "Timestamp", "Multiple")
	fmt.Printf("  %s\n", strings.Repeat("-", 100))

	for _, tx := range outliers {
		multiple := tx.Premium / threshold
		timestamp := time.Unix(0, tx.Aggregate.StartTimestamp*int64(time.Millisecond))
		timeStr := timestamp.Format("15:04:05")

		// Parse option symbol
		details, err := parseOptionSymbol(tx.Aggregate.Symbol)
		if err != nil {
			// If parsing fails, fall back to showing the raw symbol
			fmt.Printf("  %-6s %-12s %-12s %-15s %-12d %-10.2f %-12s %-10.2fx\n",
				"ERROR",
				"N/A",
				"N/A",
				"$"+formatCurrency(tx.Premium),
				tx.Aggregate.Volume,
				tx.Aggregate.VWAP,
				timeStr,
				multiple)
			continue
		}

		fmt.Printf("  %-6s %-12s %-12s %-15s %-12d %-10.2f %-12s %-10.2fx\n",
			details.Type,
			details.Expiration,
			details.Strike,
			"$"+formatCurrency(tx.Premium),
			tx.Aggregate.Volume,
			tx.Aggregate.VWAP,
			timeStr,
			multiple)
	}
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
