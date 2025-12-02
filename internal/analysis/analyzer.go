package analysis

import (
	"fmt"
	"strings"
	"time"
)

// Aggregate represents a single aggregate from the reconstructed JSON
type Aggregate struct {
	EventType         string  `json:"ev"`
	Symbol            string  `json:"sym"`
	Volume            int64   `json:"v"`
	AccumulatedVolume int64   `json:"av"`
	OfficialOpenPrice float64 `json:"op"`
	VWAP              float64 `json:"vw"`
	Open              float64 `json:"o"`
	High              float64 `json:"h"`
	Low               float64 `json:"l"`
	Close             float64 `json:"c"`
	AggregateVWAP     float64 `json:"a"`
	AverageSize       int64   `json:"z"`
	StartTimestamp    int64   `json:"s"`
	EndTimestamp      int64   `json:"e"`
}

// TimePeriodSummary represents aggregated premium data for a time period
type TimePeriodSummary struct {
	PeriodStart  time.Time `json:"period_start"`
	PeriodEnd    time.Time `json:"period_end"`
	CallPremium  float64   `json:"call_premium"`
	PutPremium   float64   `json:"put_premium"`
	TotalPremium float64   `json:"total_premium"`
	CallPutRatio float64   `json:"call_put_ratio"`
	CallVolume   int64     `json:"call_volume"`
	PutVolume    int64     `json:"put_volume"`
}

// ParseOptionType extracts the option type (call/put) from the symbol
// Format: O:{UNDERLYING}{EXPIRATION}{C|P}{STRIKE}
// Example: "O:AAPL230616C00150000" -> "call"
// Example: "O:AAPL230616P00150000" -> "put"
// The expiration date is typically 6 digits (YYMMDD), followed by C or P
func ParseOptionType(symbol string) (string, error) {
	// Remove "O:" prefix if present
	symbol = strings.TrimPrefix(symbol, "O:")

	if len(symbol) < 7 {
		return "", fmt.Errorf("invalid option symbol format: %s", symbol)
	}

	// The format is: TICKER + YYMMDD + C/P + STRIKE
	// We need to find the C or P character. It should appear after the expiration date.
	// Since ticker length varies, we'll search from the end backwards
	// looking for C or P followed by digits (the strike price)

	// Look for C or P that is followed by digits (strike price)
	for i := len(symbol) - 1; i >= 0; i-- {
		if symbol[i] == 'C' {
			// Check if followed by digits (strike price)
			if i+1 < len(symbol) && symbol[i+1] >= '0' && symbol[i+1] <= '9' {
				return "call", nil
			}
		}
		if symbol[i] == 'P' {
			// Check if followed by digits (strike price)
			if i+1 < len(symbol) && symbol[i+1] >= '0' && symbol[i+1] <= '9' {
				return "put", nil
			}
		}
	}

	return "", fmt.Errorf("could not determine option type from symbol: %s", symbol)
}

// CalculatePremium calculates premium as volume × VWAP × 100
func CalculatePremium(volume int64, vw float64) float64 {
	return float64(volume) * vw * 100
}

// RoundDownToPeriod rounds a timestamp down to the nearest N-minute boundary
func RoundDownToPeriod(timestamp int64, minutes int) int64 {
	t := time.Unix(0, timestamp*int64(time.Millisecond))

	// Calculate minutes since start of day
	totalMinutes := t.Hour()*60 + t.Minute()

	// Round down to nearest period
	roundedMinutes := (totalMinutes / minutes) * minutes

	// Create new time with rounded minutes
	rounded := time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, t.Location()).
		Add(time.Duration(roundedMinutes) * time.Minute)

	return rounded.UnixMilli()
}

// AggregatePremiums aggregates premiums by time period, separated by call/put
func AggregatePremiums(aggregates []Aggregate, periodMinutes int) ([]TimePeriodSummary, error) {
	// Map to store premiums by time period
	periodMap := make(map[int64]*TimePeriodSummary)

	for _, agg := range aggregates {
		// Determine option type
		optionType, err := ParseOptionType(agg.Symbol)
		if err != nil {
			// Skip aggregates we can't parse (log but continue)
			continue
		}

		// Calculate premium
		premium := CalculatePremium(agg.Volume, agg.VWAP)

		// Round down to time period
		periodStart := RoundDownToPeriod(agg.StartTimestamp, periodMinutes)
		periodEnd := periodStart + int64(periodMinutes*60*1000) // Add period duration in milliseconds

		// Get or create period summary
		summary, exists := periodMap[periodStart]
		if !exists {
			summary = &TimePeriodSummary{
				PeriodStart: time.Unix(0, periodStart*int64(time.Millisecond)),
				PeriodEnd:   time.Unix(0, periodEnd*int64(time.Millisecond)),
			}
			periodMap[periodStart] = summary
		}

		// Add premium and volume to appropriate type
		if optionType == "call" {
			summary.CallPremium += premium
			summary.CallVolume += agg.Volume
		} else if optionType == "put" {
			summary.PutPremium += premium
			summary.PutVolume += agg.Volume
		}

		// Update total
		summary.TotalPremium = summary.CallPremium + summary.PutPremium

		// Calculate call to put ratio
		if summary.PutPremium > 0 {
			summary.CallPutRatio = summary.CallPremium / summary.PutPremium
		} else if summary.CallPremium > 0 {
			// If put premium is 0 but call premium exists, ratio is infinity (represented as -1 or a large number)
			summary.CallPutRatio = -1 // Use -1 to indicate infinite ratio
		} else {
			summary.CallPutRatio = 0 // Both are zero
		}
	}

	// Convert map to sorted slice
	result := make([]TimePeriodSummary, 0, len(periodMap))
	for _, summary := range periodMap {
		result = append(result, *summary)
	}

	// Sort by period start time
	for i := 0; i < len(result)-1; i++ {
		for j := i + 1; j < len(result); j++ {
			if result[i].PeriodStart.After(result[j].PeriodStart) {
				result[i], result[j] = result[j], result[i]
			}
		}
	}

	return result, nil
}
