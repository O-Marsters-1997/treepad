//go:build e2e

package treepad

import "os"

// Disable the /dev/tty fallback in e2e builds so osPassthroughRunner routes
// subprocess stdio to os.Stdout/Stderr, which testscript can capture.
func init() {
	openTTY = func() *os.File { return nil }
}
