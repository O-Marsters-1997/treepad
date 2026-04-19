package register

import (
	"treepad/e2e/script"
	"treepad/internal/commands"
)

func init() {
	commands.RegisterScriptedUI(script.Run)
}
