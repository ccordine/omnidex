package worker

import (
	"context"
	"errors"
	"testing"

	"github.com/gryph/omnidex/internal/assemblyline"
	"github.com/gryph/omnidex/internal/llm"
	"github.com/gryph/omnidex/internal/model"
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
