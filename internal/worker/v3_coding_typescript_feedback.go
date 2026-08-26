package worker

import (
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strings"

	"github.com/gryph/omnidex/internal/assemblyline"
	"github.com/gryph/omnidex/internal/modelcontext"
)

const maxDirectCodingStructuredTestDiagnosticBytes = 900

var (
	directCodingANSISequencePattern = regexp.MustCompile(`\x1b\[[0-9;?]*[ -/]*[@-~]`)
)

func directCodingTypeScriptStructuredTestModelFailure(
	failure directCodingVitestFailureEvidence,
	provenance assemblyline.ArtifactIdentityProvenance,
	authorizedRegexLiterals ...string,
) (string, error) {
	name := strings.TrimSpace(failure.Name)
	message := strings.TrimSpace(failure.Message)
	if name == "" || message == "" {
		return "", fmt.Errorf("structured Vitest failure requires one exact error name and message")
	}
	clean := directCodingANSISequencePattern.ReplaceAllString(name+": "+message, "")
	clean = strings.ReplaceAll(strings.ReplaceAll(clean, "\r", " "), "\n", " ")
	fields := strings.Fields(clean)
	for index, field := range fields {
		pathCheck := maskDirectCodingAuthorizedRegularExpressions(
			field, authorizedRegexLiterals,
		)
		if modelcontext.ContainsPathIdentityWithProvenance(pathCheck, provenance) {
			fields[index] = "[source]"
		}
	}
	clean = trimForBudget(
		strings.Join(fields, " "), maxDirectCodingStructuredTestDiagnosticBytes,
	)
	if clean == "" {
		return "", fmt.Errorf("structured Vitest failure became empty after path redaction")
	}
	pathCheck := maskDirectCodingAuthorizedRegularExpressions(
		clean, authorizedRegexLiterals,
	)
	if modelcontext.ContainsPathIdentityWithProvenance(pathCheck, provenance) {
		return "", fmt.Errorf("structured Vitest failure retained path identity after redaction")
	}
	return clean, nil
}

func redactDirectCodingPathIdentities(
	value string,
	provenance assemblyline.ArtifactIdentityProvenance,
) string {
	identities := modelcontext.PathIdentities(value, provenance)
	if len(identities) == 0 {
		return value
	}
	var redacted strings.Builder
	previous := 0
	for _, identity := range identities {
		redacted.WriteString(value[previous:identity.Start])
		redacted.WriteString("[source]")
		previous = identity.End
	}
	redacted.WriteString(value[previous:])
	return redacted.String()
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
