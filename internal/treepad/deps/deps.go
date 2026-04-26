package deps

import (
	"io"
	"os"

	"golang.org/x/term"

	"treepad/internal/artifact"
	"treepad/internal/hook"
	"treepad/internal/passthrough"
	"treepad/internal/profile"
	internalsync "treepad/internal/sync"
	"treepad/internal/ui"
	"treepad/internal/worktree"
)

// Deps bundles the dependencies every treepad operation needs.
// Tests construct Deps directly with fakes; production callers use DefaultDeps.
type Deps struct {
	Runner     worktree.CommandRunner
	Syncer     internalsync.Syncer
	Opener     artifact.Opener
	HookRunner hook.Runner
	PTRunner   passthrough.Runner
	Profiler   profile.Profiler
	Out        io.Writer   // stdout: machine payloads (__TREEPAD_CD__, JSON, tables)
	Log        *ui.Printer // stderr: tagged user-facing narrative
	In         io.Reader

	// IsTerminal reports whether w is an interactive terminal.
	IsTerminal func(w io.Writer) bool
	// CDSentinel, when non-nil, returns the writer emitCD uses for the
	// __TREEPAD_CD__ payload instead of the fd-3 probe. Tests set this to a
	// bytes.Buffer to avoid touching real file descriptors.
	CDSentinel func() io.Writer
}

// DefaultDeps wires production implementations. It is the single composition
// root for callers that do not need custom dependencies.
func DefaultDeps(out, errw io.Writer, in io.Reader) Deps {
	runner := worktree.ExecRunner{}
	return Deps{
		Runner:     runner,
		Syncer:     internalsync.FileSyncer{},
		Opener:     artifact.ExecOpener{Runner: runner},
		HookRunner: hook.ExecRunner{Runner: runner},
		PTRunner:   passthrough.OSRunner{},
		Profiler:   profile.Disabled(),
		Out:        out,
		Log:        ui.New(errw),
		In:         in,
		IsTerminal: func(w io.Writer) bool {
			f, ok := w.(*os.File)
			if !ok {
				return false
			}
			return term.IsTerminal(int(f.Fd()))
		},
	}
}
