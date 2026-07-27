package worker

import (
	"strings"

	toolruntime "github.com/gryph/omnidex/internal/tools"
)

func registerV3ExecutionTools(registry *toolruntime.Registry) {
	mustRegisterTool(registry, workspaceWriteToolSpec(), executeV3WorkspaceWrite)
	mustRegisterTool(registry, commandRunToolSpec(), executeV3Command)
}

func v3ToolRequiresWorkspaceScope(name string) bool {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "workspace.research", "workspace.write", "command.run":
		return true
	default:
		return false
	}
}
