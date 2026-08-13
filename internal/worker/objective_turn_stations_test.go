package worker

import (
	"context"
	"errors"
	"testing"

	"github.com/gryph/omnidex/internal/assemblyline"
	"github.com/gryph/omnidex/internal/model"
)

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
