package assemblyline

import (
	"encoding/json"
	"fmt"
	"strings"
)

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
		evidenceID := input.validationFailure.EvidenceID
		if evidenceID == "" && input.validationFailure.FindingEvidence != "" {
			evidenceID, _ = applicationJobSpecificationReviewEvidenceID(
				input.retained, input.field, input.validationFailure.FindingEvidence,
			)
		}
		priorRejectedReview = &applicationJobSpecificationReviewEvidenceFailureProjection{
			EvidenceID:      evidenceID,
			RejectedFinding: input.validationFailure.Finding,
			Reason:          input.validationFailure.reason(),
		}
	}
	projection := struct {
		Authority            applicationJobSpecificationRepairAuthority                  `json:"authority"`
		CurrentField         ApplicationJobSpecificationField                            `json:"current_field"`
		FieldContract        string                                                      `json:"field_contract"`
		CurrentValue         any                                                         `json:"current_value"`
		LegalCurrentEvidence []applicationJobSpecificationReviewEvidence                 `json:"legal_current_evidence"`
		PriorRejectedReview  *applicationJobSpecificationReviewEvidenceFailureProjection `json:"prior_rejected_review,omitempty"`
	}{
		Authority: applicationJobSpecificationRepairAuthority{
			Surface:            input.authority.Surface,
			FocusedRequirement: input.authority.FocusedRequirement.SourceQuote,
		},
		CurrentField:         input.field,
		FieldContract:        applicationJobSpecificationRepairInstruction(input.field),
		CurrentValue:         currentValue,
		LegalCurrentEvidence: evidence,
		PriorRejectedReview:  priorRejectedReview,
	}
	raw, err := json.Marshal(projection)
	if err != nil {
		return "", fmt.Errorf("encode application job specification review authority: %w", err)
	}
	prompt := strings.Join([]string{
		"Determine whether current_value satisfies field_contract. It is faithful only when every semantic claim in current_value is required by authority.focused_requirement.",
		"Return {\"decision\":\"accept\",\"evidence_id\":\"\",\"finding\":\"\"} or {\"decision\":\"repair\",\"evidence_id\":<the legal_current_evidence id>,\"finding\":<the exact semantic problem>}.",
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
	return objectSchema(
		[]string{"decision", "evidence_id", "finding"},
		map[string]any{
			"decision": enumSchema(
				string(ApplicationJobSpecificationReviewAccept),
				string(ApplicationJobSpecificationReviewRepair),
			),
			"evidence_id": enumSchema("", input.evidenceID),
			"finding": map[string]any{
				"type": "string", "minLength": 0,
				"maxLength": maxApplicationJobSpecificationReviewFindingRunes,
			},
		},
	), nil
}
