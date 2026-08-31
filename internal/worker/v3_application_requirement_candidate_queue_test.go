package worker

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/assemblyline"
)

func TestApplicationRequirementQueueDiscardsBadProposalsWithoutReviewingAcceptedLeaves(
	t *testing.T,
) {
	t.Parallel()
	const request = "Build a browser status board that displays the current status and sends an alert when the submitted status changes. Use TypeScript."
	const first = "Display the current status."
	const firstParaphrase = "Show the current status."
	const second = "Send an alert when the submitted status changes."
	const invented = "Store previous statuses."
	const nonRuntime = "Use TypeScript."
	inventory := strings.Join([]string{
		first,
		nonRuntime,
		firstParaphrase,
		second,
		second,
		invented,
	}, "\n")
	counts := map[assemblyline.WorkKind]int{}
	runtime := typedWorkerRuntime{
		Context: context.Background(), MaxAttempts: 1,
		Execute: func(job assemblyline.PortableJob, _ string) (assemblyline.PortableResult, error) {
			counts[job.Kind]++
			candidate := ""
			switch job.Kind {
			case assemblyline.WorkApplicationProductContext:
				candidate = "A browser status board."
			case assemblyline.WorkApplicationRequirementInventory:
				candidate = inventory
			case assemblyline.WorkApplicationRequirementCandidateKind:
				leaf, err := applicationRequirementKindCandidate(job)
				if err != nil {
					return assemblyline.PortableResult{}, err
				}
				if leaf == invented {
					return assemblyline.PortableResult{}, fmt.Errorf(
						"unrequested candidate reached kind classification",
					)
				}
				kind := assemblyline.ApplicationRequirementCandidateTaskLocal
				if leaf == nonRuntime {
					kind = assemblyline.ApplicationRequirementCandidateNonRuntime
				}
				candidate, err = applicationRequirementCandidateContentPresenceForKindForTest(job, kind)
				if err != nil {
					return assemblyline.PortableResult{}, err
				}
			case assemblyline.WorkApplicationRequirementCandidateCardinality:
				candidate = assemblyline.ApplicationRequirementOneRuntimeOutcome
			case assemblyline.WorkApplicationRequirementCandidateAuthorization:
				leaf, err := applicationRequirementAuthorizationCandidate(job)
				if err != nil {
					return assemblyline.PortableResult{}, err
				}
				if leaf == invented {
					candidate = assemblyline.ApplicationRequirementCandidateNotEntailed
				} else {
					candidate = assemblyline.ApplicationRequirementCandidateEntailed
				}
			case assemblyline.WorkApplicationRequirementCandidateOutcomeRelation:
				var input assemblyline.ApplicationRequirementCandidateOutcomeRelationInput
				if err := json.Unmarshal(job.Payload, &input); err != nil {
					return assemblyline.PortableResult{}, err
				}
				if input.Candidate == firstParaphrase && input.AcceptedRequirement == first {
					candidate = assemblyline.ApplicationRequirementSameRuntimeOutcome
				} else {
					candidate = assemblyline.ApplicationRequirementDistinctRuntimeOutcomes
				}
			case assemblyline.WorkApplicationRequirementCandidateResultRelation:
				presence, presenceErr := applicationRequirementCandidateResultPresenceForRelationForTest(
					job, assemblyline.ApplicationRequirementNoDerivedResult,
				)
				if presenceErr != nil {
					return assemblyline.PortableResult{}, presenceErr
				}
				candidate = presence
			default:
				return assemblyline.PortableResult{}, fmt.Errorf("unexpected work kind %q", job.Kind)
			}
			return assemblyline.PortableResult{JobID: job.ID, Candidate: candidate}, nil
		},
	}
	resolution, err := resolveApplicationRequirementQueueFixture(t, runtime, request)
	if err != nil {
		t.Fatal(err)
	}
	if got := applicationRequirementStatements(resolution); !reflect.DeepEqual(got, []string{first, second}) {
		t.Fatalf("requirements=%v", got)
	}
	want := map[assemblyline.WorkKind]int{
		assemblyline.WorkApplicationProductContext:                      1,
		assemblyline.WorkApplicationRequirementInventory:                1,
		assemblyline.WorkApplicationRequirementCandidateKind:            7,
		assemblyline.WorkApplicationRequirementCandidateCardinality:     3,
		assemblyline.WorkApplicationRequirementCandidateAuthorization:   5,
		assemblyline.WorkApplicationRequirementCandidateOutcomeRelation: 2,
		assemblyline.WorkApplicationRequirementCandidateResultRelation:  2,
	}
	if !reflect.DeepEqual(counts, want) {
		t.Fatalf("calls=%v want=%v", counts, want)
	}
}

func TestApplicationRequirementQueueSplicesEveryPartitionChildInSourceOrder(t *testing.T) {
	t.Parallel()
	const request = "Build a record utility that shows the current record and exports it as CSV."
	const parent = "Show the current record and export it as CSV."
	children := []string{"Show the current record.", "Export the current record as CSV."}
	counts := map[assemblyline.WorkKind]int{}
	runtime := typedWorkerRuntime{
		Context: context.Background(), MaxAttempts: 1,
		Execute: func(job assemblyline.PortableJob, _ string) (assemblyline.PortableResult, error) {
			counts[job.Kind]++
			candidate := ""
			switch job.Kind {
			case assemblyline.WorkApplicationProductContext:
				candidate = "A record utility."
			case assemblyline.WorkApplicationRequirementInventory:
				candidate = parent
			case assemblyline.WorkApplicationRequirementCandidateKind:
				var err error
				candidate, err = applicationRequirementCandidateContentPresenceForKindForTest(
					job,
					assemblyline.ApplicationRequirementCandidateTaskLocal,
				)
				if err != nil {
					return assemblyline.PortableResult{}, err
				}
			case assemblyline.WorkApplicationRequirementCandidateCardinality:
				leaf, err := applicationRequirementCardinalityCandidate(job)
				if err != nil {
					return assemblyline.PortableResult{}, err
				}
				if leaf == parent {
					candidate = assemblyline.ApplicationRequirementMultipleRuntimeOutcomes
				} else {
					candidate = assemblyline.ApplicationRequirementOneRuntimeOutcome
				}
			case assemblyline.WorkApplicationRequirementCandidatePartition:
				candidate = strings.Join(children, "\n")
			case assemblyline.WorkApplicationRequirementCandidateAuthorization:
				candidate = assemblyline.ApplicationRequirementCandidateEntailed
			case assemblyline.WorkApplicationRequirementCandidateOutcomeRelation:
				candidate = assemblyline.ApplicationRequirementDistinctRuntimeOutcomes
			case assemblyline.WorkApplicationRequirementCandidateResultRelation:
				presence, presenceErr := applicationRequirementCandidateResultPresenceForRelationForTest(
					job, assemblyline.ApplicationRequirementNoDerivedResult,
				)
				if presenceErr != nil {
					return assemblyline.PortableResult{}, presenceErr
				}
				candidate = presence
			default:
				return assemblyline.PortableResult{}, fmt.Errorf("unexpected work kind %q", job.Kind)
			}
			return assemblyline.PortableResult{JobID: job.ID, Candidate: candidate}, nil
		},
	}
	resolution, err := resolveApplicationRequirementQueueFixture(t, runtime, request)
	if err != nil {
		t.Fatal(err)
	}
	if got := applicationRequirementStatements(resolution); !reflect.DeepEqual(got, children) {
		t.Fatalf("requirements=%v want=%v", got, children)
	}
	want := map[assemblyline.WorkKind]int{
		assemblyline.WorkApplicationProductContext:                      1,
		assemblyline.WorkApplicationRequirementInventory:                1,
		assemblyline.WorkApplicationRequirementCandidateKind:            6,
		assemblyline.WorkApplicationRequirementCandidateCardinality:     3,
		assemblyline.WorkApplicationRequirementCandidatePartition:       1,
		assemblyline.WorkApplicationRequirementCandidateAuthorization:   3,
		assemblyline.WorkApplicationRequirementCandidateOutcomeRelation: 1,
		assemblyline.WorkApplicationRequirementCandidateResultRelation:  2,
	}
	if !reflect.DeepEqual(counts, want) {
		t.Fatalf("calls=%v want=%v", counts, want)
	}
}

func TestApplicationRequirementQueuePreservesRuntimeChildOfMixedClause(t *testing.T) {
	t.Parallel()
	const request = "Build a browser status display using React."
	const parent = "Build a browser status display using React."
	const runtimeChild = "Display the current status."
	const nonRuntimeChild = "Use React."
	var authorizationCandidates []string
	runtime := typedWorkerRuntime{
		Context: context.Background(), MaxAttempts: 1,
		Execute: func(job assemblyline.PortableJob, _ string) (assemblyline.PortableResult, error) {
			candidate := ""
			switch job.Kind {
			case assemblyline.WorkApplicationProductContext:
				candidate = "A browser status display."
			case assemblyline.WorkApplicationRequirementInventory:
				candidate = parent
			case assemblyline.WorkApplicationRequirementCandidateKind:
				leaf, err := applicationRequirementKindCandidate(job)
				if err != nil {
					return assemblyline.PortableResult{}, err
				}
				kind := assemblyline.ApplicationRequirementCandidateTaskLocal
				switch leaf {
				case parent:
					kind = assemblyline.ApplicationRequirementCandidateMixed
				case nonRuntimeChild:
					kind = assemblyline.ApplicationRequirementCandidateNonRuntime
				}
				candidate, err = applicationRequirementCandidateContentPresenceForKindForTest(job, kind)
				if err != nil {
					return assemblyline.PortableResult{}, err
				}
			case assemblyline.WorkApplicationRequirementCandidatePartition:
				candidate = runtimeChild + "\n" + nonRuntimeChild
			case assemblyline.WorkApplicationRequirementCandidateCardinality:
				candidate = assemblyline.ApplicationRequirementOneRuntimeOutcome
			case assemblyline.WorkApplicationRequirementCandidateAuthorization:
				leaf, err := applicationRequirementAuthorizationCandidate(job)
				if err != nil {
					return assemblyline.PortableResult{}, err
				}
				authorizationCandidates = append(authorizationCandidates, leaf)
				candidate = assemblyline.ApplicationRequirementCandidateEntailed
			case assemblyline.WorkApplicationRequirementCandidateResultRelation:
				presence, presenceErr := applicationRequirementCandidateResultPresenceForRelationForTest(
					job, assemblyline.ApplicationRequirementNoDerivedResult,
				)
				if presenceErr != nil {
					return assemblyline.PortableResult{}, presenceErr
				}
				candidate = presence
			default:
				return assemblyline.PortableResult{}, fmt.Errorf("unexpected work kind %q", job.Kind)
			}
			return assemblyline.PortableResult{JobID: job.ID, Candidate: candidate}, nil
		},
	}
	resolution, err := resolveApplicationRequirementQueueFixture(t, runtime, request)
	if err != nil {
		t.Fatal(err)
	}
	if got := applicationRequirementStatements(resolution); !reflect.DeepEqual(got, []string{runtimeChild}) {
		t.Fatalf("requirements=%v", got)
	}
	if want := []string{runtimeChild, nonRuntimeChild}; !reflect.DeepEqual(authorizationCandidates, want) {
		t.Fatalf("authorized candidates=%v want=%v", authorizationCandidates, want)
	}
}

func TestApplicationRequirementQueueSkipsSemanticAuthorizationForExactRequest(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		name           string
		request        string
		productContext string
	}{
		{
			name:           "display",
			request:        "Display the submitted note.",
			productContext: "A submitted-note display.",
		},
		{
			name:           "persistence",
			request:        "Persist the submitted preference.",
			productContext: "A preference store.",
		},
	} {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			counts := map[assemblyline.WorkKind]int{}
			runtime := typedWorkerRuntime{
				Context: context.Background(), MaxAttempts: 1,
				Execute: func(job assemblyline.PortableJob, _ string) (assemblyline.PortableResult, error) {
					counts[job.Kind]++
					candidate := ""
					switch job.Kind {
					case assemblyline.WorkApplicationProductContext:
						candidate = test.productContext
					case assemblyline.WorkApplicationRequirementInventory:
						candidate = test.request
					case assemblyline.WorkApplicationRequirementCandidateAuthorization:
						return assemblyline.PortableResult{}, fmt.Errorf(
							"exact request candidate reached semantic authorization",
						)
					case assemblyline.WorkApplicationRequirementCandidateKind:
						var err error
						candidate, err = applicationRequirementCandidateContentPresenceForKindForTest(
							job,
							assemblyline.ApplicationRequirementCandidateTaskLocal,
						)
						if err != nil {
							return assemblyline.PortableResult{}, err
						}
					case assemblyline.WorkApplicationRequirementCandidateCardinality:
						candidate = assemblyline.ApplicationRequirementOneRuntimeOutcome
					case assemblyline.WorkApplicationRequirementCandidateResultRelation:
						presence, presenceErr := applicationRequirementCandidateResultPresenceForRelationForTest(
							job, assemblyline.ApplicationRequirementNoDerivedResult,
						)
						if presenceErr != nil {
							return assemblyline.PortableResult{}, presenceErr
						}
						candidate = presence
					default:
						return assemblyline.PortableResult{}, fmt.Errorf(
							"unexpected work kind %q",
							job.Kind,
						)
					}
					return assemblyline.PortableResult{JobID: job.ID, Candidate: candidate}, nil
				},
			}
			resolution, err := resolveApplicationRequirementQueueFixture(t, runtime, test.request)
			if err != nil {
				t.Fatal(err)
			}
			if got := applicationRequirementStatements(resolution); !reflect.DeepEqual(got, []string{test.request}) {
				t.Fatalf("requirements=%v", got)
			}
			want := map[assemblyline.WorkKind]int{
				assemblyline.WorkApplicationProductContext:                     1,
				assemblyline.WorkApplicationRequirementInventory:               1,
				assemblyline.WorkApplicationRequirementCandidateKind:           2,
				assemblyline.WorkApplicationRequirementCandidateCardinality:    1,
				assemblyline.WorkApplicationRequirementCandidateResultRelation: 1,
			}
			if !reflect.DeepEqual(counts, want) {
				t.Fatalf("calls=%v want=%v", counts, want)
			}
		})
	}
}

func resolveApplicationRequirementQueueFixture(
	t *testing.T,
	runtime typedWorkerRuntime,
	request string,
) (assemblyline.ApplicationIntentResolution, error) {
	t.Helper()
	applicationContext, err := assemblyline.BootstrapApplicationContext(
		request,
		assemblyline.ApplicationWorkspaceEmpty,
	)
	if err != nil {
		t.Fatal(err)
	}
	return resolveDirectCodingApplicationIntent(
		runtime,
		"intent-model",
		assemblyline.ApplicationIntentInput{UserRequest: request, Context: applicationContext},
		nil,
	)
}

func applicationRequirementStatements(
	resolution assemblyline.ApplicationIntentResolution,
) []string {
	statements := make([]string, len(resolution.Requirements))
	for index, requirement := range resolution.Requirements {
		statements[index] = requirement.Statement
	}
	return statements
}

func applicationRequirementKindCandidate(job assemblyline.PortableJob) (string, error) {
	input, err := applicationRequirementCandidateContentPresenceInputForTest(job)
	if err != nil {
		return "", err
	}
	return input.Candidate, nil
}

func applicationRequirementCardinalityCandidate(job assemblyline.PortableJob) (string, error) {
	var input assemblyline.ApplicationRequirementCandidateCardinalityInput
	if err := json.Unmarshal(job.Payload, &input); err != nil {
		return "", err
	}
	return input.Candidate, nil
}

func applicationRequirementAuthorizationCandidate(job assemblyline.PortableJob) (string, error) {
	var input assemblyline.ApplicationRequirementCandidateAuthorizationInput
	if err := json.Unmarshal(job.Payload, &input); err != nil {
		return "", err
	}
	return input.Candidate, nil
}
