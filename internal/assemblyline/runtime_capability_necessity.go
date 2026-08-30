package assemblyline

import (
	"fmt"
	"regexp"
	"strings"
)

const (
	RuntimeCapabilityNecessitySchemaV1 = "omnidex.runtime-capability-necessity.v1"

	RuntimeCapabilityNecessary    = "RUNTIME_CAPABILITY_NECESSARY"
	RuntimeCapabilityNotNecessary = "RUNTIME_CAPABILITY_NOT_NECESSARY"

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

// RuntimeCapabilityNecessityInput exposes one candidate's semantic purpose.
// Code retains its registered identity, package, declaration, implementation,
// ordering, accepted set, and queue state.
type RuntimeCapabilityNecessityInput struct {
	LocalContext     string `json:"local_context"`
	Need             string `json:"need"`
	Dialect          string `json:"dialect"`
	CandidatePurpose string `json:"candidate_purpose"`
}

type RuntimeCapabilityNecessityDecision struct {
	Schema   string `json:"schema"`
	Relation string `json:"relation"`
}

func (input RuntimeCapabilityNecessityInput) validate() error {
	if input.LocalContext == "" || input.LocalContext != strings.TrimSpace(input.LocalContext) ||
		len(input.LocalContext) > maxRuntimeCapabilityLocalContext {
		return fmt.Errorf("runtime capability necessity requires one bounded trimmed local context")
	}
	if input.Need == "" || input.Need != strings.TrimSpace(input.Need) ||
		len(input.Need) > maxRuntimeCapabilityNeedBytes {
		return fmt.Errorf("runtime capability necessity requires one bounded trimmed local need")
	}
	if input.Dialect == "" || input.Dialect != strings.TrimSpace(input.Dialect) ||
		len(input.Dialect) > maxRuntimeCapabilityDialectBytes ||
		strings.ContainsAny(input.Dialect, "\x00\r\n") {
		return fmt.Errorf("runtime capability necessity requires one bounded technical dialect")
	}
	if err := validateRuntimeCapabilityPurpose(
		"candidate", input.CandidatePurpose,
	); err != nil {
		return err
	}
	return ValidatePathFreeModelContext(
		"runtime capability necessity",
		input.LocalContext,
		input.Need,
		input.Dialect,
		input.CandidatePurpose,
	)
}

func validateRuntimeCapabilityPurpose(label, purpose string) error {
	if purpose == "" || purpose != strings.TrimSpace(purpose) ||
		len(purpose) > maxRuntimeCapabilityPurposeBytes || strings.ContainsAny(purpose, "\x00\r\n") {
		return fmt.Errorf("runtime capability %s purpose has invalid bounded text", label)
	}
	if runtimeCapabilityQualifiedSymbolPattern.MatchString(purpose) ||
		runtimeCapabilityFunctionDeclarationPattern.MatchString(purpose) {
		return fmt.Errorf("runtime capability %s purpose exposes source identifiers", label)
	}
	return nil
}

func (decision RuntimeCapabilityNecessityDecision) ValidateFor(
	input RuntimeCapabilityNecessityInput,
) error {
	if err := input.validate(); err != nil {
		return err
	}
	if decision.Schema != RuntimeCapabilityNecessitySchemaV1 {
		return fmt.Errorf(
			"runtime capability necessity schema must be %q",
			RuntimeCapabilityNecessitySchemaV1,
		)
	}
	switch decision.Relation {
	case RuntimeCapabilityNecessary, RuntimeCapabilityNotNecessary:
		return nil
	default:
		return fmt.Errorf(
			"runtime capability necessity relation %q is unsupported",
			decision.Relation,
		)
	}
}

func BuildRuntimeCapabilityNecessityPrompt(
	input RuntimeCapabilityNecessityInput,
) (string, error) {
	if err := input.validate(); err != nil {
		return "", err
	}
	return strings.Join([]string{
		"Answer one semantic necessity relation: is this exact candidate runtime behavior necessary to fulfill the exact local need in the supplied context and source dialect?",
		"Return RUNTIME_CAPABILITY_NECESSARY only when the local need cannot be fulfilled with ordinary language/runtime operations and the direct-dependency results named in local context without this candidate's exact behavior. Convenience, possible reuse, customary design, or mere usefulness is not necessity.",
		"Return exactly RUNTIME_CAPABILITY_NECESSARY or RUNTIME_CAPABILITY_NOT_NECESSARY with no candidate ID, JSON, quotes, label, Markdown, explanation, or commentary.",
		"LOCAL_CONTEXT:\n" + input.LocalContext,
		"LOCAL_NEED:\n" + input.Need,
		"SOURCE_DIALECT:\n" + input.Dialect,
		"EXACT_CANDIDATE_RUNTIME_PURPOSE:\n" + input.CandidatePurpose,
	}, "\n\n"), nil
}

func DecodeRuntimeCapabilityNecessityDecision(
	input RuntimeCapabilityNecessityInput,
	raw string,
) (RuntimeCapabilityNecessityDecision, error) {
	if err := input.validate(); err != nil {
		return RuntimeCapabilityNecessityDecision{}, err
	}
	leaf, err := decodeRawSemanticLeaf(
		"runtime capability necessity",
		raw,
		maximumStringBytes(RuntimeCapabilityNecessary, RuntimeCapabilityNotNecessary),
		false,
	)
	if err != nil {
		return RuntimeCapabilityNecessityDecision{}, err
	}
	decision := RuntimeCapabilityNecessityDecision{
		Schema: RuntimeCapabilityNecessitySchemaV1, Relation: leaf,
	}
	if err := decision.ValidateFor(input); err != nil {
		return RuntimeCapabilityNecessityDecision{}, err
	}
	return decision, nil
}
