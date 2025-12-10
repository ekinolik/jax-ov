package notifications

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// Device represents a single device token for push notifications
type Device struct {
	Token     string    `json:"token"`
	UpdatedAt time.Time `json:"updated_at"`
	IsActive  bool      `json:"is_active"`
}

// UserDevices represents all devices for a user
type UserDevices struct {
	UserID  string   `json:"user_id"`
	Devices []Device `json:"devices"`
}

// LoadUserDevices loads device tokens for a specific user
func LoadUserDevices(sub string, dir string) (*UserDevices, error) {
	filename := filepath.Join(dir, fmt.Sprintf("%s.json", sub))

	// Check if file exists
	if _, err := os.Stat(filename); os.IsNotExist(err) {
		// Return empty devices if file doesn't exist
		return &UserDevices{
			UserID:  sub,
			Devices: []Device{},
		}, nil
	}

	data, err := os.ReadFile(filename)
	if err != nil {
		return nil, fmt.Errorf("failed to read devices file: %w", err)
	}

	var devices UserDevices
	if err := json.Unmarshal(data, &devices); err != nil {
		return nil, fmt.Errorf("failed to parse devices file: %w", err)
	}

	return &devices, nil
}

// SaveUserDevices saves device tokens for a specific user
func SaveUserDevices(sub string, dir string, devices *UserDevices) error {
	// Ensure directory exists
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create devices directory: %w", err)
	}

	filename := filepath.Join(dir, fmt.Sprintf("%s.json", sub))

	// Ensure user_id is set
	devices.UserID = sub

	data, err := json.MarshalIndent(devices, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal devices: %w", err)
	}

	if err := os.WriteFile(filename, data, 0644); err != nil {
		return fmt.Errorf("failed to write devices file: %w", err)
	}

	return nil
}

// GetActiveDeviceTokens returns all active device tokens for a user
func GetActiveDeviceTokens(devices *UserDevices) []string {
	var tokens []string
	for _, device := range devices.Devices {
		if device.IsActive {
			tokens = append(tokens, device.Token)
		}
	}
	return tokens
}

// AddOrUpdateDevice adds a new device token or updates an existing one
func AddOrUpdateDevice(devices *UserDevices, token string) {
	now := time.Now()

	// Check if device already exists
	for i := range devices.Devices {
		if devices.Devices[i].Token == token {
			// Update existing device
			devices.Devices[i].UpdatedAt = now
			devices.Devices[i].IsActive = true
			return
		}
	}

	// Add new device
	devices.Devices = append(devices.Devices, Device{
		Token:     token,
		UpdatedAt: now,
		IsActive:  true,
	})
}
