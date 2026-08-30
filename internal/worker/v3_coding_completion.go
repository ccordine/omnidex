package worker

import (
	"fmt"
	"sort"
	"strings"
)

type directCodingCompletionState struct {
	AllowExistingWorkspace bool
	MutationCount          int
	LatestMutationTurn     int
	LatestCheckTurn        int
	LatestTestTurn         int
	TestsRequired          bool
	LastTestHadNoTests     bool
}

func validateDirectCodingCompletion(state directCodingCompletionState) error {
	violations := make([]string, 0, 4)
	if !state.AllowExistingWorkspace && state.MutationCount < 1 {
		violations = append(violations, "no source mutation was accepted in this coding session")
	}
	if state.LatestCheckTurn < state.LatestMutationTurn || state.LatestCheckTurn == 0 {
		violations = append(violations, "no successful verification command ran after the latest workspace mutation")
	}
	if state.TestsRequired && (state.LatestTestTurn < state.LatestMutationTurn || state.LatestTestTurn == 0) {
		violations = append(violations, "no successful test command ran after the latest workspace mutation")
	} else if state.TestsRequired && state.LastTestHadNoTests {
		violations = append(violations, "the successful test command reported no tests")
	}
	if len(violations) == 0 {
		return nil
	}
	sort.Strings(violations)
	return fmt.Errorf("coding completion is not ready: %s", strings.Join(violations, "; "))
}

func isDirectCodingSourcePath(path string) bool {
	lower := strings.ToLower(path)
	for _, extension := range []string{".go", ".rs", ".js", ".mjs", ".cjs", ".ts", ".tsx", ".php", ".py", ".java", ".cs", ".rb"} {
		if strings.HasSuffix(lower, extension) {
			return true
		}
	}
	return false
}

func isDirectCodingVerificationCommand(program string, args []string) bool {
	if len(args) == 0 || isV3WorkspaceInitializer(program, args) {
		return false
	}
	program = strings.ToLower(strings.TrimSpace(program))
	verb := strings.ToLower(strings.TrimSpace(args[0]))
	switch program {
	case "go":
		return verb == "build" || verb == "test" || verb == "vet"
	case "cargo":
		return verb == "build" || verb == "check" || verb == "clippy" || verb == "fmt" || verb == "test"
	case "npm", "pnpm", "yarn":
		if verb == "test" {
			return true
		}
		if verb == "run" {
			return len(args) >= 2 && v3VerificationScriptName(args[1])
		}
		return program != "npm" && v3VerificationScriptName(verb)
	case "pytest", "phpunit":
		return true
	case "python3":
		return len(args) >= 2 && verb == "-m" && strings.EqualFold(args[1], "pytest")
	case "dotnet", "mvn", "gradle":
		return verb == "build" || verb == "check" || verb == "test" || verb == "verify"
	default:
		return false
	}
}
