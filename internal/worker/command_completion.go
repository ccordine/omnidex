package worker

import (
	"regexp"
	"strings"
)

var (
	verificationPositiveTestsPattern = regexp.MustCompile(`(?im)(^|[^0-9])[1-9][0-9]* tests?\b|^\s*#\s*tests\s+[1-9][0-9]*\b|\btests?\s+[1-9][0-9]*\s+passed\b|^ok\s+\S+`)
	verificationZeroTestsPattern     = regexp.MustCompile(`(?im)(^|[^0-9])0 tests?\b`)
)

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
	// Aggregating runners may report a zero-test package or binary alongside
	// another suite that executed tests. Any positive execution receipt wins;
	// a local zero line cannot erase it.
	if verificationPositiveTestsPattern.MatchString(lower) {
		return false
	}
	for _, marker := range []string{"[no test files]", "no tests found", "no test files found"} {
		if strings.Contains(lower, marker) {
			return true
		}
	}
	return verificationZeroTestsPattern.MatchString(lower)
}
