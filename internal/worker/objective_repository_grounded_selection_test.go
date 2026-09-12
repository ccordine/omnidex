package worker

import (
	"context"
	"reflect"
	"testing"

	"github.com/gryph/omnidex/internal/assemblyline"
)

func TestRepositoryGroundedEvidenceIsAggregatedFromIndependentBinaryLeaves(t *testing.T) {
	t.Parallel()
	input, paragraph := repositoryGroundedSelectionFixture()
	seenEvidence := make([]string, 0, len(input.Evidence))
	decision, dispatches, err := resolveRepositoryGroundedParagraphQueue(
		context.Background(),
		input,
		func(
			_ context.Context,
			leafInput assemblyline.GroundedAnswerParagraphInventoryInput,
		) (assemblyline.GroundedAnswerParagraphInventory, int, error) {
			value, err := assemblyline.DecodeGroundedAnswerParagraphInventory(leafInput, paragraph)
			return value, 1, err
		},
		func(
			_ context.Context,
			leafInput assemblyline.GroundedAnswerParagraphEvidenceRelationInput,
		) (assemblyline.GroundedAnswerParagraphEvidenceRelationDecision, int, error) {
			seenEvidence = append(seenEvidence, leafInput.Evidence.ID)
			raw := map[string]string{
				"evidence_1": "A",
				"evidence_2": "B",
				"evidence_3": "A",
			}[leafInput.Evidence.ID]
			value, err := assemblyline.DecodeGroundedAnswerParagraphEvidenceRelationDecision(
				leafInput, raw,
			)
			return value, 1, err
		},
		fixtureGroundedParagraphRelevance,
		fixtureGroundedParagraphFullSupport,
	)
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"evidence_1", "evidence_2", "evidence_3"}; !reflect.DeepEqual(seenEvidence, want) {
		t.Fatalf("binary evidence leaf order=%v, want %v", seenEvidence, want)
	}
	if want := []string{"evidence_1", "evidence_3"}; !reflect.DeepEqual(decision.EvidenceIDs, want) {
		t.Fatalf("code-owned evidence IDs=%v, want %v", decision.EvidenceIDs, want)
	}
	if decision.Text != paragraph || dispatches != 6 {
		t.Fatalf("decision=%+v dispatches=%+v", decision, dispatches)
	}
}

func TestRepositoryGroundedEvidenceRejectsAggregateModelPackets(t *testing.T) {
	t.Parallel()
	for _, raw := range []string{
		`["evidence_1","evidence_3"]`,
		`{"evidence_ids":["evidence_1","evidence_3"]}`,
	} {
		raw := raw
		t.Run(raw, func(t *testing.T) {
			input, paragraph := repositoryGroundedSelectionFixture()
			supportCalls := 0
			_, dispatches, err := resolveRepositoryGroundedParagraphQueue(
				context.Background(),
				input,
				func(
					_ context.Context,
					leafInput assemblyline.GroundedAnswerParagraphInventoryInput,
				) (assemblyline.GroundedAnswerParagraphInventory, int, error) {
					value, err := assemblyline.DecodeGroundedAnswerParagraphInventory(leafInput, paragraph)
					return value, 1, err
				},
				func(
					_ context.Context,
					leafInput assemblyline.GroundedAnswerParagraphEvidenceRelationInput,
				) (assemblyline.GroundedAnswerParagraphEvidenceRelationDecision, int, error) {
					supportCalls++
					value, err := assemblyline.DecodeGroundedAnswerParagraphEvidenceRelationDecision(
						leafInput, raw,
					)
					return value, 1, err
				},
				fixtureGroundedParagraphRelevance,
				fixtureGroundedParagraphFullSupport,
			)
			if err == nil {
				t.Fatal("aggregate model response was decoded as repository evidence selection")
			}
			if supportCalls != 1 || dispatches != 4 {
				t.Fatalf("support calls=%d dispatches=%+v", supportCalls, dispatches)
			}
		})
	}
}

func repositoryGroundedSelectionFixture() (assemblyline.GroundedAnswerInput, string) {
	return assemblyline.GroundedAnswerInput{
		RequirementID:    "requirement_1",
		ExactRequirement: "When does the inspection occur?",
		Context:          assemblyline.ObjectiveContext{Capsules: []assemblyline.ObjectiveContextCapsule{}},
		Evidence: []assemblyline.GroundedEvidenceCapsule{
			{ID: "evidence_1", Text: "The inspection schedule lists Monday."},
			{ID: "evidence_2", Text: "The frame color is blue."},
			{ID: "evidence_3", Text: "The calendar confirms the inspection on Monday."},
		},
		KnownArtifactPaths: []string{},
	}, "The inspection occurs Monday."
}

func fixtureGroundedParagraphRelevance(
	_ context.Context, input assemblyline.GroundedParagraphRelevanceInput,
) (assemblyline.GroundedParagraphRelevance, int, error) {
	value, err := assemblyline.DecodeGroundedParagraphRelevance(input, "A")
	return value, 1, err
}

func fixtureGroundedParagraphFullSupport(
	_ context.Context, input assemblyline.GroundedParagraphSupportInput,
) (assemblyline.GroundedParagraphSupport, int, error) {
	value, err := assemblyline.DecodeGroundedParagraphSupport(input, "A")
	return value, 1, err
}
