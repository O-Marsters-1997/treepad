package profile

import (
	"bytes"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestRecorder_Stage(t *testing.T) {
	tests := []struct {
		name    string
		step    time.Duration
		calls   int
		wantDur time.Duration
	}{
		{"single", 100 * time.Millisecond, 1, 100 * time.Millisecond},
		{"accumulates_same_name", 50 * time.Millisecond, 2, 100 * time.Millisecond},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := &Recorder{
				entries: make(map[string]*entry),
				nowFunc: stepClock(tt.step),
			}
			for range tt.calls {
				r.Stage("s")()
			}
			if got := r.entries["s"].dur; got != tt.wantDur {
				t.Errorf("dur = %v, want %v", got, tt.wantDur)
			}
		})
	}

	t.Run("concurrent", func(t *testing.T) {
		r := NewRecorder()
		var wg sync.WaitGroup
		for range 50 {
			wg.Go(func() { r.Stage("x")() })
		}
		wg.Wait()
		if r.entries["x"].dur == 0 {
			t.Error("expected non-zero accumulated duration after concurrent calls")
		}
	})
}

func TestRecorder_Summary(t *testing.T) {
	tests := []struct {
		name    string
		entries map[string]*entry
		counter int
		label   string
		check   func(t *testing.T, out string)
	}{
		{
			name:  "empty_recorder_writes_nothing",
			label: "test",
			check: func(t *testing.T, out string) {
				if out != "" {
					t.Errorf("expected empty output, got %q", out)
				}
			},
		},
		{
			name:    "contains_expected_content",
			entries: map[string]*entry{"file_sync": {dur: 2 * time.Second, order: 1}},
			counter: 1,
			label:   "new feat/foo",
			check: func(t *testing.T, out string) {
				for _, want := range []string{"profile:", "new feat/foo", "file_sync", "total", "2.000s"} {
					if !strings.Contains(out, want) {
						t.Errorf("Summary missing %q:\n%s", want, out)
					}
				}
			},
		},
		{
			name: "marks_dominant_stage",
			entries: map[string]*entry{
				"fast": {dur: 100 * time.Millisecond, order: 1},
				"slow": {dur: 5 * time.Second, order: 2},
			},
			counter: 2,
			label:   "cmd",
			check: func(t *testing.T, out string) {
				var slowLine, fastLine string
				for l := range strings.SplitSeq(out, "\n") {
					if strings.Contains(l, "slow") {
						slowLine = l
					}
					if strings.Contains(l, "fast") {
						fastLine = l
					}
				}
				if !strings.Contains(slowLine, "◀") {
					t.Errorf("dominant stage 'slow' missing ◀ marker, line: %q", slowLine)
				}
				if strings.Contains(fastLine, "◀") {
					t.Errorf("non-dominant stage 'fast' has ◀ marker, line: %q", fastLine)
				}
			},
		},
		{
			name:    "single_stage_has_no_marker",
			entries: map[string]*entry{"only": {dur: time.Second, order: 1}},
			counter: 1,
			label:   "cmd",
			check: func(t *testing.T, out string) {
				if strings.Contains(out, "◀") {
					t.Errorf("single-stage summary should not have ◀ marker:\n%s", out)
				}
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := &Recorder{
				entries: tt.entries,
				counter: tt.counter,
				nowFunc: time.Now,
			}
			if r.entries == nil {
				r.entries = make(map[string]*entry)
			}
			var buf bytes.Buffer
			r.Summary(&buf, tt.label)
			tt.check(t, buf.String())
		})
	}
}

func TestDisabled(t *testing.T) {
	p := Disabled()
	for range 1000 {
		p.Stage("x")()
		p.Observe("x", 1, 1024)
	}
	var buf bytes.Buffer
	p.Summary(&buf, "test")
	if buf.Len() != 0 {
		t.Errorf("Disabled.Summary wrote %d bytes, want 0", buf.Len())
	}
}

func TestIsEnabled(t *testing.T) {
	if IsEnabled(nil) {
		t.Error("nil should not be enabled")
	}
	if IsEnabled(Disabled()) {
		t.Error("Disabled() should not be enabled")
	}
	if !IsEnabled(NewRecorder()) {
		t.Error("NewRecorder() should be enabled")
	}
}

func TestRecorder_Observe(t *testing.T) {
	r := &Recorder{
		entries: make(map[string]*entry),
		nowFunc: time.Now,
	}

	r.Observe("file_sync", 10, 1024)
	r.Observe("file_sync", 5, 512)

	e := r.entries["file_sync"]
	if e.files != 15 {
		t.Errorf("files = %d, want 15", e.files)
	}
	if e.bytes != 1536 {
		t.Errorf("bytes = %d, want 1536", e.bytes)
	}

	r.Observe("other", 1, 100)
	if r.entries["other"].files != 1 {
		t.Error("new entry via Observe not created")
	}
}

func TestFmtDur(t *testing.T) {
	tests := []struct {
		d    time.Duration
		want string
	}{
		{999 * time.Millisecond, "999ms"},
		{500 * time.Millisecond, "500ms"},
		{1 * time.Millisecond, "1ms"},
		{0, "0ms"},
		{time.Second, "1.000s"},
		{1500 * time.Millisecond, "1.500s"},
		{12345 * time.Millisecond, "12.345s"},
	}
	for _, tt := range tests {
		if got := fmtDur(tt.d); got != tt.want {
			t.Errorf("fmtDur(%v) = %q, want %q", tt.d, got, tt.want)
		}
	}
}

func TestFmtCount(t *testing.T) {
	tests := []struct {
		n    int64
		want string
	}{
		{0, "-"},
		{1, "1"},
		{999, "999"},
		{1000, "1,000"},
		{38412, "38,412"},
		{1000000, "1,000,000"},
	}
	for _, tt := range tests {
		if got := fmtCount(tt.n); got != tt.want {
			t.Errorf("fmtCount(%d) = %q, want %q", tt.n, got, tt.want)
		}
	}
}

func TestFmtBytes(t *testing.T) {
	tests := []struct {
		n    int64
		want string
	}{
		{0, "-"},
		{512, "512 B"},
		{1024, "1.0 KB"},
		{1536, "1.5 KB"},
		{1024 * 1024, "1.0 MB"},
		{int64(1.5 * 1024 * 1024), "1.5 MB"},
		{1024 * 1024 * 1024, "1.0 GB"},
		{1024 * 1024 * 1024 * 1024, "1.0 TB"},
	}
	for _, tt := range tests {
		if got := fmtBytes(tt.n); got != tt.want {
			t.Errorf("fmtBytes(%d) = %q, want %q", tt.n, got, tt.want)
		}
	}
}

func TestRecorder_Summary_MetricsColumns(t *testing.T) {
	t.Run("columns hidden when no metrics", func(t *testing.T) {
		r := &Recorder{
			entries: map[string]*entry{
				"git.worktree_add": {dur: time.Second, order: 1},
			},
			counter: 1,
			nowFunc: time.Now,
		}
		var buf bytes.Buffer
		r.Summary(&buf, "cmd")
		out := buf.String()
		if strings.Contains(out, "files") || strings.Contains(out, "bytes") {
			t.Errorf("files/bytes columns should be hidden when no metrics:\n%s", out)
		}
	})

	t.Run("columns shown when metrics present", func(t *testing.T) {
		r := &Recorder{
			entries: map[string]*entry{
				"file_sync":        {dur: 8 * time.Second, files: 38412, bytes: 1200000000, order: 1},
				"git.worktree_add": {dur: 142 * time.Millisecond, order: 2},
			},
			counter: 2,
			nowFunc: time.Now,
		}
		var buf bytes.Buffer
		r.Summary(&buf, "tp new")
		out := buf.String()
		for _, want := range []string{"files", "bytes", "38,412", "1.1 GB", "-"} {
			if !strings.Contains(out, want) {
				t.Errorf("Summary missing %q:\n%s", want, out)
			}
		}
		if !strings.Contains(out, "142ms") {
			t.Errorf("sub-second duration should render as ms:\n%s", out)
		}
		if !strings.Contains(out, "8.000s") {
			t.Errorf("second-range duration should render as seconds:\n%s", out)
		}
	})
}

// stepClock returns a nowFunc that advances by step on each call, so that
// one Stage() invocation (two calls: start + done) records exactly step.
func stepClock(step time.Duration) func() time.Time {
	base := time.Unix(1_000_000, 0)
	var mu sync.Mutex
	cur := base
	return func() time.Time {
		mu.Lock()
		defer mu.Unlock()
		ret := cur
		cur = cur.Add(step)
		return ret
	}
}
