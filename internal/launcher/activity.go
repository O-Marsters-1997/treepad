package launcher

import (
	"os"
	"time"
)

// RunState is derived from the Activity file's mtime, never a PID: a tmux
// Launcher's child exits within milliseconds while the agent runs for an
// hour, and a Conductor-launched agent was never treepad's child.
type RunState string

const (
	StatePending RunState = "pending"
	StateWorking RunState = "working"
	StateIdle    RunState = "idle"
)

// workingWindow is how recently the Activity file must have been touched to
// count as "working". No `stuck` state exists: a label that's wrong a third
// of the time is worse than no label.
const workingWindow = 90 * time.Second

// Exists reports whether branch has an Activity file. This is the
// double-launch guard: once launched, treepad never relaunches a member
// automatically.
func Exists(commonDir, branch string) bool {
	_, err := os.Stat(ActivityPath(commonDir, branch))
	return err == nil
}

// State derives branch's run state from its Activity file's mtime as of now.
// A missing file is StatePending.
func State(commonDir, branch string, now time.Time) (RunState, error) {
	info, err := os.Stat(ActivityPath(commonDir, branch))
	if os.IsNotExist(err) {
		return StatePending, nil
	}
	if err != nil {
		return "", err
	}
	if now.Sub(info.ModTime()) <= workingWindow {
		return StateWorking, nil
	}
	return StateIdle, nil
}
