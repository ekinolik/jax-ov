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
	AppleClientID  string
	AppleTeamID    string
	ApplePrivateKey string
	JWTSecret      string
	JWTExpiryHours int
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
		AppleClientID:  clientID,
		AppleTeamID:    teamID,
		ApplePrivateKey: privateKey,
		JWTSecret:      jwtSecret,
		JWTExpiryHours: jwtExpiryHours,
	}, nil
}

// JWTExpiryDuration returns the JWT expiry as a time.Duration
func (a *AuthConfig) JWTExpiryDuration() time.Duration {
	return time.Duration(a.JWTExpiryHours) * time.Hour
}
