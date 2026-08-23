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
	got := goOutput(t, "doc", "-all", ".")

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

// cliAndTUIModules are what a library caller must never be made to build.
// Importing internal packages is the facade's mechanism, so nothing about the
// exported surface would change if one of them started reaching for the command
// or TUI layer — only every consumer's build graph would.
var cliAndTUIModules = []string{
	"github.com/charmbracelet/bubbletea",
	"github.com/urfave/cli",
}

func TestPublicAPIDoesNotBuildTheCLIOrTUI(t *testing.T) {
	for _, dep := range strings.Fields(string(goOutput(t, "list", "-deps", "."))) {
		for _, mod := range cliAndTUIModules {
			if dep == mod || strings.HasPrefix(dep, mod+"/") {
				t.Errorf("root package depends on %s", dep)
			}
		}
	}
}

func goOutput(t *testing.T, args ...string) []byte {
	t.Helper()
	out, err := exec.Command("go", args...).Output()
	if err != nil {
		var exit *exec.ExitError
		if errors.As(err, &exit) {
			t.Fatalf("go %s: %v: %s", strings.Join(args, " "), err, exit.Stderr)
		}
		t.Fatalf("go %s: %v", strings.Join(args, " "), err)
	}
	return out
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
