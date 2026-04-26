package profile

import (
	"bytes"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestRecorder_SingleStage(t *testing.T) {
	r := &Recorder{
		entries: make(map[string]*entry),
		nowFunc: stepClock(100 * time.Millisecond),
	}
	r.Stage("repo.load")()
	if got := r.entries["repo.load"].dur; got != 100*time.Millisecond {
		t.Errorf("dur = %v, want 100ms", got)
	}
}

func TestRecorder_AccumulatesSameName(t *testing.T) {
	r := &Recorder{
		entries: make(map[string]*entry),
		nowFunc: stepClock(50 * time.Millisecond),
	}
	r.Stage("file_sync")()
	r.Stage("file_sync")()
	if got := r.entries["file_sync"].dur; got != 100*time.Millisecond {
		t.Errorf("dur = %v, want 100ms (two 50ms stages accumulated)", got)
	}
}

func TestRecorder_Summary_Empty(t *testing.T) {
	r := NewRecorder()
	var buf bytes.Buffer
	r.Summary(&buf, "test")
	if buf.Len() != 0 {
		t.Errorf("expected empty output for empty recorder, got %q", buf.String())
	}
}

func TestRecorder_Summary_ContainsExpectedContent(t *testing.T) {
	r := &Recorder{
		entries: map[string]*entry{
			"file_sync": {dur: 2 * time.Second, order: 1},
		},
		counter: 1,
		nowFunc: time.Now,
	}
	var buf bytes.Buffer
	r.Summary(&buf, "new feat/foo")
	out := buf.String()
	for _, want := range []string{"profile:", "new feat/foo", "file_sync", "total", "2.000s"} {
		if !strings.Contains(out, want) {
			t.Errorf("Summary missing %q:\n%s", want, out)
		}
	}
}

func TestRecorder_Summary_MarksTopStage(t *testing.T) {
	r := &Recorder{
		entries: map[string]*entry{
			"fast": {dur: 100 * time.Millisecond, order: 1},
			"slow": {dur: 5 * time.Second, order: 2},
		},
		counter: 2,
		nowFunc: time.Now,
	}
	var buf bytes.Buffer
	r.Summary(&buf, "cmd")
	out := buf.String()
	var slowLine, fastLine string
	for _, l := range strings.Split(out, "\n") {
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
}

func TestRecorder_Summary_SingleStageNoMarker(t *testing.T) {
	r := &Recorder{
		entries: map[string]*entry{"only": {dur: time.Second, order: 1}},
		counter: 1,
		nowFunc: time.Now,
	}
	var buf bytes.Buffer
	r.Summary(&buf, "cmd")
	if strings.Contains(buf.String(), "◀") {
		t.Errorf("single-stage summary should not have ◀ marker:\n%s", buf.String())
	}
}

func TestDisabled_NoOp(t *testing.T) {
	p := Disabled()
	for i := 0; i < 1000; i++ {
		p.Stage("x")()
	}
	var buf bytes.Buffer
	p.Summary(&buf, "test")
	if buf.Len() != 0 {
		t.Errorf("Disabled.Summary wrote %d bytes, want 0", buf.Len())
	}
}

func TestRecorder_Concurrent(t *testing.T) {
	r := NewRecorder()
	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			r.Stage("x")()
		}()
	}
	wg.Wait()
	if r.entries["x"].dur == 0 {
		t.Error("expected non-zero accumulated duration after concurrent calls")
	}
}

// stepClock returns a nowFunc that advances by step on each pair of calls
// (start+done = one stage). Each call advances by step/2 so that one Stage()
// invocation spans exactly step.
func stepClock(step time.Duration) func() time.Time {
	base := time.Unix(1_000_000, 0)
	var mu sync.Mutex
	t := base
	return func() time.Time {
		mu.Lock()
		defer mu.Unlock()
		ret := t
		t = t.Add(step)
		return ret
	}
}
