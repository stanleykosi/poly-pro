/**
 * @description
 * This file is responsible for managing the configuration for the remote-signer service.
 * It loads environment variables from a .env file and the system environment,
 * making them available to the application in a structured and type-safe format.
 *
 * Key features:
 * - Structured Config: Defines a `Config` struct to hold all configuration parameters.
 * - .env Loading: Uses the `godotenv` library to load variables from a `.env.local` file
 *   for easy local development.
 * - Validation: Includes checks to ensure that critical environment variables are set,
 *   preventing the service from starting in an invalid state.
 */

package config

import (
	"errors"
	"os"
	"path/filepath"

	"github.com/joho/godotenv"
)

// Config holds all configuration for the remote-signer application.
type Config struct {
	Port            string
	DummyPrivateKey string
}

/**
 * @description
 * LoadConfig reads configuration from environment variables and/or a .env.local file.
 *
 * @param path The path to the directory containing the .env.local file.
 * @returns A Config struct populated with the loaded values, or an error if loading fails
 *          or if required variables are missing.
 *
 * @notes
 * - This function is called once at application startup.
 * - It will return an error if `DUMMY_PRIVATE_KEY` is not set, as it's critical for
 *   the service's operation in its current mock state.
 */
func LoadConfig(path string) (config Config, err error) {
	// Attempt to load .env.local file.
	// We ignore the error if the file doesn't exist, as variables might be set directly
	// in the environment (e.g., in a Docker container or Kubernetes).
	envLocalPath := filepath.Join(path, ".env.local")
	_ = godotenv.Load(envLocalPath)

	// Read the gRPC server port.
	config.Port = os.Getenv("PORT")
	if config.Port == "" {
		// Default to 8081 if PORT is not set.
		config.Port = "8081"
	}

	// Read the dummy private key for development.
	config.DummyPrivateKey = os.Getenv("DUMMY_PRIVATE_KEY")
	if config.DummyPrivateKey == "" {
		return Config{}, errors.New("DUMMY_PRIVATE_KEY environment variable is not set")
	}

	return
}

