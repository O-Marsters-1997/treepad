package worktree

import (
	"bytes"
	"context"
	"fmt"
	"strings"
)

// RemoteBranchExists reports whether the branch at path has a configured
// upstream tracking ref and whether that remote still has the branch.
//
// Returns (false, false, nil) when no upstream is configured — a local branch
// that was never pushed is not a "remote-gone" finding.
// Returns (false, true, nil) when an upstream is configured but the remote no
// longer has the branch.
func RemoteBranchExists(
	ctx context.Context, runner CommandRunner,
	path, branch string,
) (exists, hasUpstream bool, err error) {
	out, err := runner.Run(ctx, "git", "-C", path, "rev-parse", "--abbrev-ref", "@{upstream}")
	if err != nil {
		return false, false, nil
	}

	remote := "origin"
	if upstream := strings.TrimSpace(string(out)); strings.Contains(upstream, "/") {
		remote = upstream[:strings.IndexByte(upstream, '/')]
	}

	lsOut, err := runner.Run(ctx, "git", "-C", path, "ls-remote", remote, "refs/heads/"+branch)
	if err != nil {
		return false, true, fmt.Errorf("git ls-remote: %w", err)
	}
	return len(bytes.TrimSpace(lsOut)) > 0, true, nil
}
