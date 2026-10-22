package worker

import (
	"context"
	"reflect"
	"testing"

	"github.com/gryph/omnidex/internal/llm"
	"github.com/gryph/omnidex/internal/model"
)

type stationActivityProbe struct {
	plainCalls, observedCalls int
	ctx                       context.Context
	prepared                  llm.PreparedModel
}

func (probe *stationActivityProbe) GeneratePreparedExact(ctx context.Context, prepared llm.PreparedModel) (llm.PreparedGeneration, error) {
	probe.plainCalls++
	probe.ctx, probe.prepared = ctx, prepared
	return llm.PreparedGeneration{Content: "exact semantic result"}, nil
}

func (probe *stationActivityProbe) GeneratePreparedExactWithActivity(ctx context.Context, prepared llm.PreparedModel, observe func(model.AgentActivity) error) (llm.PreparedGeneration, error) {
	probe.observedCalls++
	probe.ctx, probe.prepared = ctx, prepared
	if err := observe(model.AgentActivity{Schema: 1, Source: model.AgentActivityNative, Kind: model.AgentActivityMessage, Status: model.AgentActivityDelta, Role: "plan", ThreadID: "thread", TurnID: "turn", ItemID: "item", Text: "Reading the supplied requirement."}); err != nil {
		return llm.PreparedGeneration{}, err
	}
	return llm.PreparedGeneration{Content: "exact semantic result"}, nil
}

func TestExactStationPublishesPublicActivityBeforeReturningItsUnchangedResult(t *testing.T) {
	probe := &stationActivityProbe{}
	prepared := llm.PreparedModel{BaseModel: "selected-model", Prompt: "one narrow semantic question", ContextTokens: 8192, MaxOutputTokens: 128}
	observations := 0
	result, err := generatePreparedExactWithActivity(context.Background(), probe, prepared, func(activity model.AgentActivity) error {
		observations++
		return activity.Validate()
	})
	if err != nil {
		t.Fatal(err)
	}
	if probe.observedCalls != 1 || probe.plainCalls != 0 || observations != 1 {
		t.Fatalf("station dropped public activity: observed calls=%d plain calls=%d observations=%d", probe.observedCalls, probe.plainCalls, observations)
	}
	if !reflect.DeepEqual(prepared, probe.prepared) || result.Content != "exact semantic result" {
		t.Fatal("display changed the exact request or semantic result")
	}
	if _, ok := probe.ctx.Deadline(); ok {
		t.Fatal("observed request gained an artificial deadline")
	}
	if probe.ctx.Err() != context.Canceled {
		t.Fatal("observed provider context remained open after return")
	}
}

func TestExactStationWithoutDisplayUsesItsOriginalProviderEntryPoint(t *testing.T) {
	probe := &stationActivityProbe{}
	if _, err := generatePreparedExact(context.Background(), probe, llm.PreparedModel{}); err != nil {
		t.Fatal(err)
	}
	if probe.plainCalls != 1 || probe.observedCalls != 0 {
		t.Fatal("absent observer changed provider dispatch")
	}
}
