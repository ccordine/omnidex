package worker

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/gryph/omnidex/internal/assemblyline"
	"github.com/gryph/omnidex/internal/model"
	"github.com/gryph/omnidex/internal/queue"
	"github.com/gryph/omnidex/internal/roleplay"
)

func TestRoleplayGroundedParagraphQueueDiscardsBadCandidatesWithoutReopeningAcceptedState(
	t *testing.T,
) {
	t.Parallel()
	const (
		accepted     = "From the observatory, I'd call Earth's year about 365.25 days."
		unsupported  = "A hidden comet changes that period every Tuesday."
		unresponsive = "The observatory has polished brass railings."
	)
	input := roleplayGroundedSieveFixture()
	var kinds []assemblyline.WorkKind
	service := &Service{reuseObjectiveResult: func(
		_ context.Context,
		request queue.ObjectivePortableResultReuseRequest,
	) (queue.ObjectivePortableResultReuse, bool, error) {
		kinds = append(kinds, request.Job.Kind)
		var raw string
		switch request.Job.Kind {
		case assemblyline.WorkRoleplayGroundedResponseParagraphInventory:
			raw = accepted + "\n" + unsupported + "\n" + accepted + "\n" + unresponsive
		case assemblyline.WorkRoleplayGroundedResponseEvidenceRelation:
			var authority assemblyline.RoleplayGroundedEvidenceRelationInput
			if err := json.Unmarshal(request.Job.Payload, &authority); err != nil {
				return queue.ObjectivePortableResultReuse{}, false, err
			}
			raw = string(assemblyline.RoleplayGroundedEvidenceSupportsParagraph)
			if authority.ParagraphText == unsupported {
				raw = string(assemblyline.RoleplayGroundedEvidenceDoesNotSupport)
			}
		case assemblyline.WorkRoleplayGroundedResponseParagraphAuthorization:
			var authority assemblyline.RoleplayGroundedParagraphAuthorizationInput
			if err := json.Unmarshal(request.Job.Payload, &authority); err != nil {
				return queue.ObjectivePortableResultReuse{}, false, err
			}
			raw = string(assemblyline.RoleplayGroundedParagraphResponsiveAndSupported)
			if authority.ParagraphText == unsupported || authority.ParagraphText == unresponsive {
				raw = string(assemblyline.RoleplayGroundedParagraphNotAuthorized)
			}
		default:
			t.Fatalf("unexpected roleplay grounded work kind %q", request.Job.Kind)
		}
		projection, err := assemblyline.NewExactPortableResultProjection(raw)
		if err != nil {
			return queue.ObjectivePortableResultReuse{}, false, err
		}
		return queue.ObjectivePortableResultReuse{Result: assemblyline.PortableResult{
			JobID: request.Job.ID, Candidate: raw, Projection: &projection,
		}}, true, nil
	}}
	runtime := &nativeRuntimeV3{svc: service, claim: &model.ClaimedStep{}}
	decision, receipt, err := (portableObjectiveRoleplayGroundedStation{runtime: runtime}).RespondGrounded(
		t.Context(), input,
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(decision.Paragraphs) != 1 || decision.Paragraphs[0].Text != accepted ||
		len(decision.Paragraphs[0].EvidenceIDs) != 1 ||
		decision.Paragraphs[0].EvidenceIDs[0] != input.RealWorldEvidence[0].ID {
		t.Fatalf("accepted paragraphs=%#v", decision.Paragraphs)
	}
	if receipt.Calls != 0 || !receipt.Reused {
		t.Fatalf("receipt=%+v", receipt)
	}
	wantKinds := []assemblyline.WorkKind{
		assemblyline.WorkRoleplayGroundedResponseParagraphInventory,
		assemblyline.WorkRoleplayGroundedResponseParagraphAuthorization,
		assemblyline.WorkRoleplayGroundedResponseEvidenceRelation,
		assemblyline.WorkRoleplayGroundedResponseParagraphAuthorization,
		assemblyline.WorkRoleplayGroundedResponseParagraphAuthorization,
	}
	if len(kinds) != len(wantKinds) {
		t.Fatalf("work kinds=%v want=%v", kinds, wantKinds)
	}
	for index := range wantKinds {
		if kinds[index] != wantKinds[index] {
			t.Fatalf("work kinds=%v want=%v", kinds, wantKinds)
		}
	}
}

func roleplayGroundedSieveFixture() assemblyline.RoleplayGroundedResponseInput {
	contextText := "Ada is answering from the observatory."
	contextSource := "The current scene is the observatory."
	return assemblyline.RoleplayGroundedResponseInput{
		ExactQuestion: "What is Earth's orbital period?",
		RoleplayIdentity: assemblyline.RoleplayResponseIdentity{
			CharacterName: "Ada", Summary: "A careful astronomer.", Voice: "Measured",
		},
		RoleplayUserTurn: assemblyline.RoleplayUserTurnProjection{
			PersonaKind:      roleplay.UserPersonaNarrator,
			PersonaName:      roleplay.NarratorPersonaName,
			ContributionKind: roleplay.UserContributionCommand,
		},
		Context: assemblyline.ObjectiveContext{Capsules: []assemblyline.ObjectiveContextCapsule{{
			Sources: []assemblyline.ObjectiveContextSource{{
				Namespace: "roleplay_scene", CandidateID: "CTX_1",
				ContentSHA256: assemblyline.ExactObjectiveContextSHA(contextSource),
			}},
			Content: contextText, ContentSHA256: assemblyline.ExactObjectiveContextSHA(contextText),
		}}},
		RealWorldEvidence: []assemblyline.GroundedEvidenceCapsule{{
			ID: "doc-1", Text: "Earth's orbital period is approximately 365.25 days.",
		}},
	}
}
