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

func TestRoleplayCanonCandidateQueueSkipsRejectedAndDuplicateLeaves(t *testing.T) {
	t.Parallel()
	const (
		accepted   = "The bronze bell cracked."
		invented   = "A silver bell appeared."
		paraphrase = "The bronze bell became cracked."
	)
	input := assemblyline.RoleplayCanonExtractionInput{
		Source: assemblyline.RoleplayCanonSource{
			Kind:                  assemblyline.RoleplayCanonSourceUserContribution,
			AttributedPersonaName: roleplay.NarratorPersonaName,
			ExactContribution:     "The bronze bell cracked.",
			PersonaKind:           roleplay.UserPersonaNarrator,
			ContributionKind:      roleplay.UserContributionNarration,
		},
		Context: assemblyline.ObjectiveContext{
			Capsules: []assemblyline.ObjectiveContextCapsule{},
		},
	}
	var kinds []assemblyline.WorkKind
	service := &Service{reuseObjectiveResult: func(
		_ context.Context,
		request queue.ObjectivePortableResultReuseRequest,
	) (queue.ObjectivePortableResultReuse, bool, error) {
		kinds = append(kinds, request.Job.Kind)
		var raw string
		switch request.Job.Kind {
		case assemblyline.WorkRoleplayCanonFactInventory:
			raw = accepted + "\n" + invented + "\n" + accepted + "\n" + paraphrase
		case assemblyline.WorkRoleplayCanonFactCandidateAuthorization:
			var authority assemblyline.RoleplayCanonFactCandidateAuthorizationInput
			if err := json.Unmarshal(request.Job.Payload, &authority); err != nil {
				return queue.ObjectivePortableResultReuse{}, false, err
			}
			raw = assemblyline.RoleplayCanonFactEstablished
			if authority.Candidate == invented {
				raw = assemblyline.RoleplayCanonFactNotEstablished
			}
		case assemblyline.WorkRoleplayCanonFactCandidateRelation:
			raw = assemblyline.RoleplayCanonFactsEquivalent
		default:
			t.Fatalf("unexpected canon work kind %q", request.Job.Kind)
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
	decision, receipt, err := (portableObjectiveRoleplayCanonStation{runtime: runtime}).ExtractCanon(
		t.Context(), input,
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(decision.Facts) != 1 || decision.Facts[0] != accepted {
		t.Fatalf("accepted facts=%#v", decision.Facts)
	}
	if receipt.Calls != 0 || !receipt.Reused {
		t.Fatalf("receipt=%+v", receipt)
	}
	wantKinds := []assemblyline.WorkKind{
		assemblyline.WorkRoleplayCanonFactInventory,
		assemblyline.WorkRoleplayCanonFactCandidateAuthorization,
		assemblyline.WorkRoleplayCanonFactCandidateAuthorization,
		assemblyline.WorkRoleplayCanonFactCandidateAuthorization,
		assemblyline.WorkRoleplayCanonFactCandidateRelation,
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

func TestRoleplayCanonEmptyInventoryEndsWithoutCompletionReview(t *testing.T) {
	t.Parallel()
	input := assemblyline.RoleplayCanonExtractionInput{
		Source: assemblyline.RoleplayCanonSource{
			Kind:                  assemblyline.RoleplayCanonSourceUserContribution,
			AttributedPersonaName: roleplay.NarratorPersonaName,
			ExactContribution:     "Where is the bronze bell?",
			PersonaKind:           roleplay.UserPersonaNarrator,
			ContributionKind:      roleplay.UserContributionNarration,
		},
		Context: assemblyline.ObjectiveContext{
			Capsules: []assemblyline.ObjectiveContextCapsule{},
		},
	}
	reuseCalls := 0
	service := &Service{reuseObjectiveResult: func(
		_ context.Context,
		request queue.ObjectivePortableResultReuseRequest,
	) (queue.ObjectivePortableResultReuse, bool, error) {
		reuseCalls++
		if request.Job.Kind != assemblyline.WorkRoleplayCanonFactInventory {
			t.Fatalf("empty inventory dispatched %q", request.Job.Kind)
		}
		raw := assemblyline.RoleplayNoCanonFactCandidates
		projection, err := assemblyline.NewExactPortableResultProjection(raw)
		if err != nil {
			return queue.ObjectivePortableResultReuse{}, false, err
		}
		return queue.ObjectivePortableResultReuse{Result: assemblyline.PortableResult{
			JobID: request.Job.ID, Candidate: raw, Projection: &projection,
		}}, true, nil
	}}
	runtime := &nativeRuntimeV3{svc: service, claim: &model.ClaimedStep{}}
	decision, receipt, err := (portableObjectiveRoleplayCanonStation{runtime: runtime}).ExtractCanon(
		t.Context(), input,
	)
	if err != nil {
		t.Fatal(err)
	}
	if reuseCalls != 1 || len(decision.Facts) != 0 || receipt.Calls != 0 || !receipt.Reused {
		t.Fatalf("reuse_calls=%d decision=%+v receipt=%+v", reuseCalls, decision, receipt)
	}
}
