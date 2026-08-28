package assemblyline

import (
	"fmt"
	"strings"
)

const (
	WorkGroundedAnswerText             WorkKind = "grounded_answer_text"
	WorkGroundedAnswerEvidenceRelation WorkKind = "grounded_answer_evidence_relation"

	GroundedEvidenceSupportsAnswer GroundedAnswerEvidenceRelation = "SUPPORTS_ANSWER"
	GroundedEvidenceDoesNotSupport GroundedAnswerEvidenceRelation = "DOES_NOT_SUPPORT_ANSWER"
)

type GroundedAnswerEvidenceRelation string

type GroundedAnswerTextInput struct {
	ExactRequirement string                    `json:"exact_requirement"`
	Context          ObjectiveContext          `json:"objective_context"`
	Evidence         []GroundedEvidenceCapsule `json:"evidence"`
}

type GroundedAnswerTextDecision struct {
	Text string `json:"text"`
}

type GroundedAnswerEvidenceRelationInput struct {
	ExactRequirement string                  `json:"exact_requirement"`
	Context          ObjectiveContext        `json:"objective_context"`
	AnswerText       string                  `json:"answer_text"`
	Evidence         GroundedEvidenceCapsule `json:"evidence"`
}

type GroundedAnswerEvidenceRelationDecision struct {
	Relation GroundedAnswerEvidenceRelation `json:"relation"`
}

type groundedAnswerTextProjection struct {
	ExactRequirement string           `json:"exact_requirement"`
	Context          ObjectiveContext `json:"objective_context"`
	Evidence         []string         `json:"evidence"`
}

type groundedAnswerEvidenceRelationProjection struct {
	ExactRequirement string           `json:"exact_requirement"`
	Context          ObjectiveContext `json:"objective_context"`
	AnswerText       string           `json:"answer_text"`
	EvidenceText     string           `json:"evidence_text"`
}

func NewGroundedAnswerTextJob(input GroundedAnswerTextInput) (PortableJob, error) {
	return newValidatedPortableJob(WorkGroundedAnswerText, input, input.validate)
}

func NewGroundedAnswerEvidenceRelationJob(
	input GroundedAnswerEvidenceRelationInput,
) (PortableJob, error) {
	return newValidatedPortableJob(
		WorkGroundedAnswerEvidenceRelation, input, input.validate,
	)
}

func (input GroundedAnswerTextInput) validate() error {
	return validateGroundedAnswerAuthority(
		input.ExactRequirement, input.Context, input.Evidence,
	)
}

func (input GroundedAnswerEvidenceRelationInput) validate() error {
	if err := validateGroundedAnswerAuthority(
		input.ExactRequirement,
		input.Context,
		[]GroundedEvidenceCapsule{input.Evidence},
	); err != nil {
		return err
	}
	return validateGroundedText(
		"answer text", input.AnswerText, maxGroundedAnswerTextBytes, true,
	)
}

func (decision GroundedAnswerTextDecision) ValidateFor(
	input GroundedAnswerTextInput,
) error {
	if err := input.validate(); err != nil {
		return err
	}
	return validateGroundedText(
		"answer text", decision.Text, maxGroundedAnswerTextBytes, true,
	)
}

func (decision GroundedAnswerEvidenceRelationDecision) ValidateFor(
	input GroundedAnswerEvidenceRelationInput,
) error {
	if err := input.validate(); err != nil {
		return err
	}
	switch decision.Relation {
	case GroundedEvidenceSupportsAnswer, GroundedEvidenceDoesNotSupport:
		return nil
	default:
		return fmt.Errorf(
			"grounded answer evidence relation %q is unsupported",
			decision.Relation,
		)
	}
}

func BuildGroundedAnswerTextPrompt(input GroundedAnswerTextInput) (string, error) {
	if err := input.validate(); err != nil {
		return "", err
	}
	evidence := make([]string, len(input.Evidence))
	for index, capsule := range input.Evidence {
		evidence[index] = capsule.Text
	}
	projection, err := marshalObjectiveContextInputForModel(
		groundedAnswerTextProjection{
			ExactRequirement: input.ExactRequirement,
			Context:          input.Context, Evidence: evidence,
		},
		input.Context,
	)
	if err != nil {
		return "", fmt.Errorf("encode grounded answer text authority: %w", err)
	}
	return strings.Join([]string{
		"Answer one exact requirement using only the supplied evidence capsules. Every factual claim in the answer must be supported by at least one capsule.",
		"Return exactly one raw answer-text leaf. Do not return evidence IDs, citation syntax, JSON, quotes, a label, Markdown wrapping, or commentary.",
		"GROUNDING_AUTHORITY:\n" + string(projection),
	}, "\n\n"), nil
}

func BuildGroundedAnswerEvidenceRelationPrompt(
	input GroundedAnswerEvidenceRelationInput,
) (string, error) {
	if err := input.validate(); err != nil {
		return "", err
	}
	projection, err := marshalObjectiveContextInputForModel(
		groundedAnswerEvidenceRelationProjection{
			ExactRequirement: input.ExactRequirement,
			Context:          input.Context, AnswerText: input.AnswerText,
			EvidenceText: input.Evidence.Text,
		},
		input.Context,
	)
	if err != nil {
		return "", fmt.Errorf("encode grounded answer evidence relation authority: %w", err)
	}
	return strings.Join([]string{
		"Answer one semantic question: does this one evidence capsule materially support at least one factual claim in the exact answer?",
		"Return exactly SUPPORTS_ANSWER or DOES_NOT_SUPPORT_ANSWER. Return no JSON, quotes, label, explanation, or commentary.",
		"EVIDENCE_RELATION_AUTHORITY:\n" + string(projection),
	}, "\n\n"), nil
}

func DecodeGroundedAnswerTextDecision(
	input GroundedAnswerTextInput,
	raw string,
) (GroundedAnswerTextDecision, error) {
	leaf, err := decodeRawSemanticLeaf(
		"grounded answer text", raw, maxGroundedAnswerTextBytes, true,
	)
	if err != nil {
		return GroundedAnswerTextDecision{}, err
	}
	decision := GroundedAnswerTextDecision{Text: leaf}
	if err := decision.ValidateFor(input); err != nil {
		return GroundedAnswerTextDecision{}, err
	}
	return decision, nil
}

func DecodeGroundedAnswerEvidenceRelationDecision(
	input GroundedAnswerEvidenceRelationInput,
	raw string,
) (GroundedAnswerEvidenceRelationDecision, error) {
	leaf, err := decodeRawSemanticLeaf(
		"grounded answer evidence relation", raw,
		len(GroundedEvidenceDoesNotSupport), false,
	)
	if err != nil {
		return GroundedAnswerEvidenceRelationDecision{}, err
	}
	decision := GroundedAnswerEvidenceRelationDecision{
		Relation: GroundedAnswerEvidenceRelation(leaf),
	}
	if err := decision.ValidateFor(input); err != nil {
		return GroundedAnswerEvidenceRelationDecision{}, err
	}
	return decision, nil
}
