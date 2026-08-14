package objectiveadvisory

import (
	"encoding/json"
	"fmt"
	"strings"
)

const (
	ProjectionSchemaV1     = "omnidex.objective-advisory-projection.v1"
	conciseFinalAdvisoryV1 = "Return exactly one concise plain-text paragraph. Prioritize only the highest-value considerations and do not repeat points or provide multiple drafts. On the next line after that paragraph, emit exactly <END_OBJECTIVE_ADVISORY_V1> and nothing else."
)

type projectionEnvelope struct {
	Schema         string          `json:"schema"`
	TriggerID      string          `json:"trigger_id"`
	TriggerVersion string          `json:"trigger_version"`
	Input          ProjectionInput `json:"input"`
}

func BuildProjection(input ProjectionInput) (Projection, error) {
	if err := validateProjectionInput(input); err != nil {
		return Projection{}, err
	}
	raw, err := json.Marshal(projectionEnvelope{
		Schema: ProjectionSchemaV1, TriggerID: TriggerPostGroundingObjective,
		TriggerVersion: TriggerVersionV1, Input: cloneProjectionInput(input),
	})
	if err != nil {
		return Projection{}, fmt.Errorf("encode grounded objective advisory projection: %w", err)
	}
	if len(raw) > MaxProjectionBytes {
		return Projection{}, fmt.Errorf("objective advisory projection is %d bytes and exceeds %d", len(raw), MaxProjectionBytes)
	}
	hash := digest(string(raw))
	projection := Projection{
		Schema: ProjectionSchemaV1, ID: "objective-advisory-projection:" + hash,
		TriggerID: TriggerPostGroundingObjective, TriggerVersion: TriggerVersionV1,
		Input: cloneProjectionInput(input), Rendered: string(raw), RenderedSHA256: hash,
		RenderedBytes: len(raw), EstimatedTokens: (len(raw) + 3) / 4,
	}
	if err := projection.Validate(); err != nil {
		return Projection{}, err
	}
	return projection, nil
}

func (projection Projection) Validate() error {
	if projection.Schema != ProjectionSchemaV1 ||
		projection.TriggerID != TriggerPostGroundingObjective || projection.TriggerVersion != TriggerVersionV1 ||
		projection.ID != "objective-advisory-projection:"+projection.RenderedSHA256 ||
		!validSHA256(projection.RenderedSHA256) || projection.RenderedBytes != len([]byte(projection.Rendered)) ||
		projection.EstimatedTokens != (projection.RenderedBytes+3)/4 || projection.RenderedSHA256 != digest(projection.Rendered) {
		return fmt.Errorf("objective advisory projection identity or accounting is inconsistent")
	}
	if err := validateProjectionInput(projection.Input); err != nil {
		return err
	}
	want, err := json.Marshal(projectionEnvelope{
		Schema: projection.Schema, TriggerID: projection.TriggerID,
		TriggerVersion: projection.TriggerVersion, Input: projection.Input,
	})
	if err != nil || string(want) != projection.Rendered {
		return fmt.Errorf("objective advisory rendered projection is not exact canonical input")
	}
	return nil
}

func BuildPrompt(request GenerateRequest) (string, error) {
	if request.TriggerID != TriggerPostGroundingObjective || request.TriggerVersion != TriggerVersionV1 {
		return "", fmt.Errorf("objective advisory request uses an unregistered trigger")
	}
	if err := request.Projection.Validate(); err != nil {
		return "", err
	}
	if err := request.Source.validate(); err != nil {
		return "", err
	}
	prompt := strings.Join([]string{
		"Review the objective and established evidence below.",
		"Identify potentially useful implications, risks, edge cases, ambiguities, alternative interpretations, hidden constraints, verification ideas, or questions that subsequent work should keep in mind.",
		"Do not issue commands. Do not claim authority. Do not assume unsupported facts. Plain text is expected.",
		conciseFinalAdvisoryV1,
		"GROUNDED_OBJECTIVE_PROJECTION_JSON:\n" + request.Projection.Rendered,
	}, "\n\n")
	if len([]byte(prompt)) > request.Source.Budget.MaxInputBytes {
		return "", fmt.Errorf("objective advisory prompt exceeds its configured input budget")
	}
	return prompt, nil
}

func cloneProjectionInput(input ProjectionInput) ProjectionInput {
	input.UserAuthorities = append([]TextAuthority{}, input.UserAuthorities...)
	input.Constraints = append([]string{}, input.Constraints...)
	input.GroundedEvidence = append([]EvidenceSummary{}, input.GroundedEvidence...)
	input.Decisions = append([]string{}, input.Decisions...)
	input.Invariants = append([]string{}, input.Invariants...)
	input.UnresolvedQuestions = append([]string{}, input.UnresolvedQuestions...)
	return input
}
