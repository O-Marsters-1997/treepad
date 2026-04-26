//go:build e2e

package passthrough

import "os"

// Disable the /dev/tty fallback in e2e builds so OSRunner routes subprocess
// stdio to os.Stdout/Stderr, which testscript can capture.
func init() {
	OpenTTY = func() *os.File { return nil }
}
