package treepad

import (
	"io"

	"treepad/internal/artifact"
	internalsync "treepad/internal/sync"
	"treepad/internal/worktree"
)

// Deps bundles the dependencies every treepad operation needs.
// Tests construct Deps directly with fakes; production callers use DefaultDeps.
type Deps struct {
	Runner worktree.CommandRunner
	Syncer internalsync.Syncer
	Opener artifact.Opener
	Out    io.Writer
	In     io.Reader
}

// DefaultDeps wires production implementations. It is the single composition
// root for callers that do not need custom dependencies.
func DefaultDeps(out io.Writer, in io.Reader) Deps {
	runner := worktree.ExecRunner{}
	return Deps{
		Runner: runner,
		Syncer: internalsync.FileSyncer{},
		Opener: artifact.ExecOpener{Runner: runner},
		Out:    out,
		In:     in,
	}
}
