// Package worktreetest provides shared test helpers for packages that depend
// on worktree.CommandRunner.
package worktreetest

import "context"

type StaticRunner struct {
	Output []byte
	Err    error
}

func (r StaticRunner) Run(_ context.Context, _ string, _ ...string) ([]byte, error) {
	return r.Output, r.Err
}
