package worker

import (
	"fmt"
	"sort"
	"strings"

	"github.com/gryph/omnidex/internal/artifacts"
)

const (
	capabilityToolInspect     = "tool.inspect"
	capabilityWorkspaceRead   = "workspace.read"
	capabilityWorkspaceWrite  = "workspace.write"
	capabilityMemoryRead      = "memory.read"
	capabilityWebSearch       = "web.search"
	capabilityEvidenceRead    = "evidence.read"
	capabilityCommandExecute  = "command.execute"
	capabilityExternalExecute = "external.execute"
)

type v3CapabilityDefinition struct {
	ID        string
	Tool      string
	Execution bool
}

var v3CapabilityCatalog = []v3CapabilityDefinition{
	{ID: capabilityToolInspect, Tool: "tool.registry"},
	{ID: capabilityWorkspaceRead, Tool: "workspace.research"},
	{ID: capabilityWorkspaceWrite, Tool: "workspace.write", Execution: true},
	{ID: capabilityMemoryRead, Tool: "memory.retrieve"},
	{ID: capabilityWebSearch, Tool: "web.search"},
	{ID: capabilityEvidenceRead, Tool: "evidence.inspect"},
	{ID: capabilityCommandExecute, Tool: "command.run", Execution: true},
	{ID: capabilityExternalExecute, Execution: true},
}

func knownV3Capabilities() []string {
	out := make([]string, 0, len(v3CapabilityCatalog))
	for _, definition := range v3CapabilityCatalog {
		out = append(out, definition.ID)
	}
	sort.Strings(out)
	return out
}

func availableV3Capabilities(toolNames []string) []string {
	toolSet := stringSet(toolNames)
	out := make([]string, 0, len(v3CapabilityCatalog))
	for _, definition := range v3CapabilityCatalog {
		if definition.Tool == "" {
			continue
		}
		if _, ok := toolSet[definition.Tool]; ok {
			out = append(out, definition.ID)
		}
	}
	sort.Strings(out)
	return out
}

func toolsForV3Capabilities(capabilities []string) []string {
	wanted := stringSet(capabilities)
	out := make([]string, 0, len(wanted))
	for _, definition := range v3CapabilityCatalog {
		if definition.Tool == "" {
			continue
		}
		if _, ok := wanted[definition.ID]; ok {
			out = append(out, definition.Tool)
		}
	}
	sort.Strings(out)
	return out
}

func differenceStrings(all, removed []string) []string {
	removedSet := stringSet(removed)
	out := make([]string, 0, len(all))
	for _, value := range uniqueStrings(all) {
		if _, removed := removedSet[value]; !removed {
			out = append(out, value)
		}
	}
	return out
}

func containsString(values []string, target string) bool {
	_, ok := stringSet(values)[strings.ToLower(strings.TrimSpace(target))]
	return ok
}

func validateV3Intent(intent artifacts.IntentArtifact, knownCapabilities []string) error {
	if err := intent.Validate(); err != nil {
		return err
	}
	known := stringSet(knownCapabilities)
	violations := make([]string, 0, 6)
	allRequired := append([]string(nil), intent.RequiredCapabilities...)
	objectiveRequired := make([]string, 0, len(intent.RequiredCapabilities))
	for _, objective := range intent.Objectives {
		allRequired = append(allRequired, objective.RequiredCapabilities...)
		objectiveRequired = append(objectiveRequired, objective.RequiredCapabilities...)
	}
	for _, capability := range uniqueStrings(allRequired) {
		if _, ok := known[capability]; !ok {
			violations = append(violations, fmt.Sprintf("unknown capability %q", capability))
		}
	}
	if intent.RequiresAction && !containsExecutionCapability(allRequired) {
		violations = append(violations, "action intent requires an explicit execution capability")
	}
	if intent.RequiresAction && strings.TrimSpace(intent.Mode) != "execute" {
		violations = append(violations, "action intent mode must be execute")
	}
	if !intent.RequiresAction && strings.TrimSpace(intent.Mode) == "execute" {
		violations = append(violations, "execute mode requires an action objective")
	}
	if intent.MemoryMode == artifacts.MemoryModeExplicitRecall && !containsString(intent.RequiredCapabilities, capabilityMemoryRead) {
		violations = append(violations, "explicit memory recall requires memory.read")
	}
	if intent.MemoryMode == artifacts.MemoryModeOff && containsString(intent.RequiredCapabilities, capabilityMemoryRead) {
		violations = append(violations, "memory.read conflicts with memory_mode off")
	}
	if missing := differenceStrings(intent.RequiredCapabilities, objectiveRequired); len(missing) > 0 {
		violations = append(violations, "intent capabilities absent from every objective: "+strings.Join(missing, ", "))
	}
	if missing := differenceStrings(objectiveRequired, intent.RequiredCapabilities); len(missing) > 0 {
		violations = append(violations, "objective capabilities absent from intent: "+strings.Join(missing, ", "))
	}
	for _, objective := range intent.Objectives {
		if objective.RequiresAction && !containsExecutionCapability(objective.RequiredCapabilities) {
			violations = append(violations, fmt.Sprintf("objective %q requires an execution capability", objective.ID))
		}
		if containsString(objective.RequiredCapabilities, capabilityWorkspaceWrite) && !containsString(objective.RequiredCapabilities, capabilityCommandExecute) {
			violations = append(violations, fmt.Sprintf("objective %q requires command.execute to verify workspace.write", objective.ID))
		}
	}
	if len(violations) > 0 {
		sort.Strings(violations)
		return fmt.Errorf("v3 intent contract rejected: %s", strings.Join(violations, "; "))
	}
	return nil
}

func containsExecutionCapability(values []string) bool {
	set := stringSet(values)
	for _, definition := range v3CapabilityCatalog {
		if !definition.Execution {
			continue
		}
		if _, ok := set[definition.ID]; ok {
			return true
		}
	}
	return false
}

func missingV3Capabilities(required, available []string) []string {
	availableSet := stringSet(available)
	out := make([]string, 0, len(required))
	for _, capability := range uniqueStrings(required) {
		if _, ok := availableSet[capability]; !ok {
			out = append(out, capability)
		}
	}
	sort.Strings(out)
	return out
}

func stringSet(values []string) map[string]struct{} {
	out := make(map[string]struct{}, len(values))
	for _, value := range values {
		clean := strings.ToLower(strings.TrimSpace(value))
		if clean != "" {
			out[clean] = struct{}{}
		}
	}
	return out
}

func uniqueStrings(values []string) []string {
	set := stringSet(values)
	out := make([]string, 0, len(set))
	for value := range set {
		out = append(out, value)
	}
	sort.Strings(out)
	return out
}
