package cwd

import (
	"fmt"
	"os"
)

// Resolve returns override when non-empty, otherwise os.Getwd.
func Resolve(override string) (string, error) {
	if override != "" {
		return override, nil
	}
	dir, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("get current directory: %w", err)
	}
	return dir, nil
}
