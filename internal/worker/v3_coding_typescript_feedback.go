package worker

import (
	"regexp"
	"strings"
)

const maxDirectCodingModelFailureLines = 4

var (
	directCodingANSISequencePattern       = regexp.MustCompile(`\x1b\[[0-9;?]*[ -/]*[@-~]`)
	directCodingTypeScriptIdentityPattern = regexp.MustCompile(
		`(?:[A-Za-z]:)?(?:\.{0,2}/|/)?(?:[A-Za-z0-9_.-]+/)*[A-Za-z0-9_.-]+\.tsx?(?:(?::[0-9]+){1,2}|\([0-9]+,[0-9]+\))?`,
	)
	directCodingTypeScriptSourceFramePattern = regexp.MustCompile(`^[0-9]+\s*\|`)
)

func directCodingTypeScriptModelFailure(raw string) string {
	clean := directCodingANSISequencePattern.ReplaceAllString(strings.ReplaceAll(raw, "\r", ""), "")
	clean = directCodingTypeScriptIdentityPattern.ReplaceAllString(clean, "[source]")
	selected := make([]string, 0, maxDirectCodingModelFailureLines)
	seen := make(map[string]struct{})
	focusedFailure := false
	for _, rawLine := range strings.Split(clean, "\n") {
		line := strings.TrimSpace(rawLine)
		if line == "" || strings.HasPrefix(line, "× ") || directCodingTypeScriptSourceFramePattern.MatchString(line) {
			continue
		}
		if marker := strings.Index(line, "[source] > "); marker >= 0 {
			if focusedFailure {
				break
			}
			// A concrete test header is more precise than any suite-level text that
			// preceded it. Start a fresh envelope and stop at the next test.
			focusedFailure = true
			selected = selected[:0]
			seen = make(map[string]struct{})
			line = "FAILED_CHECK: " + strings.TrimSpace(line[marker+len("[source] > "):])
		} else if !strings.HasPrefix(line, "CORRECTION_REJECTION:") &&
			(directCodingTypeScriptFailureNoise(line) || !directCodingTypeScriptFailureSignal(line)) {
			continue
		}
		if _, duplicate := seen[line]; duplicate {
			continue
		}
		seen[line] = struct{}{}
		selected = append(selected, line)
		if len(selected) == maxDirectCodingModelFailureLines {
			break
		}
	}
	if len(selected) == 0 {
		return "Validation failed without a concise function-owned diagnostic."
	}
	return trimForBudget(strings.Join(selected, "\n"), 360)
}

func directCodingTypeScriptFragmentFailure(original string, rejection error) string {
	parts := make([]string, 0, 2)
	if original = strings.TrimSpace(original); original != "" {
		parts = append(parts, trimForBudget(original, 700))
	}
	if rejection != nil {
		parts = append(parts, "CORRECTION_REJECTION: "+trimForBudget(rejection.Error(), 250))
	}
	return strings.Join(parts, "\n")
}

func directCodingTypeScriptFailureNoise(line string) bool {
	lower := strings.ToLower(line)
	return strings.Contains(lower, "node_modules") ||
		lower == "- expected:" || lower == "+ received:" ||
		(strings.HasPrefix(line, "❯") && !directCodingTypeScriptFailureSignal(line)) ||
		strings.HasPrefix(line, "> ") ||
		strings.HasPrefix(lower, "test files") ||
		strings.HasPrefix(lower, "tests ") ||
		strings.HasPrefix(lower, "start at") ||
		strings.HasPrefix(lower, "duration") ||
		strings.Contains(lower, "npm error")
}

func directCodingTypeScriptFailureSignal(line string) bool {
	lower := strings.ToLower(line)
	for _, signal := range []string{
		"error ts", "assertionerror", "typeerror", "referenceerror", "rangeerror",
		"testinglibraryelementerror", "unable to find", "found multiple", "unable to fire",
		"expected", "received", "toequal", "tobe", "tohave",
	} {
		if strings.Contains(lower, signal) {
			return true
		}
	}
	return false
}
