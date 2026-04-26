package treepad

import (
	"fmt"
	"io"
	"os"
	"strconv"
)

func emitCD(d Deps, path string) {
	if w := cdSentinelWriter(d); w != nil {
		_, _ = io.WriteString(w, path+"\n")
		return
	}
	_, _ = fmt.Fprintf(d.Out, "__TREEPAD_CD__\t%s\n", path)
}

// cdSentinelWriter returns a writer for the __TREEPAD_CD__ sentinel.
// The new tp shell wrapper sets TREEPAD_CD_FD=3 and redirects fd 3 into
// its $(...) capture, letting tp's stdout go to the real terminal. When the
// env var is absent (stale wrapper, direct binary invocation, tests) it
// returns nil so emitCD falls back to d.Out.
func cdSentinelWriter(d Deps) io.Writer {
	if d.CDSentinel != nil {
		return d.CDSentinel()
	}
	fdStr := os.Getenv("TREEPAD_CD_FD")
	if fdStr == "" {
		return nil
	}
	fd, err := strconv.Atoi(fdStr)
	if err != nil || fd < 0 {
		return nil
	}
	return os.NewFile(uintptr(fd), "treepad-cd")
}

// maybeWarnStaleWrapper prints a one-line stderr hint when an agent_command is
// configured but the new shell wrapper (which sets TREEPAD_CD_FD) has not been
// installed. Fires only when TREEPAD_CD_FD is unset AND stdout is not a TTY
// (i.e. inside the old wrapper's $(...) capture). No-op in all other contexts
// (new wrapper active, CI, direct binary invocation).
func maybeWarnStaleWrapper(d Deps, hasAgentCommand bool) {
	if !hasAgentCommand {
		return
	}
	if os.Getenv("TREEPAD_CD_FD") != "" {
		return
	}
	if d.IsTerminal(d.Out) {
		return
	}
	d.Log.Warn("stale shell wrapper detected — re-run: eval \"$(tp shell-init)\"")
	d.Log.Warn("Your agent will still start interactively via /dev/tty.")
}
