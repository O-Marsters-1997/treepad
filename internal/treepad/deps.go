package treepad

import (
	"context"
	"io"
	"os"
	"time"

	"golang.org/x/term"

	"treepad/internal/artifact"
	"treepad/internal/hook"
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
	PTRunner   PassthroughRunner
	Out        io.Writer   // stdout: machine payloads (__TREEPAD_CD__, JSON, tables)
	Log        *ui.Printer // stderr: tagged user-facing narrative
	In         io.Reader

	// IsTerminal reports whether w is an interactive terminal.
	IsTerminal func(w io.Writer) bool
	// Sleep returns a channel that receives after d elapses (injectable for tests).
	Sleep func(d time.Duration) <-chan time.Time
	// CDSentinel, when non-nil, returns the writer emitCD uses for the
	// __TREEPAD_CD__ payload instead of the fd-3 probe. Tests set this to a
	// bytes.Buffer to avoid touching real file descriptors.
	CDSentinel func() io.Writer
	// NewRepoView constructs a RepoView for the given output directory.
	// Tests substitute a fakeRepoView factory to avoid the git wire protocol.
	NewRepoView func(ctx context.Context, outputDir string) (RepoView, error)
}

// DefaultDeps wires production implementations. It is the single composition
// root for callers that do not need custom dependencies.
func DefaultDeps(out, errw io.Writer, in io.Reader) Deps {
	runner := worktree.ExecRunner{}
	d := Deps{
		Runner:     runner,
		Syncer:     internalsync.FileSyncer{},
		Opener:     artifact.ExecOpener{Runner: runner},
		HookRunner: hook.ExecRunner{Runner: runner},
		PTRunner:   osPassthroughRunner{},
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
		Sleep: time.After,
	}
	d.NewRepoView = func(ctx context.Context, outputDir string) (RepoView, error) {
		return OpenRepo(ctx, d, outputDir)
	}
	return d
}
