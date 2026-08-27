package assemblyline

import (
	"fmt"
	"strings"
)

const (
	CapabilityRelationSchemaV1 = "omnidex.capability-relation.v1"
	maxCapabilityRelationNeed  = 2000
)

type CapabilityRelation string

const (
	CapabilityIndependent    CapabilityRelation = "independent"
	CapabilityLeftReadsRight CapabilityRelation = "left_reads_right"
	CapabilityRightReadsLeft CapabilityRelation = "right_reads_left"
)

type CapabilityRelationInput struct {
	LocalContext string `json:"local_context"`
	LeftNeed     string `json:"left_need"`
	RightNeed    string `json:"right_need"`
}

type CapabilityRelationDecision struct {
	Schema   string             `json:"schema"`
	Relation CapabilityRelation `json:"relation"`
}

func (input CapabilityRelationInput) validate() error {
	for label, value := range map[string]string{
		"local context": input.LocalContext,
		"left need":     input.LeftNeed,
		"right need":    input.RightNeed,
	} {
		if value == "" || value != strings.TrimSpace(value) {
			return fmt.Errorf("capability relation requires one trimmed %s", label)
		}
	}
	if len(input.LocalContext) > maxSkillLocalContext ||
		len(input.LeftNeed) > maxCapabilityRelationNeed || len(input.RightNeed) > maxCapabilityRelationNeed {
		return fmt.Errorf("capability relation input exceeds its hard context limit")
	}
	if input.LeftNeed == input.RightNeed {
		return fmt.Errorf("capability relation requires two distinct local needs")
	}
	return nil
}

func (decision CapabilityRelationDecision) ValidateFor(input CapabilityRelationInput) error {
	if err := input.validate(); err != nil {
		return err
	}
	if decision.Schema != CapabilityRelationSchemaV1 {
		return fmt.Errorf("capability relation schema must be %q", CapabilityRelationSchemaV1)
	}
	switch decision.Relation {
	case CapabilityIndependent, CapabilityLeftReadsRight,
		CapabilityRightReadsLeft:
		return nil
	default:
		return fmt.Errorf("capability relation %q is unsupported", decision.Relation)
	}
}

func BuildCapabilityRelationPrompt(input CapabilityRelationInput) (string, error) {
	if err := input.validate(); err != nil {
		return "", err
	}
	return strings.Join([]string{
		"Classify only the direct live-state dependency between two local behaviors.",
		"Return independent when neither behavior must consume a result uniquely produced by the other. Shared request or user input, related subject matter, validation overlap, visual proximity, possible reuse, or convenience does not create an edge.",
		"Return left_reads_right only when LEFT_NEED cannot satisfy its named behavior without consuming a result uniquely produced by RIGHT_NEED.",
		"Return right_reads_left only when RIGHT_NEED cannot satisfy its named behavior without consuming a result uniquely produced by LEFT_NEED.",
		"When the two phrases do not establish one necessary unique producer result, return independent.",
		"LOCAL_CONTEXT:\n" + input.LocalContext,
		"LEFT_NEED:\n" + input.LeftNeed,
		"RIGHT_NEED:\n" + input.RightNeed,
	}, "\n\n"), nil
}

func CapabilityRelationResponseSchema() map[string]any {
	return objectSchema(
		[]string{"schema", "relation"},
		map[string]any{
			"schema": map[string]any{"type": "string", "const": CapabilityRelationSchemaV1},
			"relation": enumSchema(
				CapabilityIndependent, CapabilityLeftReadsRight,
				CapabilityRightReadsLeft,
			),
		},
	)
}
