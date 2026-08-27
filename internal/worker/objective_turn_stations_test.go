package worker

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/gryph/omnidex/internal/assemblyline"
	"github.com/gryph/omnidex/internal/llm"
	"github.com/gryph/omnidex/internal/model"
	"github.com/gryph/omnidex/internal/queue"
	"github.com/gryph/omnidex/internal/roleplay"
	"github.com/gryph/omnidex/internal/station"
)

func TestObjectiveTextRejectsTransportPromptDisclosure(t *testing.T) {
	t.Parallel()
	if err := validateObjectiveTextTransportBoundary(
		"roleplay narrative", "The archive door opens into moonlight.",
	); err != nil {
		t.Fatal(err)
	}
	if err := validateObjectiveTextTransportBoundary(
		"roleplay narrative", "The archive door opens.\n"+llm.MinimalGeneratePrompt,
	); err == nil {
		t.Fatal("roleplay narrative exposed the private provider prompt hint")
	}
}

func TestReusableRoleplayPortableCallConsumesAcceptedLeafBeforeModelResolution(t *testing.T) {
	t.Parallel()
	input := assemblyline.ConversationResponseInput{
		Kind: assemblyline.ObjectiveKindStory, ExactInstruction: "Continue.",
		RoleplayIdentity: &assemblyline.RoleplayResponseIdentity{
			CharacterName: "Mara", Summary: "An archivist.", Voice: "Direct.",
		},
		RoleplayUserTurn: &assemblyline.RoleplayUserTurnProjection{
			PersonaKind: roleplay.UserPersonaNarrator, PersonaName: roleplay.NarratorPersonaName,
			ContributionKind: roleplay.UserContributionDirection,
		},
		Context: assemblyline.ObjectiveContext{
			Capsules: []assemblyline.ObjectiveContextCapsule{},
		},
	}
	job, err := assemblyline.NewConversationResponseJob(input)
	if err != nil {
		t.Fatal(err)
	}
	candidate, err := json.Marshal(assemblyline.ConversationResponseDecision{
		Schema: assemblyline.ConversationResponseSchemaV1,
		Text:   "Mara keeps her footing and answers.",
	})
	if err != nil {
		t.Fatal(err)
	}
	projection, err := assemblyline.NewExactPortableResultProjection(string(candidate))
	if err != nil {
		t.Fatal(err)
	}
	reuseCalls := 0
	service := &Service{reuseRoleplayResult: func(
		_ context.Context,
		request queue.RoleplayPortableResultReuseRequest,
	) (queue.RoleplayPortableResultReuse, bool, error) {
		reuseCalls++
		if request.Job.ID != job.ID || request.Station != station.ConversationResponse {
			t.Fatalf("reuse request=%+v", request)
		}
		return queue.RoleplayPortableResultReuse{Result: assemblyline.PortableResult{
			JobID: job.ID, Candidate: string(candidate), Projection: &projection,
		}}, true, nil
	}}
	runtime := &nativeRuntimeV3{svc: service, claim: &model.ClaimedStep{}}
	modelResolved := false
	decision, receipt, err := runObjectiveReusablePortableCall[assemblyline.ConversationResponseDecision](
		t.Context(), runtime, "conversation_response", job,
		station.ConversationResponse,
		func() (string, error) {
			modelResolved = true
			return "must-not-resolve", nil
		},
		func(value assemblyline.ConversationResponseDecision) error {
			return value.ValidateFor(input)
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if reuseCalls != 1 || modelResolved || receipt.Calls != 0 || !receipt.Reused ||
		decision.Text != "Mara keeps her footing and answers." {
		t.Fatalf(
			"reuse_calls=%d model_resolved=%t receipt=%+v decision=%+v",
			reuseCalls, modelResolved, receipt, decision,
		)
	}
}

func TestObjectivePortableCallUsesSuppliedAuthorityContext(t *testing.T) {
	t.Parallel()

	input := assemblyline.ConversationResponseInput{
		Kind: assemblyline.ObjectiveKindAnswer, ExactInstruction: "Answer this.",
	}
	job, err := assemblyline.NewConversationResponseJob(input)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	runtime := &nativeRuntimeV3{
		ctx: context.Background(), svc: &Service{}, claim: &model.ClaimedStep{},
	}
	workerRuntime := portableWorkerRuntimeWithContext(runtime, "objective", ctx)
	if workerRuntime.Context != ctx {
		t.Fatal("portable objective worker retained the runtime context instead of supplied authority")
	}
	_, calls, err := runObjectivePortableCall[assemblyline.ConversationResponseDecision](
		ctx, runtime, "test-model", "conversation_response", job,
		func(value assemblyline.ConversationResponseDecision) error { return value.ValidateFor(input) },
	)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("call error=%v want canceled supplied context", err)
	}
	if calls != 0 {
		t.Fatalf("canceled supplied authority dispatched %d model calls", calls)
	}
}

func TestObjectiveStationReceiptAcceptsOnlyZeroCallDurableReuse(t *testing.T) {
	t.Parallel()
	if err := validateObjectiveStationReceipt(
		"roleplay canon", objectiveStationReceipt{Reused: true},
	); err != nil {
		t.Fatal(err)
	}
	if err := validateObjectiveStationReceipt(
		"roleplay canon", objectiveStationReceipt{Calls: 1, Reused: true},
	); err == nil {
		t.Fatal("roleplay reuse fabricated a provider call")
	}
	if err := validateObjectiveStationReceipt(
		"roleplay canon", objectiveStationReceipt{},
	); err == nil {
		t.Fatal("zero-call roleplay station lacked durable reuse provenance")
	}
}
