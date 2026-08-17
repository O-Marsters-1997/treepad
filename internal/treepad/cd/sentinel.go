package cd

import (
	"github.com/O-Marsters-1997/treepad/internal/treepad/cdshell"
	"github.com/O-Marsters-1997/treepad/internal/treepad/deps"
)

// cdshellDeps maps treepad.Deps to cdshell.Deps.
func cdshellDeps(d deps.Deps) cdshell.Deps {
	return cdshell.Deps{
		Out:        d.Out,
		Log:        d.Log,
		IsTerminal: d.IsTerminal,
		CDSentinel: d.CDSentinel,
		Runner:     d.Runner,
	}
}

func EmitCD(d deps.Deps, path string) {
	cdshell.EmitCD(cdshellDeps(d), path)
}

func MaybeWarnStaleWrapper(d deps.Deps, hasAgentCommand bool) {
	cdshell.MaybeWarnStaleWrapper(cdshellDeps(d), hasAgentCommand)
}
