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
	}
	var buf bytes.Buffer
	p.Summary(&buf, "test")
	if buf.Len() != 0 {
		t.Errorf("Disabled.Summary wrote %d bytes, want 0", buf.Len())
	}
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
