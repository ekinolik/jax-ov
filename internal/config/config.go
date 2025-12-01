package config

import (
	"fmt"
	"os"

	"github.com/joho/godotenv"
)

// Config holds the application configuration
type Config struct {
	APIKey string
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

