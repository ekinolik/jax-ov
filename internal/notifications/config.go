package notifications

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// NotificationConfig represents a single notification configuration for a ticker
type NotificationConfig struct {
	Ticker                string  `json:"ticker"`
	RatioPremiumThreshold int     `json:"ratio_premium_threshold"` // Minimum total premium for ratio notifications
	CallRatioThreshold    float64 `json:"call_ratio_threshold"`    // Notify if call/put ratio >= this AND total premium >= ratio_premium_threshold
	PutRatioThreshold     float64 `json:"put_ratio_threshold"`     // Notify if put/call ratio >= this AND total premium >= ratio_premium_threshold
	CallPremiumThreshold  int     `json:"call_premium_threshold"`  // Notify if call premium >= this (independent)
	PutPremiumThreshold   int     `json:"put_premium_threshold"`   // Notify if put premium >= this (independent)
}

// UserNotifications represents all notification configurations for a user
type UserNotifications struct {
	UserID        string                        `json:"user_id"`
	Notifications map[string]NotificationConfig `json:"notifications"` // Map: ticker -> config
}

// LoadUserNotifications loads notification configurations for a specific user
func LoadUserNotifications(sub string, dir string) (*UserNotifications, error) {
	filename := filepath.Join(dir, fmt.Sprintf("%s.json", sub))

	// Check if file exists
	if _, err := os.Stat(filename); os.IsNotExist(err) {
		// Return empty config
		return &UserNotifications{
			UserID:        sub,
			Notifications: make(map[string]NotificationConfig),
		}, nil
	}

	// Read file
	data, err := os.ReadFile(filename)
	if err != nil {
		return nil, fmt.Errorf("failed to read notifications file: %w", err)
	}

	var config UserNotifications
	if err := json.Unmarshal(data, &config); err != nil {
		return nil, fmt.Errorf("failed to parse notifications file: %w", err)
	}

	// Ensure user_id matches
	config.UserID = sub

	return &config, nil
}

// SaveUserNotifications saves notification configurations for a specific user
func SaveUserNotifications(sub string, dir string, config *UserNotifications) error {
	// Ensure directory exists
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create notifications directory: %w", err)
	}

	// Ensure user_id is set
	config.UserID = sub

	filename := filepath.Join(dir, fmt.Sprintf("%s.json", sub))

	// Write file
	data, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal notifications: %w", err)
	}

	if err := os.WriteFile(filename, data, 0644); err != nil {
		return fmt.Errorf("failed to write notifications file: %w", err)
	}

	return nil
}

// LoadAllNotifications loads all notification configurations from the directory
// Returns a map: ticker -> []UserNotification (list of users with notifications for that ticker)
func LoadAllNotifications(dir string) (map[string][]UserNotification, error) {
	result := make(map[string][]UserNotification)

	// Check if directory exists
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		return result, nil
	}

	// Read all files in directory
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("failed to read notifications directory: %w", err)
	}

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		if filepath.Ext(entry.Name()) != ".json" {
			continue
		}

		// Extract user ID from filename (remove .json extension)
		sub := entry.Name()[:len(entry.Name())-5]

		// Load user notifications
		userConfig, err := LoadUserNotifications(sub, dir)
		if err != nil {
			// Log error but continue with other files
			continue
		}

		// Add each ticker notification to result
		for ticker, config := range userConfig.Notifications {
			result[ticker] = append(result[ticker], UserNotification{
				UserID: sub,
				Config: config,
			})
		}
	}

	return result, nil
}

// UserNotification represents a notification config for a specific user and ticker
type UserNotification struct {
	UserID string
	Config NotificationConfig
}
