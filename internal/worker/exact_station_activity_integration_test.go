package worker

import (
	"context"
	"os"
	"testing"

	"github.com/gryph/omnidex/internal/assemblyline"
	"github.com/gryph/omnidex/internal/llm"
	"github.com/gryph/omnidex/internal/model"
)

type observedEvidenceStation struct {
	exactEvidenceStationClient
	observed bool
}

func (client *observedEvidenceStation) GeneratePreparedExactWithActivity(ctx context.Context, prepared llm.PreparedModel, observe func(model.AgentActivity) error) (llm.PreparedGeneration, error) {
	client.observed = true
	activity := model.AgentActivity{Schema: 1, Source: model.AgentActivityNative, Kind: model.AgentActivityReasoningSummary, Status: model.AgentActivityDelta, Role: "plan", ThreadID: "actual-fixture-thread", TurnID: "actual-fixture-turn", ItemID: "actual-fixture-item", Text: "Comparing the supplied requirement."}
	if err := observe(activity); err != nil {
		return llm.PreparedGeneration{}, err
	}
	return client.exactEvidenceStationClient.GeneratePreparedExact(ctx, prepared)
}

func TestExactStationBoundPublicActivityPrecedesItsDurableSemanticResult(t *testing.T) {
	databaseURL := os.Getenv("OMNI_TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("isolated PostgreSQL qualification not selected")
	}
	_, repository := freshWorkerEvidenceRepository(t, databaseURL)
	ctx := context.Background()
	job, err := repository.EnqueueCodingJob(ctx, "classify one interface", t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	claim, err := repository.ClaimNextStep(ctx, "observed-station")
	if err != nil || claim == nil {
		t.Fatalf("claim=%v error=%v", claim, err)
	}
	provider := &observedEvidenceStation{exactEvidenceStationClient: exactEvidenceStationClient{fixtures: []exactEvidenceStationFixture{{candidate: "A"}}}}
	var observations []RuntimeEvent
	returned := false
	service := &Service{repo: repository, stationClient: provider, inferenceContextTokens: "8192", runtimeEventChannels: map[int64]runtimeEventChannelBinding{job.ID: {channelID: "station-channel", users: 1}}, runtimeEventSink: func(event RuntimeEvent) error {
		if event.AgentActivity != nil {
			if returned {
				t.Fatal("public activity arrived after the station result")
			}
			if event.JobID != claim.Authority.JobID || event.StepID != claim.Authority.StepID || event.Attempt != claim.Authority.Attempt || event.ChannelID != "station-channel" {
				t.Fatalf("public activity escaped its claimed step: %+v", event)
			}
			observations = append(observations, event)
		}
		return nil
	}}
	portable, err := assemblyline.NewApplicationClassificationJob(assemblyline.ApplicationClassificationInput{UserRequest: "classify one interface"})
	if err != nil {
		t.Fatal(err)
	}
	result, execution, err := service.executeExactPortableStation(ctx, claim.Authority, portable, "fixture-model")
	returned = true
	if err != nil {
		t.Fatal(err)
	}
	if !provider.observed || len(observations) != 1 || provider.calls != 1 || result.Candidate != "A" || execution.CallEvidenceID < 1 {
		t.Fatalf("exact station path lost public observation or changed result: observed=%v activity=%d calls=%d result=%+v execution=%+v", provider.observed, len(observations), provider.calls, result, execution)
	}
	evidence, err := repository.GetLLMCallEvidence(ctx, execution.CallEvidenceID)
	if err != nil || evidence.Candidate != "A" {
		t.Fatalf("original semantic evidence changed: candidate=%q error=%v", evidence.Candidate, err)
	}
}

func TestExactStationDisplayRejectsEffectsAndExpiredAuthority(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	count := 0
	service := &Service{runtimeEventSink: func(RuntimeEvent) error { count++; return nil }}
	observe := service.exactStationPublicActivity(ctx, model.StepAttemptAuthority{JobID: 1, StepID: 2, Attempt: 1})
	command := model.AgentActivity{Schema: 1, Source: model.AgentActivityNative, Kind: model.AgentActivityCommand, Status: model.AgentActivityStarted, Role: "plan", ThreadID: "thread", TurnID: "turn", ItemID: "item", Command: "not-dispatched", CWD: "/not-accessed"}
	if err := observe(command); err == nil {
		t.Fatal("semantic display accepted a tool effect")
	}
	cancel()
	text := command
	text.Kind, text.Status, text.Command, text.CWD, text.Text = model.AgentActivityMessage, model.AgentActivityDelta, "", "", "public text"
	if err := observe(text); err != context.Canceled {
		t.Fatalf("expired observation error=%v", err)
	}
	if count != 0 {
		t.Fatal("rejected observation was published")
	}
}
