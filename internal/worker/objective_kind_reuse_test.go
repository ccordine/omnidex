package worker

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/assemblyline"
	"github.com/gryph/omnidex/internal/model"
	"github.com/gryph/omnidex/internal/queue"
	"github.com/gryph/omnidex/internal/station"
)

func TestObjectiveKindStationRestoresAcceptedLeafBeforeModelResolution(t *testing.T) {
	t.Parallel()
	input := assemblyline.ConversationObjectiveKindInput{
		ExactInstruction:   "Explain the bounded subject.",
		Context:            assemblyline.ObjectiveContext{Capsules: []assemblyline.ObjectiveContextCapsule{}},
		KnownArtifactPaths: []string{},
	}
	job, err := assemblyline.NewConversationObjectiveKindJob(input)
	if err != nil {
		t.Fatal(err)
	}
	projection, err := assemblyline.NewExactPortableResultProjection(
		string(assemblyline.ObjectiveKindAnswer),
	)
	if err != nil {
		t.Fatal(err)
	}
	reuseCalls := 0
	service := &Service{reuseObjectiveResult: func(
		_ context.Context,
		request queue.ObjectivePortableResultReuseRequest,
	) (queue.ObjectivePortableResultReuse, bool, error) {
		reuseCalls++
		if request.Job.ID != job.ID || request.Station != station.ConversationObjectiveKind {
			t.Fatalf("reuse request=%+v", request)
		}
		return queue.ObjectivePortableResultReuse{Result: assemblyline.PortableResult{
			JobID: job.ID, Candidate: string(assemblyline.ObjectiveKindAnswer),
			Projection: &projection,
		}}, true, nil
	}}
	runtime := &nativeRuntimeV3{svc: service, claim: &model.ClaimedStep{}}

	decision, receipt, err := (portableObjectiveKindStation{runtime: runtime}).Classify(
		t.Context(), input,
	)
	if err != nil {
		t.Fatal(err)
	}
	if reuseCalls != 1 || decision.Kind != assemblyline.ObjectiveKindAnswer ||
		receipt.Calls != 0 || !receipt.Reused {
		t.Fatalf(
			"reuse_calls=%d decision=%+v receipt=%+v",
			reuseCalls, decision, receipt,
		)
	}
}

func TestObjectiveTurnAcceptsRestoredObjectiveKindWithoutFabricatingCall(t *testing.T) {
	t.Parallel()
	kind := &objectiveKindReceiptStation{
		decision: assemblyline.ConversationObjectiveKindDecision{
			Schema: assemblyline.ConversationObjectiveKindSchemaV1,
			Kind:   assemblyline.ObjectiveKindAnswer,
		},
		receipt: objectiveStationReceipt{Reused: true},
	}
	result, err := runObjectiveTurn(
		t.Context(),
		model.Job{
			ID: 9181, Pipeline: model.PipelineChat,
			Instruction: "Explain the bounded subject.", Metadata: objectiveAssistantMetadata(),
		},
		scriptedConversationCandidateProvider{}, emptyContextSieveStation(), kind,
		&scriptedObjectiveConversationStation{}, &scriptedObjectiveAnswerStation{},
		objectiveWorkflows{},
	)
	if err != nil {
		t.Fatal(err)
	}
	if !result.Complete || result.Kind != assemblyline.ObjectiveKindAnswer ||
		result.ModelCalls != 1 {
		t.Fatalf("result=%+v", result)
	}
}

func TestOrdinaryConversationResponseRestoresAcceptedLeafBeforeModelResolution(t *testing.T) {
	t.Parallel()
	input := assemblyline.ConversationResponseInput{
		Kind:               assemblyline.ObjectiveKindAnswer,
		ExactInstruction:   "Explain the bounded subject.",
		Context:            assemblyline.ObjectiveContext{Capsules: []assemblyline.ObjectiveContextCapsule{}},
		KnownArtifactPaths: []string{},
	}
	job, err := assemblyline.NewConversationResponseJob(input)
	if err != nil {
		t.Fatal(err)
	}
	const candidate = "The bounded subject is restored."
	projection, err := assemblyline.NewExactPortableResultProjection(candidate)
	if err != nil {
		t.Fatal(err)
	}
	reuseCalls := 0
	service := &Service{reuseObjectiveResult: func(
		_ context.Context,
		request queue.ObjectivePortableResultReuseRequest,
	) (queue.ObjectivePortableResultReuse, bool, error) {
		reuseCalls++
		if request.Job.ID != job.ID || request.Station != station.ConversationResponse {
			t.Fatalf("reuse request=%+v", request)
		}
		return queue.ObjectivePortableResultReuse{Result: assemblyline.PortableResult{
			JobID: job.ID, Candidate: candidate, Projection: &projection,
		}}, true, nil
	}}
	runtime := &nativeRuntimeV3{svc: service, claim: &model.ClaimedStep{}}

	decision, receipt, err := (portableObjectiveConversationStation{runtime: runtime}).Respond(
		t.Context(), input, "",
	)
	if err != nil {
		t.Fatal(err)
	}
	if reuseCalls != 1 || decision.Text != candidate || receipt.Calls != 0 || !receipt.Reused {
		t.Fatalf(
			"reuse_calls=%d decision=%+v receipt=%+v",
			reuseCalls, decision, receipt,
		)
	}
}

func TestObjectiveTurnAcceptsRestoredOrdinaryResponseWithoutFabricatingCall(t *testing.T) {
	t.Parallel()
	result, err := runObjectiveTurn(
		t.Context(),
		model.Job{
			ID: 9183, Pipeline: model.PipelineChat,
			Instruction: "Explain the bounded subject.", Metadata: objectiveAssistantMetadata(),
		},
		scriptedConversationCandidateProvider{}, emptyContextSieveStation(),
		&objectiveKindReceiptStation{
			decision: assemblyline.ConversationObjectiveKindDecision{
				Schema: assemblyline.ConversationObjectiveKindSchemaV1,
				Kind:   assemblyline.ObjectiveKindAnswer,
			},
			receipt: objectiveStationReceipt{Calls: 1},
		},
		&objectiveResponseReceiptStation{
			decision: assemblyline.ConversationResponseDecision{
				Schema: assemblyline.ConversationResponseSchemaV1,
				Text:   "The bounded subject is restored.",
			},
			receipt: objectiveStationReceipt{Reused: true},
		},
		&scriptedObjectiveAnswerStation{}, objectiveWorkflows{},
	)
	if err != nil {
		t.Fatal(err)
	}
	if !result.Complete || result.Output != "The bounded subject is restored." ||
		result.ModelCalls != 1 {
		t.Fatalf("result=%+v", result)
	}
}

func TestObjectiveTurnRejectsOrdinaryResponseReceiptWithoutExactSource(t *testing.T) {
	t.Parallel()
	for _, fixture := range []struct {
		name    string
		receipt objectiveStationReceipt
	}{
		{name: "zero calls without reuse provenance"},
		{name: "fresh call marked reused", receipt: objectiveStationReceipt{Calls: 1, Reused: true}},
	} {
		fixture := fixture
		t.Run(fixture.name, func(t *testing.T) {
			t.Parallel()
			result, err := runObjectiveTurn(
				t.Context(),
				model.Job{
					ID: 9184, Pipeline: model.PipelineChat,
					Instruction: "Explain the bounded subject.", Metadata: objectiveAssistantMetadata(),
				},
				scriptedConversationCandidateProvider{}, emptyContextSieveStation(),
				&objectiveKindReceiptStation{
					decision: assemblyline.ConversationObjectiveKindDecision{
						Schema: assemblyline.ConversationObjectiveKindSchemaV1,
						Kind:   assemblyline.ObjectiveKindAnswer,
					},
					receipt: objectiveStationReceipt{Calls: 1},
				},
				&objectiveResponseReceiptStation{
					decision: assemblyline.ConversationResponseDecision{
						Schema: assemblyline.ConversationResponseSchemaV1,
						Text:   "This response has no exact source.",
					},
					receipt: fixture.receipt,
				},
				&scriptedObjectiveAnswerStation{}, objectiveWorkflows{},
			)
			if err == nil || result.Complete {
				t.Fatalf("receipt=%+v result=%+v error=%v", fixture.receipt, result, err)
			}
		})
	}
}

func TestObjectiveTurnRejectsObjectiveKindReceiptWithoutExactSource(t *testing.T) {
	t.Parallel()
	for _, fixture := range []struct {
		name    string
		receipt objectiveStationReceipt
	}{
		{name: "zero calls without reuse provenance"},
		{name: "fresh call marked reused", receipt: objectiveStationReceipt{Calls: 1, Reused: true}},
	} {
		fixture := fixture
		t.Run(fixture.name, func(t *testing.T) {
			t.Parallel()
			mutated := false
			_, err := runObjectiveTurn(
				t.Context(),
				model.Job{
					ID: 9182, Pipeline: model.PipelineChat,
					Instruction: "Change the bounded subject.", Metadata: objectiveAssistantMetadata(),
				},
				scriptedConversationCandidateProvider{}, emptyContextSieveStation(),
				&objectiveKindReceiptStation{
					decision: assemblyline.ConversationObjectiveKindDecision{
						Schema: assemblyline.ConversationObjectiveKindSchemaV1,
						Kind:   assemblyline.ObjectiveKindWorkspaceMutation,
					},
					receipt: fixture.receipt,
				},
				&scriptedObjectiveConversationStation{}, &scriptedObjectiveAnswerStation{},
				objectiveWorkflows{WorkspaceMutation: func(context.Context, turnAuthority) (string, error) {
					mutated = true
					return "must not execute", nil
				}},
			)
			if err == nil || mutated {
				t.Fatalf("receipt=%+v mutated=%t error=%v", fixture.receipt, mutated, err)
			}
		})
	}
}

func TestObjectiveTurnStationSourceHasNoFreshCallMinimum(t *testing.T) {
	t.Parallel()
	stationSource, err := os.ReadFile("objective_turn_stations.go")
	if err != nil {
		t.Fatal(err)
	}
	start := strings.Index(string(stationSource), "func (adapter portableObjectiveKindStation) Classify")
	end := strings.Index(string(stationSource), "type portableObjectiveConversationStation")
	if start < 0 || end <= start {
		t.Fatal("objective-kind station source boundary is unavailable")
	}
	body := string(stationSource[start:end])
	if !strings.Contains(body, "runObjectiveReusablePortableRawLeafCall(") ||
		strings.Contains(body, "runObjectivePortableRawLeafCall(") {
		t.Fatal("objective-kind station bypasses exact accepted-result reuse")
	}
	responseStart := strings.Index(string(stationSource), "func (adapter portableObjectiveConversationStation) Respond")
	responseEnd := strings.Index(string(stationSource), "func objectiveStationModel")
	if responseStart < 0 || responseEnd <= responseStart {
		t.Fatal("conversation-response station source boundary is unavailable")
	}
	responseBody := string(stationSource[responseStart:responseEnd])
	if !strings.Contains(responseBody, "runObjectiveReusablePortableRawLeafCall(") ||
		strings.Contains(responseBody, "runObjectivePortableRawLeafCall(") {
		t.Fatal("ordinary conversation-response station bypasses exact accepted-result reuse")
	}
	workflowSource, err := os.ReadFile("objective_turn_workflow.go")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(workflowSource), "receipt.Calls != exactSemanticLeafCalls") {
		t.Fatal("objective-kind workflow retains a mandatory fresh-call minimum")
	}
}

type objectiveKindReceiptStation struct {
	decision assemblyline.ConversationObjectiveKindDecision
	receipt  objectiveStationReceipt
}

func (station *objectiveKindReceiptStation) Classify(
	_ context.Context,
	_ assemblyline.ConversationObjectiveKindInput,
) (assemblyline.ConversationObjectiveKindDecision, objectiveStationReceipt, error) {
	return station.decision, station.receipt, nil
}

type objectiveResponseReceiptStation struct {
	decision assemblyline.ConversationResponseDecision
	receipt  objectiveStationReceipt
}

func (station *objectiveResponseReceiptStation) Respond(
	_ context.Context,
	_ assemblyline.ConversationResponseInput,
	_ string,
) (assemblyline.ConversationResponseDecision, objectiveStationReceipt, error) {
	return station.decision, station.receipt, nil
}
