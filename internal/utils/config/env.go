package config

import (
	"errors"
	"fmt"
	"os"

	"github.com/joho/godotenv"
)

// LoadEnvironment gives the local dotenv file precedence over process environment
// variables. A missing file is allowed for environment-only deployments.
func LoadEnvironment(path string) error {
	err := godotenv.Overload(path)
	if err == nil || errors.Is(err, os.ErrNotExist) {
		return nil
	}
	var pathErr *os.PathError
	if errors.As(err, &pathErr) {
		return fmt.Errorf("cannot load environment file: %w", err)
	}
	// Parser errors can contain the offending line, including secret values.
	return fmt.Errorf("invalid dotenv syntax in %q", path)
}
