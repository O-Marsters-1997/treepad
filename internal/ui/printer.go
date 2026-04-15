// Package ui provides a structured, tag-prefixed printer for user-facing stderr output.
//
// All user-facing narrative output (progress, status, warnings) should be written
// via Printer so it is consistently tagged and goes to stderr, keeping stdout clean
// for machine-consumed payloads (__TREEPAD_CD__, JSON, config dumps).
//
// Error contract: any non-zero exit must emit exactly one [ERR] line on stderr
// describing the user-actionable problem. Individual commands return plain errors;
// the [ERR] tag is applied at the top-level boundary in cmd/tp/main.go.
package ui

import (
	"fmt"
	"io"
)

// Printer writes fixed-width tagged lines to an io.Writer.
// A nil Printer is safe to use; all calls are no-ops.
type Printer struct {
	w io.Writer
}

// New returns a Printer that writes to w.
func New(w io.Writer) *Printer {
	return &Printer{w: w}
}

// Step emits a [STEP] line for an action in progress.
func (p *Printer) Step(format string, a ...any) { p.write("[STEP] ", format, a...) }

// Info emits an [INFO] line for supplementary context.
func (p *Printer) Info(format string, a ...any) { p.write("[INFO] ", format, a...) }

// OK emits an [OK]   line for a successfully completed action.
func (p *Printer) OK(format string, a ...any) { p.write("[OK]   ", format, a...) }

// Warn emits a [WARN] line for non-fatal issues.
func (p *Printer) Warn(format string, a ...any) { p.write("[WARN] ", format, a...) }

// Err emits an [ERR]  line for fatal errors presented to the user.
func (p *Printer) Err(format string, a ...any) { p.write("[ERR]  ", format, a...) }

// Prompt writes a bare prompt string to stderr without a trailing newline or tag.
// Used for interactive confirmation prompts where a tag would look odd.
func (p *Printer) Prompt(format string, a ...any) {
	if p == nil || p.w == nil {
		return
	}
	_, _ = fmt.Fprintf(p.w, format, a...)
}

func (p *Printer) write(prefix, format string, a ...any) {
	if p == nil || p.w == nil {
		return
	}
	_, _ = fmt.Fprintf(p.w, prefix+format+"\n", a...)
}
