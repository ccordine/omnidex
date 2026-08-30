package assemblyline

import (
	"fmt"
	"strings"

	"github.com/gryph/omnidex/internal/exactjson"
)

const (
	WorkApplicationRequirementCandidateKind WorkKind = "application_requirement_candidate_kind"

	ApplicationRequirementCandidateTaskLocal  = "TASK_LOCAL_RUNTIME_OUTCOME"
	ApplicationRequirementCandidateNonRuntime = "NON_RUNTIME_CONSTRAINT"
	ApplicationRequirementCandidateMixed      = "MIXED_RUNTIME_AND_NON_RUNTIME"

	ApplicationRequirementCandidateRuntimeContentDimension    ApplicationRequirementCandidateContentDimension = "runtime_content"
	ApplicationRequirementCandidateNonRuntimeContentDimension ApplicationRequirementCandidateContentDimension = "non_runtime_content"

	ApplicationRequirementCandidateContentPresent ApplicationRequirementCandidateContentPresence = "PRESENT"
	ApplicationRequirementCandidateContentAbsent  ApplicationRequirementCandidateContentPresence = "ABSENT"

	ApplicationRequirementCandidateContentPresenceSchemaV1 = "omnidex.application-requirement-candidate-content-presence.v1"
	ApplicationRequirementCandidateKindSchemaV1            = "omnidex.application-requirement-candidate-kind.v1"
)

type ApplicationRequirementCandidateContentDimension string

type ApplicationRequirementCandidateContentPresence string

type ApplicationRequirementCandidateContentPresenceInput struct {
	Candidate string                                          `json:"candidate"`
	Dimension ApplicationRequirementCandidateContentDimension `json:"dimension"`
}

type ApplicationRequirementCandidateContentPresenceResult struct {
	Schema          string                                         `json:"schema"`
	AuthoritySHA256 string                                         `json:"authority_sha256"`
	Presence        ApplicationRequirementCandidateContentPresence `json:"presence"`
}

// ApplicationRequirementCandidateKindInput binds the code-owned final kind
// receipt to one exact candidate. It is not a model input.
type ApplicationRequirementCandidateKindInput struct {
	Candidate string `json:"candidate"`
}

type ApplicationRequirementCandidateKindResult struct {
	Schema          string `json:"schema"`
	CandidateSHA256 string `json:"candidate_sha256"`
	Relation        string `json:"relation"`
}

func NewApplicationRequirementCandidateContentPresenceJob(
	input ApplicationRequirementCandidateContentPresenceInput,
) (PortableJob, error) {
	return newValidatedPortableJob(
		WorkApplicationRequirementCandidateKind,
		input,
		input.validate,
	)
}

func (input ApplicationRequirementCandidateContentPresenceInput) validate() error {
	if err := validateApplicationIntentText(
		"application requirement candidate",
		input.Candidate,
		maxRequirementQuoteBytes,
	); err != nil {
		return err
	}
	switch input.Dimension {
	case ApplicationRequirementCandidateRuntimeContentDimension,
		ApplicationRequirementCandidateNonRuntimeContentDimension:
		return nil
	default:
		return fmt.Errorf(
			"application requirement candidate content dimension %q is not registered",
			input.Dimension,
		)
	}
}

func (input ApplicationRequirementCandidateKindInput) validate() error {
	return validateApplicationIntentText(
		"application requirement candidate",
		input.Candidate,
		maxRequirementQuoteBytes,
	)
}

func (result ApplicationRequirementCandidateContentPresenceResult) ValidateFor(
	input ApplicationRequirementCandidateContentPresenceInput,
) error {
	if err := input.validate(); err != nil {
		return err
	}
	if result.Schema != ApplicationRequirementCandidateContentPresenceSchemaV1 {
		return fmt.Errorf(
			"application requirement candidate content presence schema must be %q",
			ApplicationRequirementCandidateContentPresenceSchemaV1,
		)
	}
	authoritySHA256, err := applicationRequirementCandidateContentPresenceAuthoritySHA256(input)
	if err != nil {
		return err
	}
	if result.AuthoritySHA256 != authoritySHA256 {
		return fmt.Errorf(
			"application requirement candidate content presence authority hash does not match",
		)
	}
	switch result.Presence {
	case ApplicationRequirementCandidateContentPresent,
		ApplicationRequirementCandidateContentAbsent:
		return nil
	default:
		return fmt.Errorf(
			"application requirement candidate content presence %q is not registered",
			result.Presence,
		)
	}
}

func (result ApplicationRequirementCandidateKindResult) ValidateFor(
	input ApplicationRequirementCandidateKindInput,
) error {
	if err := input.validate(); err != nil {
		return err
	}
	if result.Schema != ApplicationRequirementCandidateKindSchemaV1 {
		return fmt.Errorf(
			"application requirement candidate kind schema must be %q",
			ApplicationRequirementCandidateKindSchemaV1,
		)
	}
	if result.CandidateSHA256 != ExactObjectiveContextSHA(input.Candidate) {
		return fmt.Errorf("application requirement candidate kind hash does not match")
	}
	switch result.Relation {
	case ApplicationRequirementCandidateTaskLocal,
		ApplicationRequirementCandidateNonRuntime,
		ApplicationRequirementCandidateMixed:
		return nil
	default:
		return fmt.Errorf(
			"application requirement candidate kind value %q is not registered",
			result.Relation,
		)
	}
}

func BuildApplicationRequirementCandidateContentPresencePrompt(
	input ApplicationRequirementCandidateContentPresenceInput,
) (string, error) {
	if err := input.validate(); err != nil {
		return "", err
	}
	var dimensionQuestion []string
	switch input.Dimension {
	case ApplicationRequirementCandidateRuntimeContentDimension:
		dimensionQuestion = []string{
			"Answer one semantic presence question about the exact candidate: does it specify anything the finished software itself must do, show, accept, change, store, or produce while running?",
			"Runtime content includes a software behavior, observable result, user-visible element or quality, state or persistence behavior, or runtime data or output format.",
			"Return PRESENT when the candidate directly requires an action or result of completed software, including a subjectless imperative acting on runtime or user-provided data. A product or category name alone is not direct behavior for this question. Do not infer customary controls, variants, features, prerequisites, or presentation.",
			"FINAL QUESTION:\nIs directly stated finished-software runtime content PRESENT or ABSENT? Return only PRESENT or ABSENT.",
		}
	case ApplicationRequirementCandidateNonRuntimeContentDimension:
		dimensionQuestion = []string{
			"Answer one semantic presence question about the exact candidate: does it explicitly say how or where the software must be constructed, implemented, packaged, delivered, run, deployed, or verified?",
			"The instruction to build or create the requested software is itself construction content: when either direction is present, return PRESENT regardless of any runtime meaning in the same candidate. Also return PRESENT for a stated delivery surface, language, framework, toolchain, version, packaging, tree or named-artifact shape, verification obligation, deployment obligation, or continued-availability obligation.",
			"Such content remains PRESENT when the same candidate also names runtime behavior. A subject that merely refers to finished software while stating its behavior is not enough. Determine only whether construction or delivery content is present; do not classify the candidate as a whole.",
			"FINAL QUESTION:\nIs construction-or-delivery constraint content PRESENT or ABSENT? Return only PRESENT or ABSENT.",
		}
	default:
		return "", fmt.Errorf(
			"application requirement candidate content dimension %q is not registered",
			input.Dimension,
		)
	}
	return strings.Join([]string{
		dimensionQuestion[0],
		dimensionQuestion[1],
		dimensionQuestion[2],
		"Inspect only the exact candidate. Do not rewrite it, infer another requirement, or use surrounding context.",
		"Return only the raw registered presence with no JSON, label, Markdown, or explanation.",
		"EXACT REQUIREMENT CANDIDATE:\n" + input.Candidate,
		dimensionQuestion[3],
	}, "\n\n"), nil
}

func DecodeApplicationRequirementCandidateContentPresenceResult(
	input ApplicationRequirementCandidateContentPresenceInput,
	raw string,
) (ApplicationRequirementCandidateContentPresenceResult, error) {
	var zero ApplicationRequirementCandidateContentPresenceResult
	if err := input.validate(); err != nil {
		return zero, err
	}
	leaf, err := decodeRawSemanticLeaf(
		"application requirement candidate content presence",
		raw,
		maximumStringBytes(
			string(ApplicationRequirementCandidateContentPresent),
			string(ApplicationRequirementCandidateContentAbsent),
		),
		false,
	)
	if err != nil {
		return zero, err
	}
	authoritySHA256, err := applicationRequirementCandidateContentPresenceAuthoritySHA256(input)
	if err != nil {
		return zero, err
	}
	result := ApplicationRequirementCandidateContentPresenceResult{
		Schema:          ApplicationRequirementCandidateContentPresenceSchemaV1,
		AuthoritySHA256: authoritySHA256,
		Presence:        ApplicationRequirementCandidateContentPresence(leaf),
	}
	if err := result.ValidateFor(input); err != nil {
		return zero, err
	}
	return result, nil
}

// ResolveApplicationRequirementCandidateKind folds two independently bound
// presence receipts into the code-owned three-way candidate kind. Two valid
// ABSENT receipts leave this candidate unresolved so its caller can discard
// only that candidate without manufacturing a kind.
func ResolveApplicationRequirementCandidateKind(
	candidate string,
	runtimeContent ApplicationRequirementCandidateContentPresenceResult,
	nonRuntimeContent ApplicationRequirementCandidateContentPresenceResult,
) (ApplicationRequirementCandidateKindResult, bool, error) {
	var zero ApplicationRequirementCandidateKindResult
	input := ApplicationRequirementCandidateKindInput{Candidate: candidate}
	if err := input.validate(); err != nil {
		return zero, false, err
	}
	runtimeInput := ApplicationRequirementCandidateContentPresenceInput{
		Candidate: candidate,
		Dimension: ApplicationRequirementCandidateRuntimeContentDimension,
	}
	if err := runtimeContent.ValidateFor(runtimeInput); err != nil {
		return zero, false, fmt.Errorf("validate runtime-content presence: %w", err)
	}
	nonRuntimeInput := ApplicationRequirementCandidateContentPresenceInput{
		Candidate: candidate,
		Dimension: ApplicationRequirementCandidateNonRuntimeContentDimension,
	}
	if err := nonRuntimeContent.ValidateFor(nonRuntimeInput); err != nil {
		return zero, false, fmt.Errorf("validate non-runtime-content presence: %w", err)
	}

	var relation string
	switch {
	case runtimeContent.Presence == ApplicationRequirementCandidateContentPresent &&
		nonRuntimeContent.Presence == ApplicationRequirementCandidateContentPresent:
		relation = ApplicationRequirementCandidateMixed
	case runtimeContent.Presence == ApplicationRequirementCandidateContentPresent:
		relation = ApplicationRequirementCandidateTaskLocal
	case nonRuntimeContent.Presence == ApplicationRequirementCandidateContentPresent:
		relation = ApplicationRequirementCandidateNonRuntime
	default:
		return zero, false, nil
	}
	result := ApplicationRequirementCandidateKindResult{
		Schema:          ApplicationRequirementCandidateKindSchemaV1,
		CandidateSHA256: ExactObjectiveContextSHA(candidate),
		Relation:        relation,
	}
	if err := result.ValidateFor(input); err != nil {
		return zero, false, err
	}
	return result, true, nil
}

func applicationRequirementCandidateContentPresenceAuthoritySHA256(
	input ApplicationRequirementCandidateContentPresenceInput,
) (string, error) {
	if err := input.validate(); err != nil {
		return "", err
	}
	authority, err := exactjson.Canonical(input)
	if err != nil {
		return "", fmt.Errorf(
			"encode application requirement candidate content presence authority: %w",
			err,
		)
	}
	return ExactObjectiveContextSHA(string(authority)), nil
}
