package config

import (
	"os"

	"github.com/joho/godotenv"
)

// LoadDotEnv populates the process environment from a .env file in the
// working directory when one exists. Variables already present in the
// environment (real exports, CI secrets) always win over file contents.
// A missing .env is not an error: binaries still run when the environment
// is provided externally (systemd, Docker, make test-integration).
func LoadDotEnv() {
	if _, err := os.Stat(".env"); err != nil {
		return
	}
	_ = godotenv.Load()
}
