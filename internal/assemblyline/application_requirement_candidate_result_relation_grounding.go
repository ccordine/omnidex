package assemblyline

import (
	"fmt"
	"strings"
)

const (
	WorkApplicationRequirementCandidateResultRelationGrounding WorkKind = "application_requirement_candidate_result_relation_grounding"

	ApplicationRequirementExactlyOneDeterminingRelationEntailed   = "EXACTLY_ONE_DETERMINING_RELATION_ENTAILED"
	ApplicationRequirementNoExactlyOneDeterminingRelationEntailed = "NO_EXACTLY_ONE_DETERMINING_RELATION_ENTAILED"

	ApplicationRequirementCandidateResultRelationGroundingSchemaV1 = "omnidex.application-requirement-candidate-result-relation-grounding.v1"
)

type ApplicationRequirementCandidateResultRelationGroundingInput struct {
	ImmutableRequest      string                                              `json:"immutable_request"`
	Context               ApplicationContext                                  `json:"context"`
	CandidateAuthority    ApplicationRequirementCandidateResultRelationInput  `json:"candidate_authority"`
	MissingResultRelation ApplicationRequirementCandidateResultRelationResult `json:"missing_result_relation"`
}

type ApplicationRequirementCandidateResultRelationGroundingResult struct {
	Schema                             string `json:"schema"`
	ImmutableRequestSHA256             string `json:"immutable_request_sha256"`
	ApplicationContextSHA256           string `json:"application_context_sha256"`
	CandidateSHA256                    string `json:"candidate_sha256"`
	MissingResultRelationReceiptSHA256 string `json:"missing_result_relation_receipt_sha256"`
	Relation                           string `json:"relation"`
}

func (result ApplicationRequirementCandidateResultRelationGroundingResult) ValidateCorrectionAuthority(
	immutableRequest string,
	context ApplicationContext,
	candidate string,
	defect string,
) error {
	if defect != ApplicationRequirementMissingResultRelation {
		return fmt.Errorf(
			"application requirement result-relation correction defect must be %q",
			ApplicationRequirementMissingResultRelation,
		)
	}
	input, err := canonicalApplicationRequirementCandidateResultRelationGroundingInput(
		immutableRequest,
		context,
		candidate,
	)
	if err != nil {
		return err
	}
	if err := result.ValidateFor(input); err != nil {
		return fmt.Errorf("validate result-relation correction grounding receipt: %w", err)
	}
	if result.Relation != ApplicationRequirementExactlyOneDeterminingRelationEntailed {
		return fmt.Errorf(
			"application requirement result-relation correction requires grounded relation %q",
			ApplicationRequirementExactlyOneDeterminingRelationEntailed,
		)
	}
	return nil
}

func NewApplicationRequirementCandidateResultRelationGroundingJob(
	input ApplicationRequirementCandidateResultRelationGroundingInput,
) (PortableJob, error) {
	return newValidatedPortableJob(
		WorkApplicationRequirementCandidateResultRelationGrounding,
		input,
		input.validate,
	)
}

func (input ApplicationRequirementCandidateResultRelationGroundingInput) validate() error {
	if err := (ApplicationIntentInput{
		UserRequest: input.ImmutableRequest,
		Context:     input.Context,
	}).validate(); err != nil {
		return fmt.Errorf("validate result-relation grounding application authority: %w", err)
	}
	if err := input.MissingResultRelation.ValidateFor(input.CandidateAuthority); err != nil {
		return fmt.Errorf("validate result-relation grounding defect receipt: %w", err)
	}
	if input.MissingResultRelation.Relation != ApplicationRequirementMissingResultRelation {
		return fmt.Errorf(
			"application requirement result-relation grounding requires code-established relation %q",
			ApplicationRequirementMissingResultRelation,
		)
	}
	return nil
}

func (result ApplicationRequirementCandidateResultRelationGroundingResult) ValidateFor(
	input ApplicationRequirementCandidateResultRelationGroundingInput,
) error {
	if err := input.validate(); err != nil {
		return err
	}
	if result.Schema != ApplicationRequirementCandidateResultRelationGroundingSchemaV1 {
		return fmt.Errorf(
			"application requirement result-relation grounding schema must be %q",
			ApplicationRequirementCandidateResultRelationGroundingSchemaV1,
		)
	}
	if result.ImmutableRequestSHA256 != ExactObjectiveContextSHA(input.ImmutableRequest) {
		return fmt.Errorf("application requirement result-relation grounding request hash does not match")
	}
	contextSHA256, err := applicationRequirementSemanticReceiptSHA256(input.Context)
	if err != nil {
		return fmt.Errorf("hash application context authority: %w", err)
	}
	if result.ApplicationContextSHA256 != contextSHA256 {
		return fmt.Errorf("application requirement result-relation grounding context hash does not match")
	}
	if result.CandidateSHA256 != ExactObjectiveContextSHA(input.CandidateAuthority.Candidate) {
		return fmt.Errorf("application requirement result-relation grounding candidate hash does not match")
	}
	receiptSHA256, err := applicationRequirementSemanticReceiptSHA256(
		input.MissingResultRelation,
	)
	if err != nil {
		return fmt.Errorf("hash missing result-relation receipt: %w", err)
	}
	if result.MissingResultRelationReceiptSHA256 != receiptSHA256 {
		return fmt.Errorf("application requirement result-relation grounding defect receipt hash does not match")
	}
	switch result.Relation {
	case ApplicationRequirementExactlyOneDeterminingRelationEntailed,
		ApplicationRequirementNoExactlyOneDeterminingRelationEntailed:
		return nil
	default:
		return fmt.Errorf(
			"application requirement result-relation grounding value %q is not registered",
			result.Relation,
		)
	}
}

func BuildApplicationRequirementCandidateResultRelationGroundingPrompt(
	input ApplicationRequirementCandidateResultRelationGroundingInput,
) (string, error) {
	if err := input.validate(); err != nil {
		return "", err
	}
	authorityProjection := renderApplicationContextModelProjection(
		input.ImmutableRequest,
		input.Context,
	)
	return strings.Join([]string{
		"Answer one semantic relation: do the immutable request and established facts entail exactly one determining relation for the derived-result outcome named by the exact current candidate?",
		"The code-bound defect is exact: the candidate requires a derived result but omits information needed to determine that result independently. Do not reconsider or repair the candidate.",
		"Return EXACTLY_ONE_DETERMINING_RELATION_ENTAILED only when the immutable request and established facts together fix one operation or rule and all observable operands, conditions, and result meaning needed for an independent oracle. A conventional default, common product expectation, optional policy, implementation choice, or one of several valid rules is not entailed.",
		"Return NO_EXACTLY_ONE_DETERMINING_RELATION_ENTAILED when any required part is absent or ambiguous. Do not propose a rule, rewrite the candidate, add an example value, or decide what the workflow should do.",
		"Return exactly one raw registered value and nothing else: no JSON, quotes, label, Markdown, or commentary.",
		"APPLICATION AUTHORITY:\n" + authorityProjection,
		"EXACT CURRENT CANDIDATE:\n" + input.CandidateAuthority.Candidate,
		"EXACT CODE-ESTABLISHED DEFECT:\n" + input.MissingResultRelation.Relation,
		"FINAL QUESTION:\nDo the immutable request and established facts entail exactly one determining relation for this outcome? Return only EXACTLY_ONE_DETERMINING_RELATION_ENTAILED or NO_EXACTLY_ONE_DETERMINING_RELATION_ENTAILED.",
	}, "\n\n"), nil
}

func DecodeApplicationRequirementCandidateResultRelationGroundingResult(
	input ApplicationRequirementCandidateResultRelationGroundingInput,
	raw string,
) (ApplicationRequirementCandidateResultRelationGroundingResult, error) {
	var zero ApplicationRequirementCandidateResultRelationGroundingResult
	if err := input.validate(); err != nil {
		return zero, err
	}
	leaf, err := decodeRawSemanticLeaf(
		"application requirement candidate result-relation grounding",
		raw,
		maximumStringBytes(
			ApplicationRequirementExactlyOneDeterminingRelationEntailed,
			ApplicationRequirementNoExactlyOneDeterminingRelationEntailed,
		),
		false,
	)
	if err != nil {
		return zero, err
	}
	receiptSHA256, err := applicationRequirementSemanticReceiptSHA256(
		input.MissingResultRelation,
	)
	if err != nil {
		return zero, fmt.Errorf("hash missing result-relation receipt: %w", err)
	}
	contextSHA256, err := applicationRequirementSemanticReceiptSHA256(input.Context)
	if err != nil {
		return zero, fmt.Errorf("hash application context authority: %w", err)
	}
	result := ApplicationRequirementCandidateResultRelationGroundingResult{
		Schema:                             ApplicationRequirementCandidateResultRelationGroundingSchemaV1,
		ImmutableRequestSHA256:             ExactObjectiveContextSHA(input.ImmutableRequest),
		ApplicationContextSHA256:           contextSHA256,
		CandidateSHA256:                    ExactObjectiveContextSHA(input.CandidateAuthority.Candidate),
		MissingResultRelationReceiptSHA256: receiptSHA256,
		Relation:                           leaf,
	}
	if err := result.ValidateFor(input); err != nil {
		return zero, err
	}
	return result, nil
}

func canonicalApplicationRequirementCandidateResultRelationGroundingInput(
	immutableRequest string,
	context ApplicationContext,
	candidate string,
) (ApplicationRequirementCandidateResultRelationGroundingInput, error) {
	candidateAuthority := canonicalAcceptedApplicationRequirementResultRelationInput(candidate)
	missing, err := DecodeApplicationRequirementCandidateResultRelationResult(
		candidateAuthority,
		ApplicationRequirementMissingResultRelation,
	)
	if err != nil {
		return ApplicationRequirementCandidateResultRelationGroundingInput{}, fmt.Errorf(
			"construct canonical missing result-relation receipt: %w",
			err,
		)
	}
	return ApplicationRequirementCandidateResultRelationGroundingInput{
		ImmutableRequest:      immutableRequest,
		Context:               context,
		CandidateAuthority:    candidateAuthority,
		MissingResultRelation: missing,
	}, nil
}
