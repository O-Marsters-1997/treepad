// Package launcher starts one agent in one worktree and never supervises it
// again. It stays under internal/ deliberately: an embedder with its own
// agent harness gets real process status for free and should bring its own
// spawner rather than inherit treepad's process model.
package launcher

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"text/template"

	"github.com/O-Marsters-1997/treepad/internal/slug"
)

// Data is the template context for each [batch] launch element.
type Data struct {
	Branch       string
	Slug         string
	WorktreePath string
	TicketURL    string
	Ref          string
	ActivityFile string
	Batch        string
	Chain        int
	Position     int
}

// ActivityPath returns the Activity file path for branch:
// <commonDir>/treepad/activity/<branch-slug>.log.
func ActivityPath(commonDir, branch string) string {
	return filepath.Join(commonDir, "treepad", "activity", slug.Slug(branch)+".log")
}

// Render expands each [batch] launch element against data. A nil or empty
// tmpls renders to nil, not an error — callers check len(tmpls) == 0
// themselves to decide whether to launch at all.
func Render(tmpls []string, data Data) ([]string, error) {
	if len(tmpls) == 0 {
		return nil, nil
	}
	out := make([]string, len(tmpls))
	for i, tmpl := range tmpls {
		s, err := renderOne(tmpl, data)
		if err != nil {
			return nil, fmt.Errorf("render launch[%d]: %w", i, err)
		}
		out[i] = s
	}
	return out, nil
}

func renderOne(tmpl string, data Data) (string, error) {
	t, err := template.New("launch").Parse(tmpl)
	if err != nil {
		return "", fmt.Errorf("parse: %w", err)
	}
	var buf bytes.Buffer
	if err := t.Execute(&buf, data); err != nil {
		return "", fmt.Errorf("execute: %w", err)
	}
	return buf.String(), nil
}

// Launcher starts one agent in one worktree. Treepad neither parents nor
// supervises the result afterwards — there is no supervisor loop. It exists
// as an interface so tests can fake the spawn: nothing should start a real
// process in CI.
type Launcher interface {
	// Launch starts argv detached, with dir as its working directory and
	// both stdout and stderr redirected to activityFile. It returns once the
	// process has started; it does not wait for it to finish.
	Launch(argv []string, dir, activityFile string) error
}

// ProcessLauncher is the real Launcher. It cannot reuse
// fromspec.runAgent (internal/treepad/fromspec/from_spec.go), which blocks
// on the child via a pty — this one starts the process and releases it.
type ProcessLauncher struct{}

func (ProcessLauncher) Launch(argv []string, dir, activityFile string) error {
	if len(argv) == 0 {
		return fmt.Errorf("launcher: empty command")
	}
	if err := os.MkdirAll(filepath.Dir(activityFile), 0o755); err != nil {
		return fmt.Errorf("create activity dir: %w", err)
	}
	f, err := os.OpenFile(activityFile, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return fmt.Errorf("open activity file: %w", err)
	}
	defer func() { _ = f.Close() }()

	cmd := exec.Command(argv[0], argv[1:]...)
	cmd.Dir = dir
	cmd.Stdout = f
	cmd.Stderr = f
	// Setpgid detaches the child from treepad's process group so it survives
	// tp batch sync exiting instead of receiving the same signal.
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("start %s: %w", argv[0], err)
	}
	return cmd.Process.Release()
}
