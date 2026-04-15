package commands

import (
	"context"
	"io"

	"github.com/urfave/cli/v3"
)

// shellFunc is the shell wrapper function users source via `eval "$(tp shell-init)"`.
// It captures all output from the real tp binary, extracts the __TREEPAD_CD__ directive
// emitted by `tp new`, cd's into that path, then prints the remaining output.
const shellFunc = `tp() {
  local out rc cd_path
  out=$(command tp "$@")
  rc=$?
  cd_path=$(printf '%s\n' "$out" | awk -F'\t' '/^__TREEPAD_CD__\t/ {print $2; exit}')
  printf '%s\n' "$out" | grep -v '^__TREEPAD_CD__	' || true
  [ -n "$cd_path" ] && cd "$cd_path"
  return $rc
}`

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
