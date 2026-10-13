package worker

import (
	"context"
	"errors"
	"fmt"
	"io"
	"testing"

	"github.com/gryph/omnidex/internal/assemblyline"
	"github.com/gryph/omnidex/internal/model"
	"github.com/gryph/omnidex/internal/queue"
)

func TestSemanticWorkersStopWhenExecutionAuthorityEnds(t *testing.T) {
	classificationInput := assemblyline.ApplicationClassificationInput{UserRequest: "Classify one interface."}
	classification, err := assemblyline.NewApplicationClassificationJob(classificationInput)
	if err != nil {
		t.Fatal(err)
	}
	objectiveInput := assemblyline.ConversationObjectiveKindInput{
		ExactInstruction:   "Explain one value.",
		Context:            assemblyline.ObjectiveContext{Capsules: []assemblyline.ObjectiveContextCapsule{}},
		KnownArtifactPaths: []string{},
	}
	for _, runner := range []string{"coding", "objective"} {
		job := classification
		decodeCandidate := func(candidate string) (string, error) {
			decision, err := assemblyline.DecodeApplicationClassification(classificationInput, candidate)
			return string(decision.Surface), err
		}
		if runner == "objective" {
			job, err = assemblyline.NewConversationObjectiveKindJob(objectiveInput)
			if err != nil {
				t.Fatal(err)
			}
			decodeCandidate = func(candidate string) (string, error) {
				decision, err := assemblyline.DecodeConversationObjectiveKindDecision(objectiveInput, candidate)
				return string(decision.Kind), err
			}
			if err := validateObjectiveRawCandidatePathBoundary(job.Kind, "A", assemblyline.ArtifactIdentityProvenance{}); err != nil {
				t.Fatalf("objective test result violates its production boundary: %v", err)
			}
		}
		if prompt, err := assemblyline.RenderPortableJob(job); err != nil || prompt == "" {
			t.Fatalf("%s test job is not renderable: %v", runner, err)
		}
		if err := (assemblyline.PortableResult{Candidate: "A"}).ValidateFor(job); err != nil {
			t.Fatalf("%s test result is invalid: %v", runner, err)
		}
		if _, err := decodeCandidate("A"); err != nil {
			t.Fatalf("%s test response fails its production decoder: %v", runner, err)
		}
		for _, stage := range []string{"execute", "finalize"} {
			for _, failure := range []struct {
				name     string
				err      error
				canceled bool
			}{
				{"canceled provider receipt", &exactStationEvidencePersistenceError{
					persistenceErr: fmt.Errorf("%w: origin attempt status canceled", queue.ErrLLMCallTerminalizedByAttempt),
					callErr:        io.ErrUnexpectedEOF,
				}, false},
				{"superseded generation", fmt.Errorf("%w: job generation changed", queue.ErrStaleStepAttempt), false},
				{"revoked attempt", fmt.Errorf("%w: attempt is not active", queue.ErrStaleStepAttempt), false},
				{"canceled execution context", fmt.Errorf("authority ended: %w", context.Canceled), true},
			} {
				t.Run(runner+"/"+stage+"/"+failure.name, func(t *testing.T) {
					ctx, cancel := context.WithCancel(t.Context())
					defer cancel()
					var events []typedWorkerEvent
					var observations []model.PlanningActivity
					calls, finalizations, decoded := 0, 0, 0
					runtime := typedWorkerRuntime{
						Context: ctx,
						Execute: func(assemblyline.PortableJob, string) (assemblyline.PortableResult, error) {
							calls++
							if stage == "execute" {
								if failure.canceled {
									cancel()
								}
								return assemblyline.PortableResult{}, failure.err
							}
							return assemblyline.PortableResult{Candidate: "A"}, nil
						},
						Finalize: func(assemblyline.PortableJob, assemblyline.PortableResult, error) error {
							finalizations++
							if failure.canceled {
								cancel()
							}
							return failure.err
						},
						Emit:            func(event typedWorkerEvent) { events = append(events, event) },
						ObservePlanning: func(activity model.PlanningActivity) { observations = append(observations, activity) },
					}
					decode := func(candidate string) (string, error) {
						decoded++
						return decodeCandidate(candidate)
					}
					var err error
					if runner == "coding" {
						_, err = runDirectCodingSemanticLeafCall(runtime, "fixture-model", string(job.Kind), job, nil, decode)
					} else {
						_, err = runObjectiveRawLeafWorkerCall(runtime, "fixture-model", string(job.Kind), job, decode)
					}
					if !errors.Is(err, failure.err) {
						t.Fatalf("authority-ending error lost: %v", err)
					}
					assertStoppedTypedWorker(t, events)
					if calls != 1 || len(observations) != 1 || observations[0].Kind != "semantic_started" {
						t.Fatalf("stopped call retried or claimed a result: calls=%d observations=%+v", calls, observations)
					}
					wantFinalizations := 0
					if stage == "finalize" {
						wantFinalizations = 1
						if decoded != 1 {
							t.Fatalf("successful receipt did not reach its decoder: %d", decoded)
						}
					} else if decoded != 0 {
						t.Fatalf("terminalized provider bytes reached the decoder: %d", decoded)
					}
					if finalizations != wantFinalizations {
						t.Fatalf("finalizations=%d want=%d", finalizations, wantFinalizations)
					}
				})
			}
		}
	}
}

func TestTypedWorkersStopWithoutDiscardingAuthorityErrors(t *testing.T) {
	for _, kind := range []typedWorkerKind{typedWorkerSemantic, typedWorkerFragment} {
		for _, failure := range []error{
			fmt.Errorf("%w: origin attempt canceled", queue.ErrLLMCallTerminalizedByAttempt),
			fmt.Errorf("%w: job generation changed", queue.ErrStaleStepAttempt),
			context.Canceled,
		} {
			t.Run(string(kind)+"/"+failure.Error(), func(t *testing.T) {
				ctx, cancel := context.WithCancel(t.Context())
				defer cancel()
				if errors.Is(failure, context.Canceled) {
					cancel()
				}
				var events []typedWorkerEvent
				runtime := typedWorkerRuntime{Context: ctx, Emit: func(event typedWorkerEvent) { events = append(events, event) }}
				err := failTypedWorkerForTest(runtime, kind, failure)
				if !errors.Is(err, failure) {
					t.Fatalf("authority error lost: %v", err)
				}
				assertStoppedTypedWorker(t, events)
			})
		}
	}
}

func TestTypedWorkersRetainGenuineFailureDuringCancellation(t *testing.T) {
	databaseErr := errors.New("PostgreSQL receipt commit unavailable")
	for _, kind := range []typedWorkerKind{typedWorkerSemantic, typedWorkerFragment} {
		for _, failure := range []error{
			io.ErrUnexpectedEOF,
			context.DeadlineExceeded,
			databaseErr,
			&exactStationEvidencePersistenceError{persistenceErr: databaseErr, callErr: context.Canceled},
			errors.Join(queue.ErrStaleStepAttempt, databaseErr),
			fmt.Errorf("%w: database lease validation unavailable", errStepExecutionAuthorityLost),
		} {
			t.Run(string(kind)+"/"+failure.Error(), func(t *testing.T) {
				ctx, cancel := context.WithCancel(t.Context())
				cancel()
				var events []typedWorkerEvent
				runtime := typedWorkerRuntime{Context: ctx, Emit: func(event typedWorkerEvent) { events = append(events, event) }}
				err := failTypedWorkerForTest(runtime, kind, failure)
				if !errors.Is(err, failure) {
					t.Fatalf("genuine failure lost: %v", err)
				}
				if len(events) != 1 || events[0].State != typedWorkerFailed || events[0].Detail == "" {
					t.Fatalf("genuine failure hidden: %+v", events)
				}
			})
		}
	}
}

func TestTypedWorkerCanceledErrorWithoutCanceledContextFails(t *testing.T) {
	var events []typedWorkerEvent
	runtime := typedWorkerRuntime{Context: t.Context(), Emit: func(event typedWorkerEvent) { events = append(events, event) }}
	if err := failDirectCodingSemanticCall(runtime, "fixture-model", "application_surface", 1, context.Canceled); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled provider error lost: %v", err)
	}
	if len(events) != 1 || events[0].State != typedWorkerFailed || events[0].Detail == "" {
		t.Fatalf("unexplained provider cancellation hidden: %+v", events)
	}
}

func failTypedWorkerForTest(runtime typedWorkerRuntime, kind typedWorkerKind, failure error) error {
	if kind == typedWorkerSemantic {
		return failDirectCodingSemanticCall(runtime, "fixture-model", "application_surface", 1, failure)
	}
	return failDirectCodingLanguageGeneration(runtime, "fixture-model", directCodingLanguageGenerationJob{
		Subject: "source.001", Input: assemblyline.FragmentGenerationInput{Language: "go"},
	}, 1, failure)
}

func assertStoppedTypedWorker(t *testing.T, events []typedWorkerEvent) {
	t.Helper()
	if len(events) == 0 || events[len(events)-1].State != typedWorkerState("stopped") {
		t.Fatalf("authority ended but worker reported failure: %+v", events)
	}
	for _, event := range events {
		if event.State != typedWorkerStarted && event.State != typedWorkerState("stopped") || event.Detail != "" {
			t.Fatalf("intentional stop reported an error, rejection or success: %+v", event)
		}
	}
}
