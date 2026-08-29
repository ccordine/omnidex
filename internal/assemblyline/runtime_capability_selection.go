package assemblyline

import (
	"fmt"
	"regexp"
	"strings"
)

const (
	RuntimeCapabilitySelectionSchemaV1 = "omnidex.runtime-capability-selection.v1"
	RuntimeCapabilitySelectionNone     = "NONE"

	maxRuntimeCapabilityCandidates   = 5
	maxRuntimeCapabilityLocalContext = 12288
	maxRuntimeCapabilityNeedBytes    = 2000
	maxRuntimeCapabilityDialectBytes = 256
	maxRuntimeCapabilityPurposeBytes = 512
)

var (
	runtimeCapabilityQualifiedSymbolPattern = regexp.MustCompile(
		`[A-Za-z_][A-Za-z0-9_]*\.[A-Za-z_][A-Za-z0-9_]*`,
	)
	runtimeCapabilityFunctionDeclarationPattern = regexp.MustCompile(
		`\bfunc[ \t]+[A-Za-z_][A-Za-z0-9_]*`,
	)
)

// RuntimeCapabilityCandidateSummary exposes only one semantic purpose behind
// a call-local opaque token. Code retains the registered source identity,
// package, symbol, declaration, and implementation.
type RuntimeCapabilityCandidateSummary struct {
	CandidateID string `json:"candidate_id"`
	Purpose     string `json:"purpose"`
}

// RuntimeCapabilitySelectionInput names one unresolved semantic leaf: which
// one remaining registered runtime behavior, if any, is necessary for one
// local requirement. Accumulation and the call bound remain code-owned.
type RuntimeCapabilitySelectionInput struct {
	LocalContext     string                              `json:"local_context"`
	Need             string                              `json:"need"`
	Dialect          string                              `json:"dialect"`
	AcceptedPurposes []string                            `json:"accepted_purposes"`
	Candidates       []RuntimeCapabilityCandidateSummary `json:"candidates"`
}

type RuntimeCapabilitySelectionDecision struct {
	Schema   string `json:"schema"`
	Selected string `json:"selected"`
}

func (input RuntimeCapabilitySelectionInput) validate() error {
	if input.LocalContext == "" || input.LocalContext != strings.TrimSpace(input.LocalContext) ||
		len(input.LocalContext) > maxRuntimeCapabilityLocalContext {
		return fmt.Errorf("runtime capability selection requires one bounded trimmed local context")
	}
	if input.Need == "" || input.Need != strings.TrimSpace(input.Need) ||
		len(input.Need) > maxRuntimeCapabilityNeedBytes {
		return fmt.Errorf("runtime capability selection requires one bounded trimmed local need")
	}
	if input.Dialect == "" || input.Dialect != strings.TrimSpace(input.Dialect) ||
		len(input.Dialect) > maxRuntimeCapabilityDialectBytes ||
		strings.ContainsAny(input.Dialect, "\x00\r\n") {
		return fmt.Errorf("runtime capability selection requires one bounded technical dialect")
	}
	if input.AcceptedPurposes == nil {
		return fmt.Errorf("runtime capability selection requires a non-nil accepted-purpose set")
	}
	if len(input.Candidates) < 1 || len(input.Candidates) > maxRuntimeCapabilityCandidates ||
		len(input.AcceptedPurposes)+len(input.Candidates) > maxRuntimeCapabilityCandidates {
		return fmt.Errorf(
			"runtime capability selection requires a remaining closed set within the %d-candidate bound",
			maxRuntimeCapabilityCandidates,
		)
	}
	seenPurposes := make(map[string]struct{}, len(input.AcceptedPurposes)+len(input.Candidates))
	values := []string{input.LocalContext, input.Need, input.Dialect}
	for index, purpose := range input.AcceptedPurposes {
		if err := validateRuntimeCapabilityPurpose(
			fmt.Sprintf("accepted purpose %d", index), purpose,
		); err != nil {
			return err
		}
		if _, duplicate := seenPurposes[purpose]; duplicate {
			return fmt.Errorf("runtime capability selection repeats accepted purpose %q", purpose)
		}
		seenPurposes[purpose] = struct{}{}
		values = append(values, purpose)
	}
	for index, candidate := range input.Candidates {
		wantID := fmt.Sprintf("RUNTIME_CAPABILITY_%d", index+1)
		if candidate.CandidateID != wantID {
			return fmt.Errorf("runtime capability candidate %d ID must be %s", index, wantID)
		}
		if err := validateRuntimeCapabilityPurpose(candidate.CandidateID, candidate.Purpose); err != nil {
			return err
		}
		if _, duplicate := seenPurposes[candidate.Purpose]; duplicate {
			return fmt.Errorf("runtime capability candidate %s repeats another purpose", candidate.CandidateID)
		}
		seenPurposes[candidate.Purpose] = struct{}{}
		values = append(values, candidate.Purpose)
	}
	return ValidatePathFreeModelContext("runtime capability selection", values...)
}

func validateRuntimeCapabilityPurpose(label, purpose string) error {
	if purpose == "" || purpose != strings.TrimSpace(purpose) ||
		len(purpose) > maxRuntimeCapabilityPurposeBytes || strings.ContainsAny(purpose, "\x00\r\n") {
		return fmt.Errorf("runtime capability %s has an invalid bounded purpose", label)
	}
	if runtimeCapabilityQualifiedSymbolPattern.MatchString(purpose) ||
		runtimeCapabilityFunctionDeclarationPattern.MatchString(purpose) {
		return fmt.Errorf("runtime capability %s purpose exposes source identifiers", label)
	}
	return nil
}

func (decision RuntimeCapabilitySelectionDecision) ValidateFor(
	input RuntimeCapabilitySelectionInput,
) error {
	if err := input.validate(); err != nil {
		return err
	}
	if decision.Schema != RuntimeCapabilitySelectionSchemaV1 {
		return fmt.Errorf(
			"runtime capability selection schema must be %q",
			RuntimeCapabilitySelectionSchemaV1,
		)
	}
	if decision.Selected == RuntimeCapabilitySelectionNone {
		return nil
	}
	for _, candidate := range input.Candidates {
		if decision.Selected == candidate.CandidateID {
			return nil
		}
	}
	return fmt.Errorf(
		"selected runtime capability ID %q is outside the code-owned candidate set",
		decision.Selected,
	)
}

func BuildRuntimeCapabilitySelectionPrompt(
	input RuntimeCapabilitySelectionInput,
) (string, error) {
	if err := input.validate(); err != nil {
		return "", err
	}
	lines := []string{
		"Select one remaining runtime behavior only when its exact stated purpose is necessary to fulfill the local need.",
		"Select NONE when the input, direct dependency results, ordinary language operations, and already accepted purposes are sufficient. Convenience alone is insufficient.",
		"LOCAL_CONTEXT:\n" + input.LocalContext,
		"LOCAL_NEED:\n" + input.Need,
		"SOURCE_DIALECT:\n" + input.Dialect,
		"ALREADY_ACCEPTED_PURPOSES:",
	}
	if len(input.AcceptedPurposes) == 0 {
		lines = append(lines, "NONE")
	} else {
		for _, purpose := range input.AcceptedPurposes {
			lines = append(lines, "- "+purpose)
		}
	}
	lines = append(lines, "REMAINING_CANDIDATE_PURPOSES:")
	for _, candidate := range input.Candidates {
		lines = append(lines, candidate.CandidateID+": "+candidate.Purpose)
	}
	lines = append(lines,
		"Return exactly one raw opaque candidate ID or NONE with no JSON, quotes, label, Markdown, explanation, or commentary.",
	)
	return strings.Join(lines, "\n"), nil
}

func DecodeRuntimeCapabilitySelectionDecision(
	input RuntimeCapabilitySelectionInput,
	raw string,
) (RuntimeCapabilitySelectionDecision, error) {
	if err := input.validate(); err != nil {
		return RuntimeCapabilitySelectionDecision{}, err
	}
	leaf, err := decodeRawSemanticLeaf("runtime capability selection", raw, 32, false)
	if err != nil {
		return RuntimeCapabilitySelectionDecision{}, err
	}
	decision := RuntimeCapabilitySelectionDecision{
		Schema: RuntimeCapabilitySelectionSchemaV1, Selected: leaf,
	}
	if err := decision.ValidateFor(input); err != nil {
		return RuntimeCapabilitySelectionDecision{}, err
	}
	return decision, nil
}
