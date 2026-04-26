package cd

import (
	"context"

	"treepad/internal/treepad/cdshell"
	"treepad/internal/treepad/deps"
)

type CDInput struct {
	Branch string
}

func CD(ctx context.Context, d deps.Deps, in CDInput) error {
	return cdshell.CD(ctx, cdshellDeps(d), cdshell.CDInput{Branch: in.Branch})
}
