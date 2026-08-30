package assemblyline

import (
	"fmt"
	"strings"

	"github.com/gryph/omnidex/internal/exactjson"
)

const (
	WorkApplicationRequirementCandidateAuthorization WorkKind = "application_requirement_candidate_authorization"

	ApplicationRequirementCandidateEntailed    = "ENTAILED_BY_CURRENT_REQUEST"
	ApplicationRequirementCandidateNotEntailed = "NOT_ENTAILED_BY_CURRENT_REQUEST"

	ApplicationRequirementCandidateAuthorizationSchemaV1 = "omnidex.application-requirement-candidate-authorization.v1"
)

type ApplicationRequirementCandidateAuthorizationInput struct {
	UserRequest string             `json:"user_request"`
	Context     ApplicationContext `json:"context"`
	Candidate   string             `json:"candidate"`
}

type ApplicationRequirementCandidateAuthorizationResult struct {
	Schema          string `json:"schema"`
	AuthoritySHA256 string `json:"authority_sha256"`
	Relation        string `json:"relation"`
}

func NewApplicationRequirementCandidateAuthorizationJob(
	input ApplicationRequirementCandidateAuthorizationInput,
) (PortableJob, error) {
	return newValidatedPortableJob(
		WorkApplicationRequirementCandidateAuthorization,
		input,
		input.validate,
	)
}

// ResolveExactSourceApplicationRequirementCandidateAuthorization resolves the
// only authorization relation code can prove from text identity alone. An
// exact copy of the complete immutable request is necessarily entailed by that
// request. Substrings, paraphrases, and context-derived candidates remain
// unresolved and require the ordinary semantic relation.
func ResolveExactSourceApplicationRequirementCandidateAuthorization(
	input ApplicationRequirementCandidateAuthorizationInput,
) (ApplicationRequirementCandidateAuthorizationResult, bool, error) {
	var zero ApplicationRequirementCandidateAuthorizationResult
	if err := input.validate(); err != nil {
		return zero, false, err
	}
	if input.Candidate != input.UserRequest {
		return zero, false, nil
	}
	result, err := DecodeApplicationRequirementCandidateAuthorizationResult(
		input,
		ApplicationRequirementCandidateEntailed,
	)
	if err != nil {
		return zero, false, err
	}
	return result, true, nil
}

func (input ApplicationRequirementCandidateAuthorizationInput) validate() error {
	if err := (ApplicationIntentInput{
		UserRequest: input.UserRequest,
		Context:     input.Context,
	}).validate(); err != nil {
		return err
	}
	return validateApplicationIntentText(
		"requirement authorization candidate",
		input.Candidate,
		maxRequirementQuoteBytes,
	)
}

func BuildApplicationRequirementCandidateAuthorizationPrompt(
	input ApplicationRequirementCandidateAuthorizationInput,
) (string, error) {
	if err := input.validate(); err != nil {
		return "", err
	}
	projection := renderApplicationContextModelProjection(input.UserRequest, input.Context)
	return strings.Join([]string{
		"Answer one semantic entailment question about the exact candidate below: is its complete semantic meaning required by the immutable current request together with only its established facts?",
		"Entailment is semantic, not textual identity. A direct imperative that states an operation over runtime input is itself a finished-software runtime requirement. Recasting 'Process each supplied item.' as 'The finished software processes each supplied item.' adds no semantic detail. Ordinary grammatical inflection, a neutral finished-software subject, or an exact synonym likewise adds no meaning. When those are the only differences, return ENTAILED_BY_CURRENT_REQUEST.",
		"A purpose-bearing product or category name entails only the literal core runtime action or governed result denoted by that name. Expressing the purpose noun as its corresponding action or adding a neutral runtime subject adds no meaning. The name does not entail customary controls, variants, history, persistence, presentation, process steps, triggers, or enhancements unless the request separately says so.",
		"A direction to build, test, check, or verify may itself be request-grounded and may also contain an embedded assertion about required runtime behavior.",
		"A framework, language, database, toolchain, delivery-surface, packaging, test, or deployment instruction is construction authority only. By itself it does not entail that the finished software renders an interface, stores data, exposes a service, or performs any other runtime outcome.",
		"Return ENTAILED_BY_CURRENT_REQUEST only when every semantic detail in the exact candidate is grounded by the current request. Return NOT_ENTAILED_BY_CURRENT_REQUEST if the candidate adds any unstated mechanism, interface, device or input source, algorithm, mode, prerequisite, customary feature, speculative enhancement, or merely useful behavior. An entailed core does not excuse one added detail.",
		"Return only the raw registered relation, with no JSON, label, Markdown, or explanation.",
		"IMMUTABLE CURRENT REQUEST AND ESTABLISHED FACTS:\n" + projection,
		"EXACT CANDIDATE:\n" + input.Candidate,
		"FINAL QUESTION:\nIs the complete semantic content of this exact candidate entailed by the immutable current request? Return only ENTAILED_BY_CURRENT_REQUEST or NOT_ENTAILED_BY_CURRENT_REQUEST.",
	}, "\n\n"), nil
}

func DecodeApplicationRequirementCandidateAuthorizationResult(
	input ApplicationRequirementCandidateAuthorizationInput,
	raw string,
) (ApplicationRequirementCandidateAuthorizationResult, error) {
	var zero ApplicationRequirementCandidateAuthorizationResult
	if err := input.validate(); err != nil {
		return zero, err
	}
	leaf, err := decodeRawSemanticLeaf(
		"application requirement candidate authorization",
		raw,
		maximumStringBytes(
			ApplicationRequirementCandidateEntailed,
			ApplicationRequirementCandidateNotEntailed,
		),
		false,
	)
	if err != nil {
		return zero, err
	}
	authoritySHA256, err := applicationRequirementCandidateAuthorizationAuthoritySHA256(input)
	if err != nil {
		return zero, err
	}
	result := ApplicationRequirementCandidateAuthorizationResult{
		Schema:          ApplicationRequirementCandidateAuthorizationSchemaV1,
		AuthoritySHA256: authoritySHA256,
		Relation:        leaf,
	}
	if err := result.ValidateFor(input); err != nil {
		return zero, err
	}
	return result, nil
}

func (result ApplicationRequirementCandidateAuthorizationResult) ValidateFor(
	input ApplicationRequirementCandidateAuthorizationInput,
) error {
	if err := input.validate(); err != nil {
		return err
	}
	if result.Schema != ApplicationRequirementCandidateAuthorizationSchemaV1 {
		return fmt.Errorf(
			"application requirement candidate authorization schema must be %q",
			ApplicationRequirementCandidateAuthorizationSchemaV1,
		)
	}
	authoritySHA256, err := applicationRequirementCandidateAuthorizationAuthoritySHA256(input)
	if err != nil {
		return err
	}
	if result.AuthoritySHA256 != authoritySHA256 {
		return fmt.Errorf("application requirement candidate authorization authority hash does not match")
	}
	switch result.Relation {
	case ApplicationRequirementCandidateEntailed,
		ApplicationRequirementCandidateNotEntailed:
		return nil
	default:
		return fmt.Errorf(
			"application requirement candidate authorization value %q is not registered",
			result.Relation,
		)
	}
}

func applicationRequirementCandidateAuthorizationAuthoritySHA256(
	input ApplicationRequirementCandidateAuthorizationInput,
) (string, error) {
	if err := input.validate(); err != nil {
		return "", err
	}
	authority, err := exactjson.Canonical(input)
	if err != nil {
		return "", fmt.Errorf("encode application requirement candidate authorization authority: %w", err)
	}
	return ExactObjectiveContextSHA(string(authority)), nil
}
