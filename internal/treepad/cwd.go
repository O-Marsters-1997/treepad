package treepad

import (
	"fmt"
	"path/filepath"
	"strings"
)

func cwdInside(cwd, wtPath string) bool {
	rel, err := filepath.Rel(wtPath, cwd)
	return err == nil && !strings.HasPrefix(rel, "..")
}

func requireCwdInside(cwd, wtPath, msg string) error {
	if !cwdInside(cwd, wtPath) {
		return fmt.Errorf("%s", msg)
	}
	return nil
}
