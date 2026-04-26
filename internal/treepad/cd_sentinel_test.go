package treepad

import (
	"bytes"
	"io"
	"strings"
	"testing"

	"treepad/internal/ui"
)

func TestEmitCD_CDSentinelPath(t *testing.T) {
	var sentinel, out bytes.Buffer
	d := Deps{
		Out:        &out,
		Log:        ui.New(io.Discard),
		CDSentinel: func() io.Writer { return &sentinel },
		IsTerminal: func(io.Writer) bool { return false },
	}
	emitCD(d, "/some/path")

	if sentinel.String() != "/some/path\n" {
		t.Errorf("sentinel = %q, want %q", sentinel.String(), "/some/path\n")
	}
	if out.Len() > 0 {
		t.Errorf("expected nothing written to d.Out; got %q", out.String())
	}
}

func TestEmitCD_FallbackToOut(t *testing.T) {
	var out bytes.Buffer
	d := Deps{
		Out:        &out,
		Log:        ui.New(io.Discard),
		CDSentinel: nil, // fd-3 probe will fail in test process
		IsTerminal: func(io.Writer) bool { return false },
	}
	emitCD(d, "/some/path")

	if !strings.Contains(out.String(), "__TREEPAD_CD__\t/some/path") {
		t.Errorf("d.Out missing payload; got %q", out.String())
	}
}
