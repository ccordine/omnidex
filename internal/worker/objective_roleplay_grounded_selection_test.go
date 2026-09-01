package worker

import (
	"context"
	"reflect"
	"testing"

	"github.com/gryph/omnidex/internal/assemblyline"
	"github.com/gryph/omnidex/internal/roleplay"
)

func TestRoleplayGroundedEvidenceIsAggregatedFromIndependentBinaryLeaves(t *testing.T) {
	t.Parallel()
	input, paragraph := roleplayGroundedSelectionFixture()
	seenEvidence := make([]string, 0, len(input.RealWorldEvidence))
	evidenceIDs, supporting, receipt, err := resolveRoleplayGroundedEvidenceRelations(
		context.Background(),
		input,
		paragraph,
		func(
			_ context.Context,
			leafInput assemblyline.RoleplayGroundedEvidenceRelationInput,
		) (assemblyline.RoleplayGroundedEvidenceRelation, objectiveStationReceipt, error) {
			seenEvidence = append(seenEvidence, leafInput.Evidence.ID)
			raw := map[string]string{
				"evidence_1": "A",
				"evidence_2": "B",
				"evidence_3": "A",
			}[leafInput.Evidence.ID]
			value, err := assemblyline.DecodeRoleplayGroundedResponseEvidenceRelationLeaf(
				leafInput, raw,
			)
			return value, objectiveStationReceipt{Calls: 1}, err
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"evidence_1", "evidence_2", "evidence_3"}; !reflect.DeepEqual(seenEvidence, want) {
		t.Fatalf("binary evidence leaf order=%v, want %v", seenEvidence, want)
	}
	if want := []string{"evidence_1", "evidence_3"}; !reflect.DeepEqual(evidenceIDs, want) {
		t.Fatalf("code-owned evidence IDs=%v, want %v", evidenceIDs, want)
	}
	wantSupporting := []assemblyline.GroundedEvidenceCapsule{
		input.RealWorldEvidence[0], input.RealWorldEvidence[2],
	}
	if !reflect.DeepEqual(supporting, wantSupporting) {
		t.Fatalf("supporting evidence=%v, want %v", supporting, wantSupporting)
	}
	if receipt.Calls != 3 || receipt.Reused {
		t.Fatalf("receipt=%+v", receipt)
	}
	decision, err := assemblyline.AssembleRoleplayGroundedResponseDecision(
		input,
		[]assemblyline.RoleplayGroundedParagraph{{
			Text: paragraph, EvidenceIDs: evidenceIDs,
		}},
	)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(decision.Paragraphs[0].EvidenceIDs, evidenceIDs) {
		t.Fatalf("assembled evidence IDs=%v", decision.Paragraphs[0].EvidenceIDs)
	}
}

func TestRoleplayGroundedEvidenceRejectsAggregateModelPackets(t *testing.T) {
	t.Parallel()
	for _, raw := range []string{
		`["evidence_1","evidence_3"]`,
		`{"evidence_ids":["evidence_1","evidence_3"]}`,
	} {
		raw := raw
		t.Run(raw, func(t *testing.T) {
			input, paragraph := roleplayGroundedSelectionFixture()
			calls := 0
			evidenceIDs, supporting, receipt, err := resolveRoleplayGroundedEvidenceRelations(
				context.Background(),
				input,
				paragraph,
				func(
					_ context.Context,
					leafInput assemblyline.RoleplayGroundedEvidenceRelationInput,
				) (assemblyline.RoleplayGroundedEvidenceRelation, objectiveStationReceipt, error) {
					calls++
					value, err := assemblyline.DecodeRoleplayGroundedResponseEvidenceRelationLeaf(
						leafInput, raw,
					)
					return value, objectiveStationReceipt{Calls: 1}, err
				},
			)
			if err == nil {
				t.Fatal("aggregate model response was decoded as roleplay evidence selection")
			}
			if calls != 1 || receipt.Calls != 1 || evidenceIDs != nil || supporting != nil {
				t.Fatalf(
					"calls=%d receipt=%+v IDs=%v supporting=%v",
					calls, receipt, evidenceIDs, supporting,
				)
			}
		})
	}
}

func roleplayGroundedSelectionFixture() (assemblyline.RoleplayGroundedResponseInput, string) {
	return assemblyline.RoleplayGroundedResponseInput{
		ExactQuestion: "When does the eclipse begin?",
		RoleplayIdentity: assemblyline.RoleplayResponseIdentity{
			CharacterName: "Mara",
			Summary:       "A careful astronomer who speaks plainly.",
		},
		RoleplayUserTurn: assemblyline.RoleplayUserTurnProjection{
			PersonaKind:      roleplay.UserPersonaCharacter,
			PersonaName:      "Ilan",
			ContributionKind: roleplay.UserContributionDialogue,
		},
		Context: assemblyline.ObjectiveContext{Capsules: []assemblyline.ObjectiveContextCapsule{}},
		RealWorldEvidence: []assemblyline.GroundedEvidenceCapsule{
			{ID: "evidence_1", Text: "The almanac records the eclipse at midnight."},
			{ID: "evidence_2", Text: "The observatory walls are white."},
			{ID: "evidence_3", Text: "The published schedule confirms a midnight start."},
		},
		KnownArtifactPaths: []string{},
	}, "The eclipse begins at midnight."
}
