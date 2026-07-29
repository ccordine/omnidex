package worker

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
)

var unfinishedCodePattern = regexp.MustCompile(`(?im)(\bTODO\b|\bFIXME\b|IMPLEMENT[ _-]?ME|not implemented|unimplemented!\s*\(|panic\s*\(\s*["']not implemented|\bplaceholder\b|here we would typically|would typically|would need to be refactored|here we would|left as an exercise|add assertions(?:\s+to\s+verify)?)`)

type directCodingCompletionState struct {
	AllowExistingWorkspace bool
	MutationCount          int
	LatestMutationTurn     int
	LatestCheckTurn        int
	LatestTestTurn         int
	TestsRequired          bool
	LastTestHadNoTests     bool
	WrittenSource          map[string]string
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
	for path, content := range state.WrittenSource {
		if isDirectCodingSourcePath(path) && unfinishedCodePattern.MatchString(content) {
			violations = append(violations, fmt.Sprintf("written source %s still contains an unfinished implementation marker", path))
		}
	}
	if len(violations) == 0 {
		return nil
	}
	sort.Strings(violations)
	return fmt.Errorf("coding completion is not ready: %s", strings.Join(violations, "; "))
}

func directCodingUnfinishedDiagnostic(state directCodingCompletionState) *directCodingDiagnostic {
	paths := make([]string, 0, len(state.WrittenSource))
	for path := range state.WrittenSource {
		paths = append(paths, path)
	}
	sort.Strings(paths)
	for _, path := range paths {
		content := state.WrittenSource[path]
		if !isDirectCodingSourcePath(path) {
			continue
		}
		marker := strings.TrimSpace(unfinishedCodePattern.FindString(content))
		if marker == "" {
			continue
		}
		return directCodingStaticFileDiagnostic(
			path,
			fmt.Sprintf(
				"%s contains forbidden unfinished implementation marker %q. Replace it with the real implementation required by the assigned requirements without weakening existing behavior.",
				path,
				marker,
			),
		)
	}
	return nil
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
