package assemblyline

import (
	"encoding/json"
	"fmt"
	"strings"
)

type applicationJobSpecificationReviewAuthority struct {
	Surface            ApplicationSurface `json:"surface"`
	FocusedRequirement string             `json:"focused_requirement"`
}

func BuildApplicationJobSpecificationReviewPrompt(
	input ApplicationJobSpecificationReviewInput,
) (string, error) {
	if err := input.validate(); err != nil {
		return "", err
	}
	currentValue, exists := applicationJobSpecificationReviewEvidenceValue(
		input.retained, input.field, input.evidenceID,
	)
	if !exists {
		return "", fmt.Errorf("application job specification review evidence is unavailable")
	}
	evidence := []applicationJobSpecificationReviewEvidence{{
		ID: input.evidenceID, Value: currentValue,
	}}
	var priorRejectedReview *applicationJobSpecificationReviewEvidenceFailureProjection
	if input.validationFailure != nil {
		priorRejectedReview = &applicationJobSpecificationReviewEvidenceFailureProjection{
			EvidenceID: input.validationFailure.EvidenceID,
			Reason:     input.validationFailure.reason(),
		}
	}
	projection := struct {
		Authority            applicationJobSpecificationReviewAuthority                  `json:"authority"`
		CurrentField         ApplicationJobSpecificationField                            `json:"current_field"`
		FieldContract        string                                                      `json:"field_contract"`
		CurrentValue         any                                                         `json:"current_value"`
		CanRemove            bool                                                        `json:"can_remove"`
		LegalCurrentEvidence []applicationJobSpecificationReviewEvidence                 `json:"legal_current_evidence"`
		PriorRejectedReview  *applicationJobSpecificationReviewEvidenceFailureProjection `json:"prior_rejected_review,omitempty"`
	}{
		Authority: applicationJobSpecificationReviewAuthority{
			Surface:            input.authority.Surface,
			FocusedRequirement: input.authority.FocusedRequirement.SourceQuote,
		},
		CurrentField:         input.field,
		FieldContract:        applicationJobSpecificationReviewInstruction(input.field),
		CurrentValue:         currentValue,
		CanRemove:            applicationJobSpecificationReviewCanRemove(input.retained, input.field),
		LegalCurrentEvidence: evidence,
		PriorRejectedReview:  priorRejectedReview,
	}
	raw, err := json.Marshal(projection)
	if err != nil {
		return "", fmt.Errorf("encode application job specification review authority: %w", err)
	}
	prompt := strings.Join([]string{
		"Determine whether current_value satisfies field_contract. It is faithful only when every semantic claim in current_value is required by authority.focused_requirement.",
		"Return accept when current_value is faithful: {\"decision\":\"accept\",\"evidence_id\":\"\",\"replacement_value\":\"\"}.",
		"Return remove only when can_remove is true and the whole current_value is outside authority.focused_requirement: {\"decision\":\"remove\",\"evidence_id\":<the legal_current_evidence id>,\"replacement_value\":\"\"}.",
		"Otherwise return replace with one complete corrected value for current_value: {\"decision\":\"replace\",\"evidence_id\":<the legal_current_evidence id>,\"replacement_value\":<complete replacement value>}.",
		"replacement_value is the semantic leaf itself, not an instruction, plan, explanation, or patch.",
		"APPLICATION_JOB_SPECIFICATION_REVIEW_INPUT_JSON:\n" + string(raw),
	}, "\n\n")
	if len(prompt) > maxPortablePayloadBytes {
		return "", fmt.Errorf("application job specification review prompt exceeds %d bytes", maxPortablePayloadBytes)
	}
	return prompt, nil
}

func ApplicationJobSpecificationReviewResponseSchema(
	input ApplicationJobSpecificationReviewInput,
) (map[string]any, error) {
	if err := input.validate(); err != nil {
		return nil, err
	}
	decisions := []string{
		string(ApplicationJobSpecificationReviewAccept),
		string(ApplicationJobSpecificationReviewReplace),
	}
	if applicationJobSpecificationReviewCanRemove(input.retained, input.field) {
		decisions = append(decisions, string(ApplicationJobSpecificationReviewRemove))
	}
	return objectSchema(
		[]string{"decision", "evidence_id", "replacement_value"},
		map[string]any{
			"decision":    enumSchema(decisions...),
			"evidence_id": enumSchema("", input.evidenceID),
			"replacement_value": map[string]any{
				"type": "string", "minLength": 0,
				"maxLength": maxApplicationJobSpecificationReplacementRunes,
			},
		},
	), nil
}

func applicationJobSpecificationReviewInstruction(field ApplicationJobSpecificationField) string {
	switch field {
	case ApplicationJobSpecificationObjectiveField:
		return "state one concrete local product outcome faithful to the focused requirement"
	case ApplicationJobSpecificationRequiredBehaviorsField:
		return "state one concrete user action and observable result faithful to the focused requirement"
	case ApplicationJobSpecificationAcceptanceCriteriaField:
		return "state one observable check faithful to the focused requirement"
	default:
		return "reject the unsupported review target"
	}
}
