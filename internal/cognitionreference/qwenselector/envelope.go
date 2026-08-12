package qwenselector

import (
	"fmt"

	"github.com/gryph/omnidex/internal/cognitionreference"
	"github.com/gryph/omnidex/internal/exactjson"
)

const EnvelopeSchemaV1 = "omnidex.semantic-gap-selection.v1"

const selectionInstruction = "Select exactly one candidate_id from candidates. Return no other field."

type selectionEnvelope struct {
	Schema      string              `json:"schema"`
	Instruction string              `json:"instruction"`
	Question    string              `json:"question"`
	Evidence    []envelopeEvidence  `json:"evidence"`
	Candidates  []envelopeCandidate `json:"candidates"`
}

type envelopeEvidence struct {
	ID      cognitionreference.EvidenceID `json:"id"`
	Content string                        `json:"content"`
}

type envelopeCandidate struct {
	CandidateID cognitionreference.CandidateID  `json:"candidate_id"`
	Summary     string                          `json:"summary"`
	EvidenceIDs []cognitionreference.EvidenceID `json:"evidence_ids"`
}

func renderEnvelope(gap cognitionreference.SemanticGap) (string, error) {
	gap = gap.Clone()
	if err := gap.Validate(); err != nil {
		return "", err
	}
	envelope := selectionEnvelope{
		Schema: EnvelopeSchemaV1, Instruction: selectionInstruction, Question: gap.Question,
		Evidence:   make([]envelopeEvidence, len(gap.Evidence)),
		Candidates: make([]envelopeCandidate, len(gap.Candidates)),
	}
	for index, evidence := range gap.Evidence {
		envelope.Evidence[index] = envelopeEvidence{ID: evidence.ID, Content: evidence.Content}
	}
	for index, candidate := range gap.Candidates {
		envelope.Candidates[index] = envelopeCandidate{
			CandidateID: candidate.ID, Summary: candidate.Summary,
			EvidenceIDs: append([]cognitionreference.EvidenceID{}, candidate.EvidenceIDs...),
		}
	}
	raw, err := exactjson.Canonical(envelope)
	if err != nil {
		return "", fmt.Errorf("render exact semantic gap envelope: %w", err)
	}
	return string(raw), nil
}

func responseSchema(gap cognitionreference.SemanticGap) map[string]any {
	candidates := make([]any, len(gap.Candidates))
	for index := range gap.Candidates {
		candidates[index] = string(gap.Candidates[index].ID)
	}
	return map[string]any{
		"type":                 "object",
		"additionalProperties": false,
		"required":             []any{"candidate_id"},
		"properties": map[string]any{
			"candidate_id": map[string]any{"type": "string", "enum": candidates},
		},
	}
}
