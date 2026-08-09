package queue

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/gryph/omnidex/internal/model"
)

func TestScrumChannelOperationRequiresExplicitTypedIdentity(t *testing.T) {
	_, err := describeScrumChannelOperation(ScrumChannelOperationRequest{
		ProjectID: 4, CardID: "card-4", Message: "Continue.",
	})
	if err == nil {
		t.Fatal("missing lifecycle operation identity must fail")
	}
}

func TestScrumChannelOperationRejectsUnregisteredOrMixedEffects(t *testing.T) {
	operationID, err := NewLifecycleOperationID("scrum-channel-types")
	if err != nil {
		t.Fatal(err)
	}
	base := ScrumChannelOperationCommand{
		Request: ScrumChannelOperationRequest{
			OperationID: operationID, ProjectID: 4, CardID: "card-4", Message: "Continue.",
		},
		ExpectedCardUpdatedAt: time.Now(), ResultAgent: "omnidex", ResultAction: "started",
		Effect: ScrumChannelEffect{
			Kind: ScrumChannelStartJob, Instruction: "Continue.", Pipeline: model.PipelineScrum,
			Metadata: json.RawMessage(`{"project_id":4}`),
		},
	}
	if _, _, err := normalizeScrumChannelOperation(base); err != nil {
		t.Fatalf("valid start operation: %v", err)
	}
	mixed := base
	mixed.Effect.Kind = ScrumChannelReplanJob
	mixed.Effect.JobID = 19
	if _, _, err := normalizeScrumChannelOperation(mixed); err == nil {
		t.Fatal("replan effect with start-job fields must fail")
	}
	unregistered := base
	unregistered.Effect.Kind = "unknown"
	if _, _, err := normalizeScrumChannelOperation(unregistered); err == nil {
		t.Fatal("unregistered effect must fail")
	}
}
