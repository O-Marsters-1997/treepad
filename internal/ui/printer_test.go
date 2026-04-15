package ui

import (
	"strings"
	"testing"
)

func TestPrinterTags(t *testing.T) {
	cases := []struct {
		name    string
		fn      func(p *Printer)
		wantPfx string
	}{
		{"Step", func(p *Printer) { p.Step("doing %s", "thing") }, "[STEP] doing thing"},
		{"Info", func(p *Printer) { p.Info("context %d", 42) }, "[INFO] context 42"},
		{"OK", func(p *Printer) { p.OK("done %s", "it") }, "[OK]   done it"},
		{"Warn", func(p *Printer) { p.Warn("watch out") }, "[WARN] watch out"},
		{"Err", func(p *Printer) { p.Err("it broke") }, "[ERR]  it broke"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var b strings.Builder
			p := New(&b)
			tc.fn(p)
			got := b.String()
			if !strings.HasPrefix(got, tc.wantPfx) {
				t.Errorf("got %q, want prefix %q", got, tc.wantPfx)
			}
			if !strings.HasSuffix(got, "\n") {
				t.Errorf("output missing trailing newline: %q", got)
			}
		})
	}
}

func TestPrinterNilSafe(t *testing.T) {
	var p *Printer
	// none of these should panic
	p.Step("x")
	p.Info("x")
	p.OK("x")
	p.Warn("x")
	p.Err("x")
	p.Prompt("x")
}

func TestPrinterPromptNoNewline(t *testing.T) {
	var b strings.Builder
	p := New(&b)
	p.Prompt("continue? [y/N]: ")
	if got := b.String(); got != "continue? [y/N]: " {
		t.Errorf("got %q, want no trailing newline", got)
	}
}
