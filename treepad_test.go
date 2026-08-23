package treepad

import (
	"context"
	"errors"
	"io"
	"os"
	"testing"

	"github.com/O-Marsters-1997/treepad/internal/hook"
	"github.com/O-Marsters-1997/treepad/internal/passthrough"
)

// The pre-flight refuses interactive hooks before any of these runners is
// reached, so nothing else in the suite would notice if the library's terminal
// runner went back to the real one. This is the second mechanism on its own:
// whatever a library path hands a subprocess, it is never a terminal.
func TestLibraryDepsRefuseATerminal(t *testing.T) {
	opened := false
	prev := passthrough.OpenTTY
	passthrough.OpenTTY = func() *os.File {
		opened = true
		return nil
	}
	t.Cleanup(func() { passthrough.OpenTTY = prev })

	d := libDeps(t.TempDir(), io.Discard)
	interactive := []hook.HookEntry{{Command: "read answer", Interactive: true}}

	err := d.HookRunner.Run(context.Background(), interactive, hook.Data{Branch: "feature/x"})
	if !errors.Is(err, ErrInteractiveHook) {
		t.Errorf("hook runner error = %v, want it to wrap ErrInteractiveHook", err)
	}
	if _, err := d.PTRunner.Run(context.Background(), "", "sh", "-c", "true"); !errors.Is(err, ErrInteractiveHook) {
		t.Errorf("passthrough runner error = %v, want it to wrap ErrInteractiveHook", err)
	}
	if opened {
		t.Error("a library dependency opened a TTY")
	}
}
