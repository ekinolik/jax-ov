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
	"text/tabwriter"

	"github.com/ekinolik/jax-ov/internal/analysis"
)

// ContractSummary represents aggregated premium data for a single contract
type ContractSummary struct {
	Symbol           string  `json:"symbol"`
	TotalPremium     float64 `json:"total_premium"`
	TotalVolume      int64   `json:"total_volume"`
	OptionType       string  `json:"option_type"`
	TransactionCount int     `json:"transaction_count"`
}

// ContractDetails represents parsed contract information
type ContractDetails struct {
	Underlying string
	Expiration string
	Strike     string
	Type       string
	FullSymbol string
}

func main() {
	// Parse command-line flags
	input := flag.String("input", "", "Input JSON or JSONL file path (required)")
	topN := flag.Int("top", 5, "Number of top contracts to display (default: 5)")
	output := flag.String("output", "", "Optional output JSON file path")
	flag.Parse()

	// Validate flags
	if *input == "" {
		log.Fatal("Error: --input is required")
	}

	if *topN <= 0 {
		log.Fatal("Error: --top must be greater than 0")
	}

	// Read aggregates from file
	fmt.Printf("Reading file: %s\n", *input)
	aggregates, err := readAggregates(*input)
	if err != nil {
		log.Fatalf("Failed to read file: %v", err)
	}

	fmt.Printf("Loaded %d aggregates\n", len(aggregates))
	fmt.Printf("Calculating premiums per contract...\n")

	// Group by contract and calculate total premium
	contractMap := make(map[string]*ContractSummary)

	for _, agg := range aggregates {
		// Determine option type
		optionType, err := analysis.ParseOptionType(agg.Symbol)
		if err != nil {
			// Skip aggregates we can't parse
			continue
		}

		// Get or create contract summary
		summary, exists := contractMap[agg.Symbol]
		if !exists {
			summary = &ContractSummary{
				Symbol:           agg.Symbol,
				OptionType:       optionType,
				TransactionCount: 0,
			}
			contractMap[agg.Symbol] = summary
		}

		// Calculate premium for this aggregate
		premium := analysis.CalculatePremium(agg.Volume, agg.VWAP)

		// Accumulate premium, volume, and transaction count
		summary.TotalPremium += premium
		summary.TotalVolume += agg.Volume
		summary.TransactionCount++
	}

	// Convert map to slice
	contracts := make([]ContractSummary, 0, len(contractMap))
	for _, summary := range contractMap {
		contracts = append(contracts, *summary)
	}

	// Sort by total premium descending
	sort.Slice(contracts, func(i, j int) bool {
		return contracts[i].TotalPremium > contracts[j].TotalPremium
	})

	// Take top N
	if *topN > len(contracts) {
		*topN = len(contracts)
	}
	topContracts := contracts[:*topN]

	fmt.Printf("Found %d unique contracts\n", len(contracts))
	fmt.Printf("Top %d contracts by premium:\n\n", *topN)

	// Display table
	displayTable(topContracts)

	// Write JSON output if requested
	if *output != "" {
		fmt.Printf("\nWriting results to %s...\n", *output)
		if err := writeJSONOutput(topContracts, *output); err != nil {
			log.Fatalf("Failed to write JSON output: %v", err)
		}
		fmt.Printf("Successfully wrote results to %s\n", *output)
	}
}

// readAggregates reads either JSON or JSONL format
func readAggregates(filename string) ([]analysis.Aggregate, error) {
	file, err := os.Open(filename)
	if err != nil {
		return nil, fmt.Errorf("failed to open file: %w", err)
	}
	defer file.Close()

	// Try to detect format by reading first byte
	firstByte := make([]byte, 1)
	_, err = file.Read(firstByte)
	if err != nil {
		return nil, fmt.Errorf("failed to read file: %w", err)
	}

	// Reset file pointer
	file.Seek(0, 0)

	// If first byte is '[', it's JSON array format
	if firstByte[0] == '[' {
		return readJSONArray(file)
	}

	// Otherwise, assume JSONL format
	return readJSONL(file)
}

// readJSONArray reads a JSON array format
func readJSONArray(file *os.File) ([]analysis.Aggregate, error) {
	var aggregates []analysis.Aggregate
	decoder := json.NewDecoder(file)
	if err := decoder.Decode(&aggregates); err != nil {
		return nil, fmt.Errorf("failed to parse JSON array: %w", err)
	}
	return aggregates, nil
}

// readJSONL reads a JSONL format (one JSON object per line)
func readJSONL(file *os.File) ([]analysis.Aggregate, error) {
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
		return nil, fmt.Errorf("error reading JSONL file: %w", err)
	}

	return aggregates, nil
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

// parseContractSymbol parses an option contract symbol into its components
// Format: O:{UNDERLYING}{EXPIRATION}{C|P}{STRIKE}
// Example: O:AAPL230616C00150000 -> AAPL, 2023-06-16, 150.00, CALL
func parseContractSymbol(symbol string) (ContractDetails, error) {
	// Remove "O:" prefix if present
	symbol = strings.TrimPrefix(symbol, "O:")

	if len(symbol) < 7 {
		return ContractDetails{}, fmt.Errorf("invalid symbol format: %s", symbol)
	}

	// Find the C or P that indicates call/put
	// It should be followed by digits (strike price)
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
		return ContractDetails{}, fmt.Errorf("could not find call/put indicator in: %s", symbol)
	}

	// Extract components
	// Everything before callPutIndex-6 is the underlying (expiration is 6 digits: YYMMDD)
	expirationStart := callPutIndex - 6
	if expirationStart < 0 {
		return ContractDetails{}, fmt.Errorf("invalid symbol format: %s", symbol)
	}

	underlying := symbol[:expirationStart]
	expirationStr := symbol[expirationStart:callPutIndex]
	strikeStr := symbol[callPutIndex+1:]

	// Parse expiration (YYMMDD -> YYYY-MM-DD)
	if len(expirationStr) != 6 {
		return ContractDetails{}, fmt.Errorf("invalid expiration format: %s", expirationStr)
	}

	// Parse year (assume 20XX for years 00-99, could be 19XX for very old contracts)
	year := "20" + expirationStr[0:2]
	month := expirationStr[2:4]
	day := expirationStr[4:6]
	expiration := fmt.Sprintf("%s-%s-%s", year, month, day)

	// Parse strike (option strikes are stored with last 3 digits as decimal part)
	// Example: "00220000" -> 220.000, "220500" -> 220.500
	// The strike is stored as an integer where the last 3 digits represent thousandths
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

	return ContractDetails{
		Underlying: underlying,
		Expiration: expiration,
		Strike:     strike,
		Type:       optionType,
		FullSymbol: "O:" + symbol,
	}, nil
}

// displayTable displays the top contracts in a formatted table
func displayTable(contracts []ContractSummary) {
	w := tabwriter.NewWriter(os.Stdout, 0, 4, 2, ' ', tabwriter.AlignRight)

	// Header with better spacing
	fmt.Fprintln(w, "Rank\tUnderlying\tExpiration\tStrike\tType\tTotal Premium\t\tTotal Volume\t\tTransactions")
	fmt.Fprintln(w, "----\t----------\t-----------\t------\t----\t-------------\t\t------------\t\t------------")

	// Rows
	for i, contract := range contracts {
		rank := i + 1
		premiumFormatted := formatCurrency(contract.TotalPremium)
		volumeFormatted := formatCurrency(float64(contract.TotalVolume))

		// Parse contract symbol
		details, err := parseContractSymbol(contract.Symbol)
		if err != nil {
			// If parsing fails, fall back to showing full symbol
			premiumPadded := fmt.Sprintf("%25s", "$"+premiumFormatted)
			volumePadded := fmt.Sprintf("%20s", volumeFormatted)
			fmt.Fprintf(w, "%d\t%s\t\t\t\t%s\t%s\t\t%s\t\t%d\n",
				rank,
				contract.Symbol,
				strings.ToUpper(contract.OptionType),
				premiumPadded,
				volumePadded,
				contract.TransactionCount)
			continue
		}

		// Right-justify the premium and volume values
		premiumPadded := fmt.Sprintf("%25s", "$"+premiumFormatted)
		volumePadded := fmt.Sprintf("%20s", volumeFormatted)

		fmt.Fprintf(w, "%d\t%s\t%s\t%s\t%s\t%s\t\t%s\t\t%d\n",
			rank,
			details.Underlying,
			details.Expiration,
			details.Strike,
			details.Type,
			premiumPadded,
			volumePadded,
			contract.TransactionCount)
	}

	w.Flush()
}

// writeJSONOutput writes the top contracts to a JSON file
func writeJSONOutput(contracts []ContractSummary, filename string) error {
	file, err := os.Create(filename)
	if err != nil {
		return err
	}
	defer file.Close()

	encoder := json.NewEncoder(file)
	encoder.SetIndent("", "  ")
	return encoder.Encode(contracts)
}
