package worker

import (
	"errors"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/assemblyline"
	"github.com/gryph/omnidex/internal/model"
)

func TestSemanticPlanningExplainsEveryCallAndValidatedSelection(t *testing.T) {
	for _, runner := range []string{"coding"} {
		for _, reject := range []bool{false, true} {
			t.Run(runner+"/finalization", func(t *testing.T) {
				candidate := "The finished software lets a user select a colour."
				request := "Build software that lets a user select a colour."
				context, err := assemblyline.BootstrapApplicationContext(request)
				if err != nil {
					t.Fatal(err)
				}
				input := assemblyline.ApplicationRequirementCandidateAuthorizationInput{UserRequest: request, Context: context, Candidate: candidate}
				job, err := assemblyline.NewApplicationRequirementCandidateAuthorizationJob(input)
				if err != nil {
					t.Fatal(err)
				}
				var observations []model.PlanningActivity
				calls, finalized := 0, false
				runtime := typedWorkerRuntime{Context: t.Context(), ObservePlanning: func(activity model.PlanningActivity) {
					if err := activity.Validate(); err != nil {
						t.Fatal(err)
					}
					if activity.Kind == "semantic_completed" && !finalized {
						t.Fatal("uncommitted result published")
					}
					observations = append(observations, activity)
				}, Execute: func(actual assemblyline.PortableJob, actualModel string) (assemblyline.PortableResult, error) {
					calls++
					if len(observations) != 1 {
						t.Fatal("semantic call submitted without its question, reason and actual choices")
					}
					start := observations[0]
					if start.Kind != "semantic_started" || start.Subject != candidate || start.Model != "fixture-model" || start.Station != string(job.Kind) || start.CallID != 0 || len(start.Options) != 2 || start.Why == "" || start.Question == "" || start.Consumer == "" {
						t.Fatalf("incomplete call explanation: %+v", start)
					}
					if actual.Key() != job.Key() || actualModel != "fixture-model" {
						t.Fatal("display changed original call")
					}
					return assemblyline.PortableResult{Candidate: "B"}, nil
				}, ReadCallEvidenceID: func(actual assemblyline.PortableJob) int64 {
					if finalized || actual.Key() != job.Key() {
						t.Fatal("call identity was not read from the original pending execution")
					}
					return 23
				}, Finalize: func(_ assemblyline.PortableJob, _ assemblyline.PortableResult, err error) error {
					if err != nil {
						return err
					}
					if reject {
						return errors.New("fixture journal refusal")
					}
					finalized = true
					return nil
				}}
				decode := func(raw string) (assemblyline.ApplicationRequirementCandidateAuthorizationResult, error) {
					return assemblyline.DecodeApplicationRequirementCandidateAuthorizationResult(input, raw)
				}
				if runner == "coding" {
					_, err = runDirectCodingSemanticLeafCall(runtime, "fixture-model", string(job.Kind), job, nil, decode)
				} else {
					_, err = runObjectiveRawLeafWorkerCall(runtime, "fixture-model", string(job.Kind), job, decode)
				}
				if reject {
					if err == nil || len(observations) != 1 {
						t.Fatal("rejected finalization claimed a result")
					}
					return
				}
				if err != nil || calls != 1 || len(observations) != 2 {
					t.Fatalf("unchanged call lacks exact lifecycle: calls=%d observations=%v err=%v", calls, observations, err)
				}
				result := observations[1]
				if result.Kind != "semantic_completed" || result.CallID != 23 || result.WorkKind != observations[0].WorkKind || result.Station != observations[0].Station || result.Result != observations[0].Options[1] || !strings.Contains(result.Result, "not grounded") {
					t.Fatalf("actual selection was lost: %+v", result)
				}
			})
		}
	}
}
