package worker

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/assemblyline"
	"github.com/gryph/omnidex/internal/llm"
	"github.com/gryph/omnidex/internal/model"
	"github.com/gryph/omnidex/internal/station"
)

func TestObjectiveSoleChoiceSkipsModelResolutionAndExecution(t *testing.T) {
	t.Parallel()
	input := soleJoinPathInput()
	job, err := assemblyline.NewDatabaseJoinPathSelectionJob(input)
	if err != nil {
		t.Fatal(err)
	}
	modelResolutions := 0
	decision, receipt, err := runObjectivePortableRawLeafStation(
		context.Background(), nil, "sole_join_path", job, station.DatabaseJoinPathSelection,
		func() (string, error) {
			modelResolutions++
			return "should-not-resolve", nil
		},
		func(raw string) (assemblyline.DatabaseJoinPathSelectionDecision, error) {
			return assemblyline.DecodeDatabaseJoinPathSelectionDecision(input, raw)
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if modelResolutions != 0 || receipt.Calls != 0 {
		t.Fatalf("model resolutions=%d provider calls=%d", modelResolutions, receipt.Calls)
	}
	if decision.PathID != "customer-orders" {
		t.Fatalf("selected path = %q", decision.PathID)
	}
}

func TestExactStationBoundaryConsumesSoleChoiceWithoutProviderAuthority(t *testing.T) {
	t.Parallel()
	input := soleJoinPathInput()
	job, err := assemblyline.NewDatabaseJoinPathSelectionJob(input)
	if err != nil {
		t.Fatal(err)
	}
	result, execution, err := (&Service{}).executeExactPortableStation(
		context.Background(), model.StepAttemptAuthority{}, job, "",
	)
	if err != nil {
		t.Fatal(err)
	}
	if result.Candidate != "A" || execution.Candidate != "A" ||
		execution.ProviderCalls != 0 || execution.CallEvidenceID != 0 || !execution.InferenceFree {
		t.Fatalf("result=%#v execution=%#v", result, execution)
	}
}

func TestPortableWorkerFinalizesSoleChoiceWithoutProviderAuthority(t *testing.T) {
	t.Parallel()
	input := soleJoinPathInput()
	job, err := assemblyline.NewDatabaseJoinPathSelectionJob(input)
	if err != nil {
		t.Fatal(err)
	}
	provider := &deterministicFinalizationProviderProbe{}
	native := &nativeRuntimeV3{
		svc: &Service{stationClient: provider},
		ctx: context.Background(),
		claim: &model.ClaimedStep{Authority: model.StepAttemptAuthority{
			JobID: 1, Generation: 1, StepID: 2, Attempt: 1, WorkerID: "worker",
		}},
	}
	runtime := portableWorkerRuntime(native, "deterministic-choice")
	decision, err := runDirectCodingSemanticLeafCall(
		runtime, "", "sole_join_path", job, nil,
		func(raw string) (assemblyline.DatabaseJoinPathSelectionDecision, error) {
			return assemblyline.DecodeDatabaseJoinPathSelectionDecision(input, raw)
		},
	)
	if err != nil {
		t.Fatalf("resolve and finalize deterministic portable choice: %v", err)
	}
	if decision.PathID != "customer-orders" {
		t.Fatalf("selected path = %q", decision.PathID)
	}
	if provider.calls != 0 || runtime.ProviderCalls() != 0 {
		t.Fatalf("provider calls=%d runtime calls=%d", provider.calls, runtime.ProviderCalls())
	}
	if err := runtime.Release(job); err != nil {
		t.Fatalf("release finalized deterministic portable choice: %v", err)
	}
}

func TestZeroCallProviderReplayIsNotClassifiedAsInferenceFree(t *testing.T) {
	t.Parallel()
	input := assemblyline.ApplicationClassificationInput{UserRequest: "classify one interface"}
	job, err := assemblyline.NewApplicationClassificationJob(input)
	if err != nil {
		t.Fatal(err)
	}
	projection, err := assemblyline.NewExactPortableResultProjection("A")
	if err != nil {
		t.Fatal(err)
	}
	result := assemblyline.PortableResult{
		JobID: job.ID, Candidate: "A", Projection: &projection,
	}
	execution := exactStationExecution{
		CallEvidenceID: 9, WorkID: job.ID, WorkKind: job.Kind,
		Model: "provider-model", Iteration: 1, Candidate: "A",
		CandidateResponseSHA256: projection.SourceResponseSHA256,
		Replayed:                true,
	}
	handled, err := finalizeInferenceFreePortableResult(job, result, execution)
	if err != nil || handled {
		t.Fatalf("provider replay handled as inference-free=%t err=%v", handled, err)
	}
}

type deterministicFinalizationProviderProbe struct {
	calls int
}

func (probe *deterministicFinalizationProviderProbe) GeneratePreparedExact(
	context.Context,
	llm.PreparedModel,
) (llm.PreparedGeneration, error) {
	probe.calls++
	return llm.PreparedGeneration{}, fmt.Errorf("deterministic choice reached the provider")
}

func TestProviderDispatchRequiresRegisteredSemanticUncertainty(t *testing.T) {
	t.Parallel()
	_, _, err := (&Service{}).dispatchExactStationCall(
		context.Background(), model.StepAttemptAuthority{},
		exactStationCall{WorkKind: assemblyline.WorkKind("unregistered")},
		llm.PreparedModel{},
	)
	if err == nil || !strings.Contains(err.Error(), "semantic uncertainty") {
		t.Fatalf("dispatch admission error = %v", err)
	}
}

func soleJoinPathInput() assemblyline.DatabaseJoinPathSelectionInput {
	return assemblyline.DatabaseJoinPathSelectionInput{
		EvidenceNeedID: "need-1",
		ExactNeed:      "Read the orders belonging to each customer.",
		Context:        assemblyline.ObjectiveContext{},
		FromRelationID: "customers",
		ToRelationID:   "orders",
		Candidates: []assemblyline.DatabaseJoinPathCandidate{
			{
				PathID:     "customer-orders",
				Descriptor: "Customer records relate directly to order records.",
			},
		},
	}
}
