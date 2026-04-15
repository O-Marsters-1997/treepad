package artifact

import (
	"context"
	"errors"
	"strings"
	"testing"
)

type fakeRunner struct {
	calls [][]string
	err   error
}

func (f *fakeRunner) Run(_ context.Context, name string, args ...string) ([]byte, error) {
	f.calls = append(f.calls, append([]string{name}, args...))
	return nil, f.err
}

func TestExecOpenerOpen(t *testing.T) {
	t.Run("empty spec does nothing", func(t *testing.T) {
		runner := &fakeRunner{}
		o := ExecOpener{Runner: runner}

		if err := o.Open(context.Background(), OpenSpec{}, OpenData{ArtifactPath: "/some/file"}); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(runner.calls) != 0 {
			t.Errorf("expected no runner calls, got %d", len(runner.calls))
		}
	})

	t.Run("renders command args and runs", func(t *testing.T) {
		runner := &fakeRunner{}
		o := ExecOpener{Runner: runner}
		spec := OpenSpec{Command: []string{"open", "{{.ArtifactPath}}"}}
		data := OpenData{ArtifactPath: "/out/repo-feat.code-workspace"}

		if err := o.Open(context.Background(), spec, data); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(runner.calls) != 1 {
			t.Fatalf("expected 1 runner call, got %d", len(runner.calls))
		}
		if runner.calls[0][0] != "open" {
			t.Errorf("command = %q, want %q", runner.calls[0][0], "open")
		}
		if runner.calls[0][1] != "/out/repo-feat.code-workspace" {
			t.Errorf("arg = %q, want the artifact path", runner.calls[0][1])
		}
	})

	t.Run("malformed template returns error", func(t *testing.T) {
		o := ExecOpener{Runner: &fakeRunner{}}
		spec := OpenSpec{Command: []string{"{{.Unclosed"}}

		err := o.Open(context.Background(), spec, OpenData{})
		if err == nil || !strings.Contains(err.Error(), "render open command arg 0") {
			t.Fatalf("got error %v, want error containing %q", err, "render open command arg 0")
		}
	})

	t.Run("propagates runner error", func(t *testing.T) {
		runner := &fakeRunner{err: errors.New("open failed")}
		o := ExecOpener{Runner: runner}
		spec := OpenSpec{Command: []string{"open", "/file"}}

		err := o.Open(context.Background(), spec, OpenData{ArtifactPath: "/file"})
		if err == nil || !strings.Contains(err.Error(), "open failed") {
			t.Fatalf("got error %v, want error containing %q", err, "open failed")
		}
	})
}
