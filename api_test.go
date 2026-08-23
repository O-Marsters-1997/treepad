package treepad

import (
	"bytes"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

const (
	apiGoldenPath = "testdata/api.golden"
	regenerateAPI = "run `just api-golden` and review the diff"
)

// TestPublicAPIMatchesGolden pins the whole exported surface. The facade
// imports internal packages by design, so batch's no-internal-imports rule
// cannot apply here; a snapshot catches the two things that survive that gap —
// an exported signature naming an internal type, which Go permits silently,
// and growth of the surface nobody planned.
func TestPublicAPIMatchesGolden(t *testing.T) {
	got, err := exec.Command("go", "doc", "-all", ".").Output()
	if err != nil {
		var exit *exec.ExitError
		if errors.As(err, &exit) {
			t.Fatalf("go doc -all .: %v: %s", err, exit.Stderr)
		}
		t.Fatalf("go doc -all .: %v", err)
	}

	want, err := os.ReadFile(filepath.FromSlash(apiGoldenPath))
	if err != nil {
		t.Fatalf("read %s: %v; %s", apiGoldenPath, err, regenerateAPI)
	}

	if bytes.Equal(got, want) {
		return
	}
	line, gotLine, wantLine := firstDifference(string(got), string(want))
	t.Errorf("public API differs from %s at line %d; %s\n got: %s\nwant: %s",
		apiGoldenPath, line, regenerateAPI, gotLine, wantLine)
}

// firstDifference reports the 1-based line where got and want diverge, so a CI
// log names the changed declaration instead of reprinting the whole surface.
func firstDifference(got, want string) (line int, gotLine, wantLine string) {
	gotLines, wantLines := strings.Split(got, "\n"), strings.Split(want, "\n")
	for i := range max(len(gotLines), len(wantLines)) {
		g, w := lineAt(gotLines, i), lineAt(wantLines, i)
		if g != w {
			return i + 1, g, w
		}
	}
	return 0, "", ""
}

func lineAt(lines []string, i int) string {
	if i >= len(lines) {
		return "<end of output>"
	}
	return lines[i]
}
