package worker

import "strings"

func isV3WorkspaceInitializer(program string, args []string) bool {
	if len(args) == 0 {
		return false
	}
	program = strings.ToLower(strings.TrimSpace(program))
	verb := strings.ToLower(strings.TrimSpace(args[0]))
	return program == "go" && verb == "mod" ||
		program == "cargo" && verb == "init" ||
		program == "npm" && verb == "init"
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
