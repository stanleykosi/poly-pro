/**
 * @description
 * This file is responsible for managing the application's configuration.
 * It loads environment variables from a .env file and the system environment,
 * making them available to the rest of the application in a structured format.
 *
 * Key features:
 * - Structured Config: Defines a `Config` struct to hold all configuration parameters.
 * - .env Loading: Uses the `godotenv` library to load variables from a `.env.local` file,
 *   which is ideal for local development.
 * - Type-Safe Access: Provides a single, clear point of entry for accessing configuration values.
 */

package config

import (
	"errors"
	"os"
	"path/filepath"

	"github.com/joho/godotenv"
)

// Config holds all configuration for the application.
// Values are read from environment variables or a .env file.
type Config struct {
	Port           string
	DatabaseURL    string
	ClerkSecretKey string
	ClerkIssuerURL string // Added for JWT validation
}

/**
 * @description
 * LoadConfig reads configuration from environment variables and/or a .env.local file
 * located in the specified path.
 *
 * @param path The path to the directory containing the .env.local file.
 * @returns A Config struct populated with the loaded values, or an error if loading fails.
 *
 * @notes
 * - It first attempts to load from a .env.local file. If the file doesn't exist, it proceeds
 *   assuming environment variables are set directly (e.g., in a production environment).
 * - This function is typically called once at application startup.
 */
func LoadConfig(path string) (config Config, err error) {
	// Attempt to load environment files from the given path.
	// Try .env.local first (for local development), then fall back to .env
	// If neither exists, godotenv.Load returns an error which we can ignore,
	// as the variables might be set in the environment directly.
	envLocalPath := filepath.Join(path, ".env.local")
	envPath := filepath.Join(path, ".env")
	
	// Try .env.local first, then .env as fallback
	if err := godotenv.Load(envLocalPath); err != nil {
		_ = godotenv.Load(envPath)
	}

	// Read each environment variable into the Config struct.
	// os.Getenv returns an empty string if the variable is not set.
	// In a real production app, you might add validation here to ensure
	// critical variables are not empty.
	config.Port = os.Getenv("PORT")
	if config.Port == "" {
		// Default to 8080 if PORT is not set
		config.Port = "8080"
	}

	config.DatabaseURL = os.Getenv("DATABASE_URL")
	config.ClerkSecretKey = os.Getenv("CLERK_SECRET_KEY")
	config.ClerkIssuerURL = os.Getenv("CLERK_ISSUER_URL")

	// Validate that critical variables are not empty
	if config.DatabaseURL == "" {
		return Config{}, errors.New("DATABASE_URL is not set")
	}
	if config.ClerkSecretKey == "" {
		return Config{}, errors.New("CLERK_SECRET_KEY is not set")
	}
	if config.ClerkIssuerURL == "" {
		return Config{}, errors.New("CLERK_ISSUER_URL is not set")
	}

	return
}

