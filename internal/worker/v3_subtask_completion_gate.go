package worker

import (
	"fmt"
	"sort"
	"strings"

	"github.com/gryph/omnidex/internal/artifacts"
)

const subtaskCompletionGateTool = "objective.gate"

func validateSubtaskCompletionEvidence(objective artifacts.Objective, records []subtaskToolRecord, workspaceState string) error {
	violations := make([]string, 0, 5)
	latestMutation := -1
	latestVerification := -1
	latestTest := -1
	latestTestOutput := ""
	for index, record := range records {
		if record.Result.Accepted && record.Result.Tool == "workspace.write" {
			latestMutation = index
		}
		if !record.Result.Accepted || record.Result.Tool != "command.run" || !toolResultSucceeded(record.Result) {
			continue
		}
		program, _ := record.Call.Input["program"].(string)
		args, _ := strictV3StringArray(record.Call.Input["args"], "args")
		if isV3WorkspaceInitializer(program, args) {
			continue
		}
		latestVerification = index
		if isV3TestCommand(program, args) {
			latestTest = index
			latestTestOutput = strings.TrimSpace(toolResultText(record.Result.Output, "stdout") + "\n" + toolResultText(record.Result.Output, "stderr"))
		}
	}
	if containsString(objective.RequiredCapabilities, capabilityWorkspaceWrite) && latestMutation < 0 {
		violations = append(violations, "no successful workspace write exists")
	}
	criteria := strings.ToLower(strings.Join(objective.AcceptanceCriteria, " "))
	requiresTests := strings.Contains(criteria, "test")
	if containsString(objective.RequiredCapabilities, capabilityCommandExecute) {
		if latestVerification < 0 {
			violations = append(violations, "no successful verification command exists")
		} else if latestVerification < latestMutation {
			violations = append(violations, "verification must run after the latest workspace write")
		}
	}
	if requiresTests {
		switch {
		case latestTest < 0:
			violations = append(violations, "no successful test command exists")
		case latestTest < latestMutation:
			violations = append(violations, "tests must run after the latest workspace write")
		case verificationReportsNoTests(latestTestOutput):
			violations = append(violations, "the successful command reported no tests")
		}
	}
	if strings.Contains(criteria, "readme") && !strings.Contains(strings.ToLower(workspaceState), "readme") {
		violations = append(violations, "the current workspace has no README file")
	}
	if len(violations) == 0 {
		return nil
	}
	sort.Strings(violations)
	return fmt.Errorf("objective completion evidence is incomplete: %s", strings.Join(violations, "; "))
}

func toolResultSucceeded(result artifacts.ToolResultArtifact) bool {
	succeeded, ok := result.Output["succeeded"].(bool)
	return ok && succeeded
}

func isV3WorkspaceInitializer(program string, args []string) bool {
	if len(args) == 0 {
		return false
	}
	program = strings.ToLower(strings.TrimSpace(program))
	verb := strings.ToLower(strings.TrimSpace(args[0]))
	return program == "go" && verb == "mod" || program == "cargo" && verb == "init" || program == "npm" && verb == "init"
}

func verificationReportsNoTests(output string) bool {
	lower := strings.ToLower(strings.TrimSpace(output))
	for _, marker := range []string{"[no test files]", "running 0 tests", "0 tests", "no tests found", "no test files found"} {
		if strings.Contains(lower, marker) {
			return true
		}
	}
	return false
}
