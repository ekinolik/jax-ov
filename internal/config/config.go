package config

import (
	"fmt"
	"os"
	"strconv"
	"time"

	"github.com/joho/godotenv"
)

// Config holds the application configuration
type Config struct {
	APIKey string
}

// AuthConfig holds authentication configuration
type AuthConfig struct {
	AppleClientID   string
	AppleTeamID     string
	ApplePrivateKey string
	JWTSecret       string
	JWTExpiryHours  int
}

// Load loads configuration from environment variables
// It first tries to load from .env file, then reads from environment
func Load() (*Config, error) {
	// Try to load .env file (ignore error if it doesn't exist)
	_ = godotenv.Load()

	apiKey := os.Getenv("MASSIVE_API_KEY")
	if apiKey == "" {
		return nil, fmt.Errorf("MASSIVE_API_KEY environment variable is required")
	}

	return &Config{
		APIKey: apiKey,
	}, nil
}

// LoadAuth loads authentication configuration from environment variables
func LoadAuth() (*AuthConfig, error) {
	// Try to load .env file (ignore error if it doesn't exist)
	_ = godotenv.Load()

	clientID := os.Getenv("APPLE_CLIENT_ID")
	if clientID == "" {
		return nil, fmt.Errorf("APPLE_CLIENT_ID environment variable is required")
	}

	teamID := os.Getenv("APPLE_TEAM_ID")
	if teamID == "" {
		return nil, fmt.Errorf("APPLE_TEAM_ID environment variable is required")
	}

	privateKey := os.Getenv("APPLE_PRIVATE_KEY")
	if privateKey == "" {
		return nil, fmt.Errorf("APPLE_PRIVATE_KEY environment variable is required")
	}

	jwtSecret := os.Getenv("JWT_SECRET")
	if jwtSecret == "" {
		return nil, fmt.Errorf("JWT_SECRET environment variable is required")
	}

	// Default to 7 days (168 hours) if not specified
	jwtExpiryHours := 168
	if expiryStr := os.Getenv("JWT_EXPIRY_HOURS"); expiryStr != "" {
		expiry, err := strconv.Atoi(expiryStr)
		if err != nil || expiry <= 0 {
			return nil, fmt.Errorf("JWT_EXPIRY_HOURS must be a positive integer")
		}
		jwtExpiryHours = expiry
	}

	return &AuthConfig{
		AppleClientID:   clientID,
		AppleTeamID:     teamID,
		ApplePrivateKey: privateKey,
		JWTSecret:       jwtSecret,
		JWTExpiryHours:  jwtExpiryHours,
	}, nil
}

// JWTExpiryDuration returns the JWT expiry as a time.Duration
func (a *AuthConfig) JWTExpiryDuration() time.Duration {
	return time.Duration(a.JWTExpiryHours) * time.Hour
}

// APNSConfig holds APNS (Apple Push Notification Service) configuration
type APNSConfig struct {
	KeyPath     string
	KeyID       string
	TeamID      string
	Topic       string
	Environment string
}

// LoadAPNS loads APNS configuration from environment variables
func LoadAPNS() (*APNSConfig, error) {
	// Try to load .env file (ignore error if it doesn't exist)
	_ = godotenv.Load()

	keyPath := os.Getenv("APNS_KEY_PATH")
	if keyPath == "" {
		return nil, fmt.Errorf("APNS_KEY_PATH environment variable is required")
	}

	keyID := os.Getenv("APNS_KEY_ID")
	if keyID == "" {
		return nil, fmt.Errorf("APNS_KEY_ID environment variable is required")
	}

	teamID := os.Getenv("APNS_TEAM_ID")
	if teamID == "" {
		return nil, fmt.Errorf("APNS_TEAM_ID environment variable is required")
	}
	// Validate Team ID format (should be 10 characters, alphanumeric)
	if len(teamID) != 10 {
		return nil, fmt.Errorf("APNS_TEAM_ID must be a 10-character alphanumeric string (found: %q, length: %d). This should be your Apple Developer Team ID, not the team name", teamID, len(teamID))
	}

	topic := os.Getenv("APNS_TOPIC")
	if topic == "" {
		return nil, fmt.Errorf("APNS_TOPIC environment variable is required")
	}

	// Default to production if not specified
	environment := os.Getenv("APNS_ENVIRONMENT")
	if environment == "" {
		environment = "production"
	}
	if environment != "production" && environment != "sandbox" {
		return nil, fmt.Errorf("APNS_ENVIRONMENT must be 'production' or 'sandbox'")
	}

	return &APNSConfig{
		KeyPath:     keyPath,
		KeyID:       keyID,
		TeamID:      teamID,
		Topic:       topic,
		Environment: environment,
	}, nil
}
