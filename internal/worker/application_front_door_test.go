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

func TestApplicationFrontDoorUsesOneInventoryAndOneSievePassPerAcceptedLeaf(
	t *testing.T,
) {
	t.Parallel()
	const request = "Build a browser counter in ARTIFACT_1 that shows the count and can increment, decrement, and reset it."
	const authoritativeRequest = "Build a browser counter in ui/private-counter.ts that shows the count and can increment, decrement, and reset it."
	const productContext = "A browser counter."
	applicationContext, err := assemblyline.BootstrapApplicationContext(
		request,
		assemblyline.ApplicationWorkspaceEmpty,
	)
	if err != nil {
		t.Fatal(err)
	}
	requestAuthority, err := newDirectCodingApplicationRequestAuthority(
		authoritativeRequest,
		request,
	)
	if err != nil {
		t.Fatal(err)
	}
	leaves := []string{
		"Show the current count.",
		"Increment the current count.",
		"Decrement the current count.",
		"Reset the current count.",
	}
	counts := make(map[assemblyline.WorkKind]int)
	callOrder := make([]assemblyline.WorkKind, 0, 4)
	candidateCounts := make(map[string]map[assemblyline.WorkKind]int)
	runtime := typedWorkerRuntime{
		Context: context.Background(), MaxAttempts: 1,
		Execute: func(job assemblyline.PortableJob, _ string) (assemblyline.PortableResult, error) {
			prompt, err := assemblyline.RenderPortableJob(job)
			if err != nil {
				return assemblyline.PortableResult{}, err
			}
			if strings.Contains(prompt, "ui/private-counter.ts") ||
				strings.Contains(string(job.Payload), "ui/private-counter.ts") {
				return assemblyline.PortableResult{}, fmt.Errorf("semantic station received unredacted authority")
			}
			counts[job.Kind]++
			callOrder = append(callOrder, job.Kind)
			candidate := ""
			switch job.Kind {
			case assemblyline.WorkApplicationClassify:
				candidate = string(assemblyline.ApplicationSurfaceBrowser)
			case assemblyline.WorkApplicationProductContext:
				candidate = productContext
			case assemblyline.WorkApplicationRequirementInventory:
				candidate = strings.Join(leaves, "\n")
			case assemblyline.WorkApplicationRequirementCandidateKind:
				candidate, err = applicationRequirementCandidateContentPresenceForKindForTest(
					job,
					assemblyline.ApplicationRequirementCandidateTaskLocal,
				)
				if err != nil {
					return assemblyline.PortableResult{}, err
				}
				recordApplicationFrontDoorCandidateCall(t, candidateCounts, job)
			case assemblyline.WorkApplicationRequirementCandidateCardinality:
				candidate = assemblyline.ApplicationRequirementOneRuntimeOutcome
				recordApplicationFrontDoorCandidateCall(t, candidateCounts, job)
			case assemblyline.WorkApplicationRequirementCandidateAuthorization:
				var input assemblyline.ApplicationRequirementCandidateAuthorizationInput
				if err := json.Unmarshal(job.Payload, &input); err != nil {
					return assemblyline.PortableResult{}, err
				}
				if input.UserRequest != request || strings.Count(prompt, request) != 1 {
					return assemblyline.PortableResult{}, fmt.Errorf("authorization lost immutable request")
				}
				if candidateCounts[input.Candidate] == nil {
					candidateCounts[input.Candidate] = map[assemblyline.WorkKind]int{}
				}
				candidateCounts[input.Candidate][job.Kind]++
				candidate = assemblyline.ApplicationRequirementCandidateEntailed
			case assemblyline.WorkApplicationRequirementCandidateOutcomeRelation:
				candidate = assemblyline.ApplicationRequirementDistinctRuntimeOutcomes
			case assemblyline.WorkApplicationRequirementCandidateResultRelation:
				candidate = assemblyline.ApplicationRequirementNoDerivedResult
				recordApplicationFrontDoorCandidateCall(t, candidateCounts, job)
			default:
				return assemblyline.PortableResult{}, fmt.Errorf("unexpected semantic work kind %q", job.Kind)
			}
			return assemblyline.PortableResult{JobID: job.ID, Candidate: candidate}, nil
		},
	}
	interpretation, err := runDirectCodingApplicationInterpreter(
		runtime,
		"intent-model",
		"surface-model",
		"artifact-model",
		requestAuthority,
		applicationContext,
		nil,
	)
	if err != nil {
		t.Fatal(err)
	}
	wantCounts := map[assemblyline.WorkKind]int{
		assemblyline.WorkApplicationClassify:                            1,
		assemblyline.WorkApplicationProductContext:                      1,
		assemblyline.WorkApplicationRequirementInventory:                1,
		assemblyline.WorkApplicationRequirementCandidateKind:            8,
		assemblyline.WorkApplicationRequirementCandidateCardinality:     4,
		assemblyline.WorkApplicationRequirementCandidateAuthorization:   4,
		assemblyline.WorkApplicationRequirementCandidateOutcomeRelation: 6,
		assemblyline.WorkApplicationRequirementCandidateResultRelation:  4,
	}
	if !reflect.DeepEqual(counts, wantCounts) {
		t.Fatalf("front-door calls=%v want=%v", counts, wantCounts)
	}
	if len(callOrder) == 0 || callOrder[0] != assemblyline.WorkApplicationRequirementInventory {
		t.Fatalf("front-door first call=%v want inventory", callOrder)
	}
	firstResultRelation := applicationFrontDoorFirstCallIndex(
		callOrder,
		assemblyline.WorkApplicationRequirementCandidateResultRelation,
	)
	productIndex := applicationFrontDoorFirstCallIndex(
		callOrder,
		assemblyline.WorkApplicationProductContext,
	)
	surfaceIndex := applicationFrontDoorFirstCallIndex(
		callOrder,
		assemblyline.WorkApplicationClassify,
	)
	if firstResultRelation < 0 || productIndex <= firstResultRelation || surfaceIndex <= productIndex {
		t.Fatalf(
			"front-door order=%v want accepted leaf before product context before surface classification",
			callOrder,
		)
	}
	for _, leaf := range leaves {
		for _, kind := range []assemblyline.WorkKind{
			assemblyline.WorkApplicationRequirementCandidateKind,
			assemblyline.WorkApplicationRequirementCandidateCardinality,
			assemblyline.WorkApplicationRequirementCandidateAuthorization,
			assemblyline.WorkApplicationRequirementCandidateResultRelation,
		} {
			want := 1
			if kind == assemblyline.WorkApplicationRequirementCandidateKind {
				want = 2
			}
			if candidateCounts[leaf][kind] != want {
				t.Fatalf(
					"accepted leaf %q work %q calls=%d want=%d",
					leaf,
					kind,
					candidateCounts[leaf][kind],
					want,
				)
			}
		}
	}
	if interpretation.Specification.ProductQuote != productContext ||
		len(interpretation.AcceptedRequirements) != len(leaves) {
		t.Fatalf("interpretation=%+v", interpretation)
	}
	for index, accepted := range interpretation.AcceptedRequirements {
		if accepted.Statement != leaves[index] ||
			accepted.ResultRelation.Relation != assemblyline.ApplicationRequirementNoDerivedResult {
			t.Fatalf("accepted[%d]=%+v", index, accepted)
		}
	}
}

func applicationFrontDoorFirstCallIndex(
	calls []assemblyline.WorkKind,
	want assemblyline.WorkKind,
) int {
	for index, kind := range calls {
		if kind == want {
			return index
		}
	}
	return -1
}

func recordApplicationFrontDoorCandidateCall(
	t *testing.T,
	counts map[string]map[assemblyline.WorkKind]int,
	job assemblyline.PortableJob,
) {
	t.Helper()
	var candidate string
	switch job.Kind {
	case assemblyline.WorkApplicationRequirementCandidateKind:
		input, err := applicationRequirementCandidateContentPresenceInputForTest(job)
		if err != nil {
			t.Fatal(err)
		}
		candidate = input.Candidate
	case assemblyline.WorkApplicationRequirementCandidateCardinality:
		var input assemblyline.ApplicationRequirementCandidateCardinalityInput
		if err := json.Unmarshal(job.Payload, &input); err != nil {
			t.Fatal(err)
		}
		candidate = input.Candidate
	case assemblyline.WorkApplicationRequirementCandidateResultRelation:
		var input assemblyline.ApplicationRequirementCandidateResultRelationInput
		if err := json.Unmarshal(job.Payload, &input); err != nil {
			t.Fatal(err)
		}
		candidate = input.Candidate
	default:
		t.Fatalf("unregistered candidate-count work %q", job.Kind)
	}
	if counts[candidate] == nil {
		counts[candidate] = map[assemblyline.WorkKind]int{}
	}
	counts[candidate][job.Kind]++
}

func TestApplicationFrontDoorRejectsUnauthenticatedRequestBeforeSemanticWork(t *testing.T) {
	t.Parallel()
	const request = "Build a browser status display."
	applicationContext, err := assemblyline.BootstrapApplicationContext(
		request,
		assemblyline.ApplicationWorkspaceEmpty,
	)
	if err != nil {
		t.Fatal(err)
	}
	authority, err := newDirectCodingApplicationRequestAuthority(request, request)
	if err != nil {
		t.Fatal(err)
	}
	authority.requestSHA256 = strings.Repeat("a", 64)
	calls := 0
	runtime := typedWorkerRuntime{
		Context: context.Background(), MaxAttempts: 1,
		Execute: func(job assemblyline.PortableJob, _ string) (assemblyline.PortableResult, error) {
			calls++
			return assemblyline.PortableResult{JobID: job.ID}, nil
		},
	}
	_, err = runDirectCodingApplicationInterpreter(
		runtime,
		"intent-model",
		"surface-model",
		"artifact-model",
		authority,
		applicationContext,
		nil,
	)
	if err == nil || calls != 0 {
		t.Fatalf("unauthenticated request error=%v semantic calls=%d", err, calls)
	}
}
