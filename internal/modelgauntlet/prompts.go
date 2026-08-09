package modelgauntlet

import (
	"fmt"
	"strings"

	"github.com/gryph/omnidex/internal/assemblyline"
)

const deliberationLensSchemaV1 = "omnidex.deliberation-lens.v1"

type deliberationLens string

const (
	lensStateFlow        deliberationLens = "state_flow"
	lensTemporalOrder    deliberationLens = "temporal_order"
	lensMutualConstraint deliberationLens = "mutual_constraint"
	lensIsolation        deliberationLens = "isolation"
)

type deliberationLensDecision struct {
	Schema string           `json:"schema"`
	Lens   deliberationLens `json:"lens"`
}

type rawDeliberation struct {
	Content string `json:"content"`
}

func (deliberationLens) advisoryBriefing() {}

type capabilityRelationAdvisoryStation struct {
	input assemblyline.CapabilityRelationInput
}

func (capabilityRelationAdvisoryStation) workKind() assemblyline.WorkKind {
	return assemblyline.WorkCapabilityRelation
}

func (station capabilityRelationAdvisoryStation) buildBriefingPrompt(authoritativePrompt string) (string, error) {
	if _, err := assemblyline.BuildCapabilityRelationPrompt(station.input); err != nil {
		return "", err
	}
	return buildLensSelectionPrompt(authoritativePrompt), nil
}

func (capabilityRelationAdvisoryStation) briefingResponseSchema() map[string]any {
	return lensSelectionResponseSchema()
}

func (capabilityRelationAdvisoryStation) decodeBriefing(raw string) (advisoryBriefing, error) {
	return decodeLens(raw)
}

func (station capabilityRelationAdvisoryStation) buildDeliberationPrompt(
	authoritativePrompt string,
	briefing advisoryBriefing,
) (string, error) {
	lens, okay := briefing.(deliberationLens)
	if !okay {
		return "", fmt.Errorf("capability relation briefing has type %T, expected deliberationLens", briefing)
	}
	if _, err := assemblyline.BuildCapabilityRelationPrompt(station.input); err != nil {
		return "", err
	}
	return buildDeliberationPrompt(authoritativePrompt, lens)
}

func (capabilityRelationAdvisoryStation) synthesisInstruction() string {
	return "Classify the relation under the original capability-relation contract below."
}

func (station capabilityRelationAdvisoryStation) validateCandidate(raw string) error {
	_, err := decodeRelation(raw, station.input)
	return err
}

func buildLensSelectionPrompt(authoritativePrompt string) string {
	return strings.Join([]string{
		"Select exactly one code-registered analysis lens for a separate reasoner.",
		"The lens selects how to inspect the relation; it does not decide the relation.",
		"state_flow: trace which behavior produces live state read by the other.",
		"temporal_order: inspect whether one behavior must occur before the other.",
		"mutual_constraint: inspect whether both behaviors continuously constrain each other.",
		"isolation: test whether each behavior can be implemented without current state from the other.",
		authoritativePrompt,
	}, "\n\n")
}

func lensSelectionResponseSchema() map[string]any {
	return map[string]any{
		"type":                 "object",
		"additionalProperties": false,
		"required":             []string{"schema", "lens"},
		"properties": map[string]any{
			"schema": map[string]any{"type": "string", "const": deliberationLensSchemaV1},
			"lens": map[string]any{"type": "string", "enum": []deliberationLens{
				lensStateFlow, lensTemporalOrder, lensMutualConstraint, lensIsolation,
			}},
		},
	}
}

func buildDeliberationPrompt(authoritativePrompt string, lens deliberationLens) (string, error) {
	instruction, err := lensInstruction(lens)
	if err != nil {
		return "", err
	}
	return strings.Join([]string{
		"Analyze the two supplied local behaviors using only the selected code-owned lens.",
		"Return a concise advisory memo in plain text. Do not emit JSON and do not make an authoritative final classification.",
		"SELECTED_LENS:\n" + string(lens),
		"LENS_INSTRUCTION:\n" + instruction,
		authoritativePrompt,
	}, "\n\n"), nil
}

func lensInstruction(lens deliberationLens) (string, error) {
	switch lens {
	case lensStateFlow:
		return "Identify state producers and current-state readers in each direction.", nil
	case lensTemporalOrder:
		return "Test each possible ordering and distinguish sequencing from a live-state dependency.", nil
	case lensMutualConstraint:
		return "Test whether changes on either side immediately constrain valid state on the other.", nil
	case lensIsolation:
		return "Try to implement each behavior independently and identify any current data that makes isolation impossible.", nil
	default:
		return "", fmt.Errorf("deliberation lens %q is unsupported", lens)
	}
}
