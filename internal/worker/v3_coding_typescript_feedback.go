package worker

import (
	"fmt"
	"regexp"
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
	message = directCodingTypeScriptPrimaryTestFailure(name, message)
	message = directCodingANSISequencePattern.ReplaceAllString(message, "")
	observation, err := directCodingTestingLibraryRoleObservationProjection(
		name, message, failure.AccessibilityObservation,
	)
	if err != nil {
		return "", fmt.Errorf("project Testing Library role observation: %w", err)
	}
	clean := directCodingANSISequencePattern.ReplaceAllString(name+": "+message, "")
	clean, err = canonicalizeDirectCodingTypeScriptDiagnosticRegularExpressions(
		clean, authorizedRegexLiterals,
	)
	if err != nil {
		return "", fmt.Errorf("canonicalize structured Vitest regular expressions: %w", err)
	}
	if observation != "" {
		clean += " " + observation
	}
	clean = strings.ReplaceAll(strings.ReplaceAll(clean, "\r", " "), "\n", " ")
	fields := strings.Fields(clean)
	for index, field := range fields {
		if modelcontext.ContainsPathIdentityWithProvenance(field, provenance) {
			fields[index] = "[source]"
		}
	}
	clean = trimForBudget(
		strings.Join(fields, " "), maxDirectCodingStructuredTestDiagnosticBytes,
	)
	if clean == "" {
		return "", fmt.Errorf("structured Vitest failure became empty after path redaction")
	}
	if modelcontext.ContainsPathIdentityWithProvenance(clean, provenance) {
		return "", fmt.Errorf("structured Vitest failure retained path identity after redaction")
	}
	return clean, nil
}

// Testing Library appends provider prose and serialized DOM after the primary
// error paragraph. Code captures the requested role's computed accessible
// names separately as typed data before reducing that prose. Other error types
// and messages without the provider paragraph boundary remain unchanged.
func directCodingTypeScriptPrimaryTestFailure(name string, message string) string {
	if name != "TestingLibraryElementError" {
		return message
	}
	normalized := strings.ReplaceAll(message, "\r\n", "\n")
	if boundary := strings.Index(normalized, "\n\n"); boundary >= 0 {
		primary := strings.TrimSpace(normalized[:boundary])
		if primary != "" {
			return primary
		}
	}
	return message
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
	if directCodingTypeScriptCompilerContainsPathIdentity(feedback) {
		return "", fmt.Errorf("TypeScript stage diagnostic for block %s contains path identity", diagnostic.BlockID)
	}
	return feedback, nil
}
