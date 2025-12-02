package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"strings"
	"text/tabwriter"

	"github.com/ekinolik/jax-ov/internal/analysis"
)

func main() {
	// Parse command-line flags
	input := flag.String("input", "", "Input JSON file path (required)")
	period := flag.Int("period", 5, "Time period in minutes (default: 5)")
	output := flag.String("output", "", "Optional output JSON file path")
	flag.Parse()

	// Validate flags
	if *input == "" {
		log.Fatal("Error: --input is required")
	}

	if *period <= 0 {
		log.Fatal("Error: --period must be greater than 0")
	}

	// Read input file
	fmt.Printf("Reading input file: %s\n", *input)
	data, err := os.ReadFile(*input)
	if err != nil {
		log.Fatalf("Failed to read input file: %v", err)
	}

	// Parse JSON
	var aggregates []analysis.Aggregate
	if err := json.Unmarshal(data, &aggregates); err != nil {
		log.Fatalf("Failed to parse JSON: %v", err)
	}

	fmt.Printf("Loaded %d aggregates\n", len(aggregates))
	fmt.Printf("Aggregating premiums by %d-minute periods...\n", *period)

	// Aggregate premiums
	summaries, err := analysis.AggregatePremiums(aggregates, *period)
	if err != nil {
		log.Fatalf("Failed to aggregate premiums: %v", err)
	}

	fmt.Printf("Found %d time periods\n\n", len(summaries))

	// Display table
	displayTable(summaries)

	// Write JSON output if requested
	if *output != "" {
		fmt.Printf("\nWriting results to %s...\n", *output)
		if err := writeJSONOutput(summaries, *output); err != nil {
			log.Fatalf("Failed to write JSON output: %v", err)
		}
		fmt.Printf("Successfully wrote results to %s\n", *output)
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

// formatRatio formats the call to put ratio
func formatRatio(ratio float64) string {
	if ratio < 0 {
		return "N/A" // Infinite ratio (no puts)
	}
	if ratio == 0 {
		return "0.00"
	}
	return fmt.Sprintf("%.2f", ratio)
}

// displayTable displays the premium summary in a formatted table
func displayTable(summaries []analysis.TimePeriodSummary) {
	w := tabwriter.NewWriter(os.Stdout, 0, 8, 2, ' ', tabwriter.AlignRight)

	// Header
	fmt.Fprintln(w, "Time Period\t\tCall Premium\tPut Premium\tTotal Premium\tCall/Put Ratio")
	fmt.Fprintln(w, "-------------------\t\t------------\t-----------\t-------------\t-------------")

	// Rows
	for _, summary := range summaries {
		timeStr := summary.PeriodStart.Format("2006-01-02 15:04:05")
		callFormatted := formatCurrency(summary.CallPremium)
		putFormatted := formatCurrency(summary.PutPremium)
		totalFormatted := formatCurrency(summary.TotalPremium)
		ratioFormatted := formatRatio(summary.CallPutRatio)

		// Right-justify the premium values by padding to a fixed width
		callPadded := fmt.Sprintf("%20s", "$"+callFormatted)
		putPadded := fmt.Sprintf("%19s", "$"+putFormatted)
		totalPadded := fmt.Sprintf("%21s", "$"+totalFormatted)
		ratioPadded := fmt.Sprintf("%13s", ratioFormatted)

		fmt.Fprintf(w, "%s\t\t%s\t%s\t%s\t%s\n",
			timeStr,
			callPadded,
			putPadded,
			totalPadded,
			ratioPadded)
	}

	w.Flush()
}

// writeJSONOutput writes the summaries to a JSON file
func writeJSONOutput(summaries []analysis.TimePeriodSummary, filename string) error {
	file, err := os.Create(filename)
	if err != nil {
		return err
	}
	defer file.Close()

	encoder := json.NewEncoder(file)
	encoder.SetIndent("", "  ")
	return encoder.Encode(summaries)
}
