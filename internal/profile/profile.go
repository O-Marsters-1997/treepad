// Package profile provides lightweight wall-time stage profiling.
package profile

import (
	"fmt"
	"io"
	"sort"
	"strings"
	"sync"
	"time"
)

// Profiler records wall-time stage durations.
// Call Stage to start a named stage and call the returned closure to end it.
// Use as: defer p.Stage("foo")()
type Profiler interface {
	Stage(name string) func()
	Summary(w io.Writer, totalLabel string)
}

// NewRecorder returns a production Recorder using the real clock.
func NewRecorder() *Recorder {
	return &Recorder{
		entries: make(map[string]*entry),
		nowFunc: time.Now,
	}
}

// Disabled returns a no-op Profiler that records and emits nothing.
func Disabled() Profiler { return sharedDisabled }

// OrDisabled returns p if non-nil, otherwise Disabled().
// Use at package boundaries where p may come from an unset Deps field.
func OrDisabled(p Profiler) Profiler {
	if p == nil {
		return sharedDisabled
	}
	return p
}

type entry struct {
	dur   time.Duration
	order int
}

// Recorder is a Profiler that accumulates wall-time durations per named stage.
// Repeated calls to Stage with the same name add to the existing total.
type Recorder struct {
	mu      sync.Mutex
	entries map[string]*entry
	counter int
	nowFunc func() time.Time
}

// Stage starts timing name and returns a done closure.
func (r *Recorder) Stage(name string) func() {
	start := r.nowFunc()
	return func() {
		elapsed := r.nowFunc().Sub(start)
		r.mu.Lock()
		defer r.mu.Unlock()
		if e, ok := r.entries[name]; ok {
			e.dur += elapsed
		} else {
			r.counter++
			r.entries[name] = &entry{dur: elapsed, order: r.counter}
		}
	}
}

// Summary writes a timing table to w, sorted by duration descending.
// The row with the longest duration is marked with ◀.
func (r *Recorder) Summary(w io.Writer, totalLabel string) {
	r.mu.Lock()
	type row struct {
		name  string
		dur   time.Duration
		order int
	}
	rows := make([]row, 0, len(r.entries))
	var total time.Duration
	for name, e := range r.entries {
		rows = append(rows, row{name: name, dur: e.dur, order: e.order})
		total += e.dur
	}
	r.mu.Unlock()

	if len(rows) == 0 {
		return
	}

	sort.Slice(rows, func(i, j int) bool {
		if rows[i].dur != rows[j].dur {
			return rows[i].dur > rows[j].dur
		}
		return rows[i].order < rows[j].order
	})

	nameWidth := len("total")
	for _, ro := range rows {
		if len(ro.name) > nameWidth {
			nameWidth = len(ro.name)
		}
	}

	sep := strings.Repeat("─", nameWidth) + "  " + strings.Repeat("─", 10) + "  " + strings.Repeat("─", 6)
	fmt.Fprintf(w, "\nprofile: %s (total %s)\n", totalLabel, fmtDur(total))
	fmt.Fprintf(w, "  %-*s  %-10s  %s\n", nameWidth, "stage", "duration", "   pct")
	fmt.Fprintf(w, "  %s\n", sep)
	for i, ro := range rows {
		pct := 0.0
		if total > 0 {
			pct = float64(ro.dur) / float64(total) * 100
		}
		marker := ""
		if i == 0 && len(rows) > 1 {
			marker = "  ◀"
		}
		fmt.Fprintf(w, "  %-*s  %-10s  %5.1f%%%s\n", nameWidth, ro.name, fmtDur(ro.dur), pct, marker)
	}
	fmt.Fprintf(w, "  %s\n", sep)
	fmt.Fprintf(w, "  %-*s  %-10s  100.0%%\n\n", nameWidth, "total", fmtDur(total))
}

func fmtDur(d time.Duration) string {
	return fmt.Sprintf("%.3fs", d.Seconds())
}

type noopProfiler struct{}

var disabledDone = func() {}

func (noopProfiler) Stage(_ string) func()         { return disabledDone }
func (noopProfiler) Summary(_ io.Writer, _ string) {}

var sharedDisabled Profiler = noopProfiler{}
