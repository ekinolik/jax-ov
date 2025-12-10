package notifications

import (
	"github.com/ekinolik/jax-ov/internal/analysis"
)

// EvaluateThresholds checks if a period summary triggers any notification thresholds
// Returns true if any threshold is triggered
func EvaluateThresholds(summary analysis.TimePeriodSummary, config NotificationConfig) bool {
	// Check Call Premium Threshold (independent)
	if config.CallPremiumThreshold > 0 && summary.CallPremium >= float64(config.CallPremiumThreshold) {
		return true
	}

	// Check Put Premium Threshold (independent)
	if config.PutPremiumThreshold > 0 && summary.PutPremium >= float64(config.PutPremiumThreshold) {
		return true
	}

	// Check Call Ratio Threshold (requires ratio_premium_threshold to be met)
	if config.CallRatioThreshold > 0 && config.RatioPremiumThreshold > 0 {
		if summary.TotalPremium >= float64(config.RatioPremiumThreshold) {
			// Check if call/put ratio meets threshold
			// Note: call_put_ratio = call_premium / put_premium
			// If put_premium is 0, call_put_ratio is -1 (infinite)
			if summary.CallPutRatio >= config.CallRatioThreshold {
				return true
			}
		}
	}

	// Check Put Ratio Threshold (requires ratio_premium_threshold to be met)
	if config.PutRatioThreshold > 0 && config.RatioPremiumThreshold > 0 {
		if summary.TotalPremium >= float64(config.RatioPremiumThreshold) {
			// Calculate put/call ratio (inverse of call_put_ratio)
			// put_ratio = put_premium / call_premium
			var putRatio float64
			if summary.CallPremium > 0 {
				putRatio = summary.PutPremium / summary.CallPremium
			} else if summary.PutPremium > 0 {
				// Infinite put ratio (all puts, no calls)
				putRatio = -1 // Use -1 to indicate infinite
			} else {
				putRatio = 0
			}

			if putRatio >= config.PutRatioThreshold {
				return true
			}
		}
	}

	return false
}
