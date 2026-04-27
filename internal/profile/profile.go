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

// Profiler records wall-time stage durations and optional file-transfer metrics.
// Call Stage to start a named stage and call the returned closure to end it.
// Use as: defer p.Stage("foo")()
// Call Observe after a stage to attach file/byte counts to its summary row.
type Profiler interface {
	Stage(name string) func()
	Observe(name string, files, bytes int64)
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

// IsEnabled reports whether p is an active (non-disabled) Profiler.
// Use to gate expensive metric-collection work that should only run under --profile.
func IsEnabled(p Profiler) bool {
	return p != nil && p != sharedDisabled
}

type entry struct {
	dur   time.Duration
	files int64
	bytes int64
	order int
}

// Recorder is a Profiler that accumulates wall-time durations per named stage.
// Repeated calls to Stage or Observe with the same name add to the existing total.
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

// Observe records file and byte counts for name, adding to any existing totals.
func (r *Recorder) Observe(name string, files, bytes int64) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if e, ok := r.entries[name]; ok {
		e.files += files
		e.bytes += bytes
	} else {
		r.counter++
		r.entries[name] = &entry{files: files, bytes: bytes, order: r.counter}
	}
}

// Summary writes a timing table to w, sorted by duration descending.
// The row with the longest duration is marked with ◀.
// Files and bytes columns are shown only when at least one stage has non-zero counts.
func (r *Recorder) Summary(w io.Writer, totalLabel string) {
	r.mu.Lock()
	type row struct {
		name  string
		dur   time.Duration
		files int64
		bytes int64
		order int
	}
	rows := make([]row, 0, len(r.entries))
	var total time.Duration
	var totalFiles, totalBytes int64
	for name, e := range r.entries {
		rows = append(rows, row{name: name, dur: e.dur, files: e.files, bytes: e.bytes, order: e.order})
		total += e.dur
		totalFiles += e.files
		totalBytes += e.bytes
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

	const (
		durWidth   = 10
		filesWidth = 9
		bytesWidth = 9
		pctWidth   = 6
	)

	showMetrics := totalFiles > 0 || totalBytes > 0

	var sep string
	if showMetrics {
		sep = strings.Repeat("─", nameWidth) + "  " +
			strings.Repeat("─", durWidth) + "  " +
			strings.Repeat("─", filesWidth) + "  " +
			strings.Repeat("─", bytesWidth) + "  " +
			strings.Repeat("─", pctWidth)
	} else {
		sep = strings.Repeat("─", nameWidth) + "  " +
			strings.Repeat("─", durWidth) + "  " +
			strings.Repeat("─", pctWidth)
	}

	_, _ = fmt.Fprintf(w, "\nprofile: %s (total %s)\n", totalLabel, fmtDur(total))
	if showMetrics {
		_, _ = fmt.Fprintf(w, "  %-*s  %-*s  %*s  %*s  %s\n",
			nameWidth, "stage", durWidth, "duration", filesWidth, "files", bytesWidth, "bytes", "   pct")
	} else {
		_, _ = fmt.Fprintf(w, "  %-*s  %-*s  %s\n", nameWidth, "stage", durWidth, "duration", "   pct")
	}
	_, _ = fmt.Fprintf(w, "  %s\n", sep)

	for i, ro := range rows {
		pct := 0.0
		if total > 0 {
			pct = float64(ro.dur) / float64(total) * 100
		}
		marker := ""
		if i == 0 && len(rows) > 1 {
			marker = "  ◀"
		}
		if showMetrics {
			_, _ = fmt.Fprintf(w, "  %-*s  %-*s  %*s  %*s  %5.1f%%%s\n",
				nameWidth, ro.name, durWidth, fmtDur(ro.dur),
				filesWidth, fmtCount(ro.files), bytesWidth, fmtBytes(ro.bytes),
				pct, marker)
		} else {
			_, _ = fmt.Fprintf(w, "  %-*s  %-*s  %5.1f%%%s\n", nameWidth, ro.name, durWidth, fmtDur(ro.dur), pct, marker)
		}
	}

	_, _ = fmt.Fprintf(w, "  %s\n", sep)
	if showMetrics {
		_, _ = fmt.Fprintf(w, "  %-*s  %-*s  %*s  %*s  100.0%%\n\n",
			nameWidth, "total", durWidth, fmtDur(total),
			filesWidth, fmtCount(totalFiles), bytesWidth, fmtBytes(totalBytes))
	} else {
		_, _ = fmt.Fprintf(w, "  %-*s  %-*s  100.0%%\n\n", nameWidth, "total", durWidth, fmtDur(total))
	}
}

func fmtDur(d time.Duration) string {
	if d >= time.Second {
		return fmt.Sprintf("%.3fs", d.Seconds())
	}
	return fmt.Sprintf("%dms", d.Milliseconds())
}

func fmtCount(n int64) string {
	if n == 0 {
		return "-"
	}
	s := fmt.Sprintf("%d", n)
	var b []byte
	for i := range len(s) {
		if i > 0 && (len(s)-i)%3 == 0 {
			b = append(b, ',')
		}
		b = append(b, s[i])
	}
	return string(b)
}

func fmtBytes(n int64) string {
	if n == 0 {
		return "-"
	}
	const (
		kb = 1024
		mb = 1024 * kb
		gb = 1024 * mb
		tb = 1024 * gb
	)
	switch {
	case n >= tb:
		return fmt.Sprintf("%.1f TB", float64(n)/tb)
	case n >= gb:
		return fmt.Sprintf("%.1f GB", float64(n)/gb)
	case n >= mb:
		return fmt.Sprintf("%.1f MB", float64(n)/mb)
	case n >= kb:
		return fmt.Sprintf("%.1f KB", float64(n)/kb)
	default:
		return fmt.Sprintf("%d B", n)
	}
}

type noopProfiler struct{}

var disabledDone = func() {}

func (noopProfiler) Stage(_ string) func()         { return disabledDone }
func (noopProfiler) Observe(_ string, _, _ int64)  {}
func (noopProfiler) Summary(_ io.Writer, _ string) {}

var sharedDisabled Profiler = noopProfiler{}
