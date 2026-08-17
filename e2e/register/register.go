package register

import (
	"github.com/O-Marsters-1997/treepad/e2e/script"
	"github.com/O-Marsters-1997/treepad/internal/commands"
)

func init() {
	commands.RegisterScriptedUI(script.Run)
}
