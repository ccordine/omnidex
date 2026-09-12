package worker

import (
	"context"
	"reflect"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/assemblyline"
)

func TestRepositoryGroundedParagraphNegativesStopAtTheirOwnRelation(t *testing.T) {
	t.Parallel()
	input, accepted := repositoryGroundedSelectionFixture()
	const unrelated = "The frame is blue."
	const unsupported = "The inspection occurs Friday."
	seen := []string{}
	decision, dispatches, err := resolveRepositoryGroundedParagraphQueue(
		context.Background(), input,
		func(_ context.Context, leaf assemblyline.GroundedAnswerParagraphInventoryInput) (assemblyline.GroundedAnswerParagraphInventory, int, error) {
			seen = append(seen, "inventory")
			value, err := assemblyline.DecodeGroundedAnswerParagraphInventory(leaf, strings.Join([]string{
				accepted, unrelated, unsupported, accepted,
			}, "\n"))
			return value, 1, err
		},
		func(_ context.Context, leaf assemblyline.GroundedAnswerParagraphEvidenceRelationInput) (assemblyline.GroundedAnswerParagraphEvidenceRelationDecision, int, error) {
			seen = append(seen, "attribution:"+leaf.Evidence.ID)
			if leaf.ParagraphText != accepted {
				t.Fatalf("rejected candidate reached attribution: %q", leaf.ParagraphText)
			}
			value, err := assemblyline.DecodeGroundedAnswerParagraphEvidenceRelationDecision(leaf, "A")
			return value, 1, err
		},
		func(_ context.Context, leaf assemblyline.GroundedParagraphRelevanceInput) (assemblyline.GroundedParagraphRelevance, int, error) {
			seen = append(seen, "relevance:"+leaf.ParagraphText)
			if leaf.ExactQuestion != input.ExactRequirement || !reflect.DeepEqual(leaf.Context, assemblyline.CloneObjectiveContext(input.Context)) {
				t.Fatal("relevance lost its exact question or meaning context")
			}
			raw := "A"
			if leaf.ParagraphText == unrelated {
				raw = "B"
			}
			value, err := assemblyline.DecodeGroundedParagraphRelevance(leaf, raw)
			return value, 1, err
		},
		func(_ context.Context, leaf assemblyline.GroundedParagraphSupportInput) (assemblyline.GroundedParagraphSupport, int, error) {
			seen = append(seen, "support:"+leaf.ParagraphText)
			if leaf.ParagraphText == unrelated {
				t.Fatal("irrelevant candidate reached factual support")
			}
			if leaf.ClaimDomain != assemblyline.GroundedAllFactualClaims || !reflect.DeepEqual(leaf.Evidence, input.Evidence) {
				t.Fatal("factual support lost its bounded evidence or claim scope")
			}
			raw := "A"
			if leaf.ParagraphText == unsupported {
				raw = "B"
			}
			value, err := assemblyline.DecodeGroundedParagraphSupport(leaf, raw)
			return value, 1, err
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{
		"inventory", "relevance:" + accepted, "support:" + accepted,
		"attribution:evidence_1", "attribution:evidence_2", "attribution:evidence_3",
		"relevance:" + unrelated, "relevance:" + unsupported, "support:" + unsupported,
	}
	if !reflect.DeepEqual(seen, want) || dispatches != len(want) {
		t.Fatalf("stage order=%q calls=%d; want %q", seen, dispatches, want)
	}
	if decision.Text != accepted || !reflect.DeepEqual(decision.EvidenceIDs, []string{"evidence_1", "evidence_2", "evidence_3"}) {
		t.Fatalf("negative or duplicate candidate changed accepted state: %+v", decision)
	}
}

func TestRepositoryGroundedParagraphNoSurvivorsFailsWithoutAttribution(t *testing.T) {
	t.Parallel()
	for _, stage := range []string{"irrelevant", "unsupported", "invalid-relevance", "invalid-support"} {
		t.Run(stage, func(t *testing.T) {
			input, paragraph := repositoryGroundedSelectionFixture()
			supportCalls := 0
			decision, dispatches, err := resolveRepositoryGroundedParagraphQueue(
				context.Background(), input,
				func(_ context.Context, leaf assemblyline.GroundedAnswerParagraphInventoryInput) (assemblyline.GroundedAnswerParagraphInventory, int, error) {
					value, err := assemblyline.DecodeGroundedAnswerParagraphInventory(leaf, paragraph)
					return value, 1, err
				},
				func(_ context.Context, _ assemblyline.GroundedAnswerParagraphEvidenceRelationInput) (assemblyline.GroundedAnswerParagraphEvidenceRelationDecision, int, error) {
					t.Fatal("a rejected or invalid relation reached citation attribution")
					return assemblyline.GroundedAnswerParagraphEvidenceRelationDecision{}, 0, nil
				},
				func(_ context.Context, _ assemblyline.GroundedParagraphRelevanceInput) (assemblyline.GroundedParagraphRelevance, int, error) {
					value := assemblyline.GroundedParagraphRelevant
					if stage == "irrelevant" {
						value = assemblyline.GroundedParagraphNotRelevant
					} else if stage == "invalid-relevance" {
						value = "unknown"
					}
					return value, 1, nil
				},
				func(_ context.Context, _ assemblyline.GroundedParagraphSupportInput) (assemblyline.GroundedParagraphSupport, int, error) {
					supportCalls++
					value := assemblyline.GroundedParagraphNotFullySupported
					if stage == "invalid-support" {
						value = "unknown"
					}
					return value, 1, nil
				},
			)
			if err == nil || decision.Text != "" {
				t.Fatalf("invalid or empty survivor set returned an answer: %+v, %v", decision, err)
			}
			wantSupport := 0
			if stage == "unsupported" || stage == "invalid-support" {
				wantSupport = 1
			}
			if supportCalls != wantSupport || dispatches != 2+wantSupport {
				t.Fatalf("support calls=%d dispatches=%+v", supportCalls, dispatches)
			}
			wantError := "no responsive fully supported paragraphs"
			if strings.HasPrefix(stage, "invalid-") {
				wantError = "is not registered"
			}
			if !strings.Contains(err.Error(), wantError) {
				t.Fatalf("error=%v; want %q", err, wantError)
			}
		})
	}
}
