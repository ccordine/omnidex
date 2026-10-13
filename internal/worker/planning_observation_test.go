package worker

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/assemblyline"
	"github.com/gryph/omnidex/internal/model"
)

func TestPlanningObservationReportsValidatedCandidateDispositions(t *testing.T) {
	const unrequested = "The finished software sends an unrelated report."
	const compound = "The finished software bookmarks passages and silences alarms."
	const bookmark = "The finished software bookmarks passages."
	const silence = "The finished software silences alarms."
	const unresolved = "The finished software is pleasing."
	const request = "Build software to bookmark passages and silence alarms."
	authority := planningObservationAuthority(t, request)
	var observations, semantic []model.PlanningActivity
	var responses []string
	calls, finalized := 0, false
	runtime := typedWorkerRuntime{Context: context.Background(),
		Execute: func(job assemblyline.PortableJob, actualModel string) (assemblyline.PortableResult, error) {
			if len(semantic) != calls*2+1 || semantic[len(semantic)-1].Kind != "semantic_started" {
				t.Fatal("existing semantic call lacks its preceding explanation")
			}
			assertPlanningSemanticStart(t, semantic[len(semantic)-1], job, actualModel)
			calls++
			finalized = false
			response := "A"
			switch job.Kind {
			case assemblyline.WorkApplicationRequirementInventory:
				response = strings.Join([]string{unrequested, compound, unresolved, bookmark}, "\n")
			case assemblyline.WorkApplicationRequirementCandidateAuthorization:
				var input assemblyline.ApplicationRequirementCandidateAuthorizationInput
				if err := json.Unmarshal(job.Payload, &input); err != nil {
					return assemblyline.PortableResult{}, err
				}
				if input.Candidate == unrequested {
					response = "B"
				}
			case assemblyline.WorkApplicationRequirementCandidateKind:
				var input assemblyline.ApplicationRequirementCandidateContentPresenceInput
				if err := json.Unmarshal(job.Payload, &input); err != nil {
					return assemblyline.PortableResult{}, err
				}
				if input.Dimension == assemblyline.ApplicationRequirementCandidateNonRuntimeContentDimension || input.Candidate == unresolved {
					response = "B"
				}
			case assemblyline.WorkApplicationRequirementCandidateCardinality:
				var input assemblyline.ApplicationRequirementCandidateCardinalityInput
				if err := json.Unmarshal(job.Payload, &input); err != nil {
					return assemblyline.PortableResult{}, err
				}
				if input.Candidate == compound {
					response = "B"
				}
			case assemblyline.WorkApplicationRequirementCandidatePartition:
				response = bookmark + "\n" + silence
			case assemblyline.WorkApplicationRequirementCandidateResultRelation,
				assemblyline.WorkApplicationRequirementCandidateOutcomeRelation:
				response = "B"
			default:
				return assemblyline.PortableResult{}, fmt.Errorf("unexpected planning work %q", job.Kind)
			}
			responses = append(responses, response)
			return assemblyline.PortableResult{Candidate: response}, nil
		},
		Finalize: func(_ assemblyline.PortableJob, _ assemblyline.PortableResult, err error) error {
			finalized = true
			return err
		},
	}
	installPlanningObserver(t, &runtime, func(activity model.PlanningActivity) {
		if activity.Kind == "semantic_started" {
			semantic = append(semantic, activity)
			return
		}
		if activity.Kind == "semantic_completed" {
			if !finalized || len(semantic) != calls*2-1 {
				t.Fatal("semantic result preceded finalization or lost its exact call")
			}
			start := semantic[len(semantic)-1]
			result := responses[len(responses)-1]
			for _, option := range start.Options {
				id, _, _ := strings.Cut(option, ". ")
				if result == id {
					result = option
					break
				}
			}
			start.Kind, start.Result = "semantic_completed", result
			if !reflect.DeepEqual(activity, start) {
				t.Fatalf("validated semantic result lost exact question/options: %+v, want %+v", activity, start)
			}
			semantic = append(semantic, activity)
			return
		}
		if activity.Kind == "inventory_completed" || activity.Kind == "candidate_resolved" {
			if !finalized {
				t.Fatal("planning observation preceded semantic finalization")
			}
		}
		observations = append(observations, activity)
	})
	proposals, err := resolveDirectCodingApplicationPlan(runtime, planningObservationModels(), authority, nil)
	if err != nil {
		t.Fatal(err)
	}
	want := []model.PlanningActivity{
		{Schema: 1, Kind: "inventory_started"},
		{Schema: 1, Kind: "inventory_completed", Count: 4},
		{Schema: 1, Kind: "candidate_started", Subject: unrequested},
		{Schema: 1, Kind: "candidate_resolved", Subject: unrequested, Outcome: "unrequested"},
		{Schema: 1, Kind: "candidate_started", Subject: compound},
		{Schema: 1, Kind: "candidate_resolved", Subject: compound, Outcome: "partitioned", Count: 2},
		{Schema: 1, Kind: "candidate_started", Subject: bookmark},
		{Schema: 1, Kind: "candidate_resolved", Subject: bookmark, Outcome: "retained"},
		{Schema: 1, Kind: "candidate_started", Subject: silence},
		{Schema: 1, Kind: "candidate_resolved", Subject: silence, Outcome: "retained"},
		{Schema: 1, Kind: "candidate_started", Subject: unresolved},
		{Schema: 1, Kind: "candidate_resolved", Subject: unresolved, Outcome: "unresolved"},
		{Schema: 1, Kind: "candidate_started", Subject: bookmark},
		{Schema: 1, Kind: "candidate_resolved", Subject: bookmark, Outcome: "duplicate"},
		{Schema: 1, Kind: "selected", Count: 2},
	}
	if !reflect.DeepEqual(observations, want) {
		t.Fatalf("request-specific planning observations=%+v, want %+v", observations, want)
	}
	if calls != 20 || len(semantic) != 40 || len(proposals) != 2 || proposals[0].Statement != bookmark || proposals[1].Statement != silence {
		t.Fatalf("observation changed planning: calls=%d proposals=%+v", calls, proposals)
	}
}

func TestPlanningObservationWithholdsUnvalidatedOutcomes(t *testing.T) {
	const candidate = "The finished software bookmarks passages."
	authority := planningObservationAuthority(t, "Build a passage bookmarker.")
	for _, failed := range []string{"inventory_decode", "inventory_finalize", "candidate_decode", "candidate_finalize"} {
		t.Run(failed, func(t *testing.T) {
			var observations, semantic []model.PlanningActivity
			calls := 0
			runtime := typedWorkerRuntime{Context: context.Background(),
				Execute: func(job assemblyline.PortableJob, _ string) (assemblyline.PortableResult, error) {
					calls++
					response := "A"
					if job.Kind == assemblyline.WorkApplicationRequirementInventory {
						response = candidate
						if failed == "inventory_decode" {
							response = "\n"
						}
					} else if failed == "candidate_decode" {
						response = "MALFORMED_AUTHORIZATION_RELATION"
					}
					return assemblyline.PortableResult{Candidate: response}, nil
				},
				Finalize: func(job assemblyline.PortableJob, _ assemblyline.PortableResult, err error) error {
					if failed == "inventory_finalize" && job.Kind == assemblyline.WorkApplicationRequirementInventory ||
						failed == "candidate_finalize" && job.Kind == assemblyline.WorkApplicationRequirementCandidateAuthorization {
						return fmt.Errorf("fixture finalization failed")
					}
					return err
				},
			}
			installPlanningObserver(t, &runtime, func(activity model.PlanningActivity) {
				if activity.Kind == "semantic_started" || activity.Kind == "semantic_completed" {
					semantic = append(semantic, activity)
					return
				}
				observations = append(observations, activity)
			})
			_, err := resolveDirectCodingApplicationPlan(runtime, planningObservationModels(), authority, nil)
			if err == nil {
				t.Fatal("invalid planning station unexpectedly succeeded")
			}
			want := []model.PlanningActivity{{Schema: 1, Kind: "inventory_started"}}
			wantCalls := 1
			if strings.HasPrefix(failed, "candidate_") {
				want = append(want, model.PlanningActivity{Schema: 1, Kind: "inventory_completed", Count: 1},
					model.PlanningActivity{Schema: 1, Kind: "candidate_started", Subject: candidate})
				wantCalls = 2
			}
			if !reflect.DeepEqual(observations, want) || calls != wantCalls {
				t.Fatalf("invalid output leaked planning state: observations=%+v calls=%d", observations, calls)
			}
			if len(semantic) != wantCalls*2-1 || semantic[len(semantic)-1].Kind != "semantic_started" {
				t.Fatalf("unvalidated semantic result was published: %+v", semantic)
			}
		})
	}
}

func assertPlanningSemanticStart(t *testing.T, activity model.PlanningActivity, job assemblyline.PortableJob, actualModel string) {
	t.Helper()
	if err := activity.Validate(); err != nil {
		t.Fatal(err)
	}
	contract, err := assemblyline.SemanticUncertaintyContractForWorkKind(job.Kind)
	if err != nil {
		t.Fatal(err)
	}
	if activity.WorkKind != string(job.Kind) || activity.Model != actualModel || activity.Why != contract.DeterministicLimitation ||
		activity.Consumer != contract.DeterministicConsumer || activity.Result != "" || activity.InferenceFree {
		t.Fatalf("semantic call explanation changed its exact scope: %+v", activity)
	}
	prompt, err := assemblyline.RenderPortableJob(job)
	if err != nil {
		t.Fatal(err)
	}
	if job.Kind == assemblyline.WorkApplicationRequirementInventory || job.Kind == assemblyline.WorkApplicationRequirementCandidatePartition {
		if activity.Question != contract.ExactQuestion || !reflect.DeepEqual(activity.Options, []string{"Free-form output: " + contract.SingleResult}) {
			t.Fatalf("free-form call misstates its actual permitted output: %+v", activity)
		}
		return
	}
	if len(activity.Options) != 2 || !strings.Contains(prompt, "\n\n"+activity.Question+"\n\n") {
		t.Fatalf("semantic call omits its exact offered question/options: %+v", activity)
	}
	for _, option := range activity.Options {
		if !strings.Contains(prompt, "\n\n"+option+"\n\n") {
			t.Fatalf("semantic explanation invents an option absent from the request: %q", option)
		}
	}
}

func TestPlanningObservationRuntimeRetainsClaimedAuthority(t *testing.T) {
	var events []RuntimeEvent
	authority := model.StepAttemptAuthority{JobID: 13, StepID: 17, Attempt: 3}
	service := &Service{
		runtimeEventChannels: map[int64]runtimeEventChannelBinding{13: {channelID: "planning-channel", users: 1}},
		runtimeEventSink: func(event RuntimeEvent) error {
			events = append(events, event)
			return nil
		},
	}
	runtime := portableWorkerRuntime(&nativeRuntimeV3{svc: service, ctx: context.Background(),
		claim: &model.ClaimedStep{Authority: authority}}, "coding")
	field := reflect.ValueOf(runtime).FieldByName("ObservePlanning")
	if !field.IsValid() || field.IsNil() {
		t.Fatal("portable runtime does not observe planning")
	}
	activity := model.PlanningActivity{Schema: 1, Kind: "candidate_started", Subject: "The finished software bookmarks passages."}
	field.Interface().(func(model.PlanningActivity))(activity)
	if len(events) != 1 {
		t.Fatalf("planning runtime events=%+v", events)
	}
	event := events[0]
	observed := reflect.ValueOf(event).FieldByName("PlanningActivity")
	if event.JobID != 13 || event.ChannelID != "planning-channel" || event.StepID != 17 || event.Attempt != 3 ||
		event.Kind != "planning_activity" || !observed.IsValid() || observed.IsNil() ||
		!reflect.DeepEqual(observed.Interface(), &activity) || event.AgentActivity != nil || event.Detail != "" {
		t.Fatalf("planning observation lost its claimed authority: %+v", event)
	}
}

func installPlanningObserver(t *testing.T, runtime *typedWorkerRuntime, observe func(model.PlanningActivity)) {
	t.Helper()
	field := reflect.ValueOf(runtime).Elem().FieldByName("ObservePlanning")
	if field.IsValid() {
		field.Set(reflect.ValueOf(observe))
	}
}

func planningObservationAuthority(t *testing.T, request string) assemblyline.ApplicationIntentInput {
	t.Helper()
	applicationContext, err := assemblyline.BootstrapApplicationContext(request)
	if err != nil {
		t.Fatal(err)
	}
	return assemblyline.ApplicationIntentInput{UserRequest: request, Context: applicationContext}
}

func planningObservationModels() directCodingApplicationIntentModels {
	return directCodingApplicationIntentModels{Requirements: "fixture-requirement", ResultRelation: "fixture-result"}
}
