package worker

import (
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strings"

	"github.com/gryph/omnidex/internal/modelcontext"
)

const maxDirectCodingTestFailureLines = 7

var (
	directCodingANSISequencePattern       = regexp.MustCompile(`\x1b\[[0-9;?]*[ -/]*[@-~]`)
	directCodingTypeScriptIdentityPattern = regexp.MustCompile(
		`(?:[A-Za-z]:)?(?:\.{0,2}/|/)?(?:[A-Za-z0-9_.-]+/)*[A-Za-z0-9_.-]+\.tsx?(?:(?::[0-9]+){1,2}|\([0-9]+,[0-9]+\))?`,
	)
	directCodingTypeScriptSourceFramePattern = regexp.MustCompile(`^[0-9]+\s*\|`)
)

func directCodingTypeScriptTestModelFailure(
	raw string,
	authorizedRegexLiterals ...string,
) string {
	clean := directCodingANSISequencePattern.ReplaceAllString(strings.ReplaceAll(raw, "\r", ""), "")
	clean = directCodingTypeScriptIdentityPattern.ReplaceAllString(clean, "[source]")
	selected := make([]string, 0, maxDirectCodingTestFailureLines)
	seen := make(map[string]struct{})
	focusedFailure := false
	captureDetail := false
	for _, rawLine := range strings.Split(clean, "\n") {
		line := strings.TrimSpace(rawLine)
		if line == "" || strings.HasPrefix(line, "× ") || directCodingTypeScriptSourceFramePattern.MatchString(line) {
			continue
		}
		pathCheckLine := maskDirectCodingAuthorizedRegularExpressions(
			line, authorizedRegexLiterals,
		)
		if modelcontext.ContainsPathIdentity(pathCheckLine) {
			captureDetail = false
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
			captureDetail = false
		} else if captureDetail {
			captureDetail = false
		} else if !strings.HasPrefix(line, "CORRECTION_REJECTION:") &&
			(directCodingTypeScriptFailureNoise(line) || !directCodingTypeScriptFailureSignal(line)) {
			continue
		}
		if directCodingTypeScriptFailureDetailHeading(line) {
			captureDetail = true
		}
		if _, duplicate := seen[line]; duplicate {
			continue
		}
		seen[line] = struct{}{}
		selected = append(selected, line)
		if len(selected) == maxDirectCodingTestFailureLines {
			break
		}
	}
	if len(selected) == 0 {
		return "Validation failed without a concise function-owned diagnostic."
	}
	return trimForBudget(strings.Join(selected, "\n"), 360)
}

func directCodingTypeScriptFailureDetailHeading(line string) bool {
	normalized := strings.TrimSpace(strings.TrimLeft(strings.TrimSpace(line), "+-"))
	lower := strings.ToLower(normalized)
	if !strings.HasSuffix(lower, ":") {
		return false
	}
	return strings.HasPrefix(lower, "expected") || strings.HasPrefix(lower, "received")
}

func directCodingTypeScriptStageModelFeedback(diagnostic *directCodingStageDiagnostic) (string, error) {
	if diagnostic == nil {
		return "", fmt.Errorf("TypeScript stage model feedback requires one diagnostic")
	}
	feedback := strings.TrimSpace(diagnostic.ModelFeedback)
	if feedback == "" {
		return "", fmt.Errorf(
			"TypeScript stage diagnostic for block %s lacks one exact path-free model failure",
			diagnostic.BlockID,
		)
	}
	pathCheckFeedback := maskDirectCodingAuthorizedRegularExpressions(
		feedback, diagnostic.AuthorizedRegexLiterals,
	)
	if directCodingTypeScriptCompilerContainsPathIdentity(pathCheckFeedback) {
		return "", fmt.Errorf("TypeScript stage diagnostic for block %s contains path identity", diagnostic.BlockID)
	}
	return feedback, nil
}

func maskDirectCodingAuthorizedRegularExpressions(
	value string,
	authorized []string,
) string {
	literals := append([]string(nil), authorized...)
	sort.SliceStable(literals, func(left, right int) bool {
		return len(literals[left]) > len(literals[right])
	})
	for _, literal := range literals {
		if literal = strings.TrimSpace(literal); literal != "" {
			value = strings.ReplaceAll(value, literal, "[regular_expression]")
			if encoded, err := json.Marshal(literal); err == nil && len(encoded) >= 2 {
				value = strings.ReplaceAll(
					value, string(encoded[1:len(encoded)-1]), "[regular_expression]",
				)
			}
		}
	}
	return value
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
		"unsupported_acceptance_observation",
	} {
		if strings.Contains(lower, signal) {
			return true
		}
	}
	return false
}
