package commands

import (
	"context"
	"io"

	"github.com/urfave/cli/v3"
)

// shellFunc is the shell wrapper function users source via `eval "$(tp shell-init)"`.
// Stdout (fd 1) passes straight to the terminal via fd 4, so tp's output streams
// in real time and interactive children (e.g. claude) inherit a real TTY.
// TREEPAD_CD_FD=3 tells tp to write the __TREEPAD_CD__ directive to fd 3, which
// is redirected into the $(...) capture so only that payload is captured.
// TP_PREV_WORKTREE tracks the last directory before a tp-initiated cd, enabling
// `tp cd -` to toggle between two worktrees.
const shellFunc = `tp() {
  local cd_path rc
  if [ "$1" = "cd" ] && [ "$2" = "-" ]; then
    [ -z "$TP_PREV_WORKTREE" ] && { printf 'tp: no previous worktree\n' >&2; return 1; }
    local _prev="$TP_PREV_WORKTREE"
    TP_PREV_WORKTREE="$PWD"
    cd -- "$_prev"
    return $?
  fi
  cd_path="$(TREEPAD_CD_FD=3 command tp "$@" 3>&1 1>&4)"; rc=$?
  if [ -n "$cd_path" ]; then
    TP_PREV_WORKTREE="$PWD"
    cd -- "$cd_path"
  fi
  return $rc
} 4>&1`

func shellInitCommand() *cli.Command {
	return &cli.Command{
		Name:  "shell-init",
		Usage: "print shell integration function; add `eval \"$(tp shell-init)\"` to your shell rc file",
		Action: func(_ context.Context, cmd *cli.Command) error {
			_, err := io.WriteString(cmd.Root().Writer, shellFunc+"\n")
			return err
		},
	}
}
