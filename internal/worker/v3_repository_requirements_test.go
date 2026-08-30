package worker

import (
	"context"
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/assemblyline"
)

func TestExistingRepositoryRequirementsUseOneInventoryAndCodeOwnedCandidateQueue(
	t *testing.T,
) {
	t.Parallel()
	const request = "Add audit logging. The service is old. Add CSV export."
	calls := make([]assemblyline.WorkKind, 0, 5)
	authorizationCandidates := make([]string, 0, 3)
	runtime := typedWorkerRuntime{
		Context:     context.Background(),
		MaxAttempts: 3,
		Execute: func(
			job assemblyline.PortableJob,
			model string,
		) (assemblyline.PortableResult, error) {
			calls = append(calls, job.Kind)
			if model != "qwen-stable" {
				t.Fatalf("model=%q kind=%q", model, job.Kind)
			}
			switch job.Kind {
			case assemblyline.WorkRepositoryRequirementInventory:
				return assemblyline.PortableResult{
					JobID: job.ID,
					Candidate: strings.Join([]string{
						"Add audit logging.",
						"Add audit logging.",
						"The service is old.",
						"Add CSV export.",
					}, "\n"),
				}, nil
			case assemblyline.WorkRepositoryRequirementCandidateAuthorization:
				var input assemblyline.RepositoryRequirementCandidateAuthorizationInput
				if err := json.Unmarshal(job.Payload, &input); err != nil {
					t.Fatal(err)
				}
				candidate := input.Inventory.Candidates[input.CandidateIndex]
				authorizationCandidates = append(authorizationCandidates, candidate)
				relation := assemblyline.RepositoryRequirementCandidateRequiresChange
				if candidate == "The service is old." {
					relation = assemblyline.RepositoryRequirementCandidateNoChange
				}
				return assemblyline.PortableResult{
					JobID: job.ID, Candidate: relation,
				}, nil
			case assemblyline.WorkRepositoryRequirementCandidateRelation:
				return assemblyline.PortableResult{
					JobID:     job.ID,
					Candidate: assemblyline.RepositoryRequirementCandidatesDistinctChanges,
				}, nil
			default:
				t.Fatalf("unexpected work kind=%q", job.Kind)
				return assemblyline.PortableResult{}, nil
			}
		},
	}
	applicationContext, err := assemblyline.BootstrapApplicationContext(
		request,
		assemblyline.ApplicationWorkspaceExisting,
	)
	if err != nil {
		t.Fatal(err)
	}
	requirements, err := interpretRepositoryRequirements(
		runtime,
		"qwen-stable",
		request,
		applicationContext,
		nil,
	)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(requirements, []string{
		"Add audit logging.",
		"Add CSV export.",
	}) {
		t.Fatalf("requirements=%q", requirements)
	}
	if !reflect.DeepEqual(authorizationCandidates, []string{
		"Add audit logging.",
		"The service is old.",
		"Add CSV export.",
	}) {
		t.Fatalf("candidate authorization calls=%q", authorizationCandidates)
	}
	wantCalls := []assemblyline.WorkKind{
		assemblyline.WorkRepositoryRequirementInventory,
		assemblyline.WorkRepositoryRequirementCandidateAuthorization,
		assemblyline.WorkRepositoryRequirementCandidateAuthorization,
		assemblyline.WorkRepositoryRequirementCandidateAuthorization,
		assemblyline.WorkRepositoryRequirementCandidateRelation,
	}
	if !reflect.DeepEqual(calls, wantCalls) {
		t.Fatalf("calls=%q want=%q", calls, wantCalls)
	}
}

func TestRepositoryRequirementQueueClosesOnlyByExhaustionAndSkipsOverlap(
	t *testing.T,
) {
	t.Parallel()
	const request = "Add audit logging. The service is old. Add CSV export."
	authorizationCalls := 0
	pairCalls := 0
	runtime := typedWorkerRuntime{
		Context:     context.Background(),
		MaxAttempts: 3,
		Execute: func(
			job assemblyline.PortableJob,
			_ string,
		) (assemblyline.PortableResult, error) {
			switch job.Kind {
			case assemblyline.WorkRepositoryRequirementInventory:
				return assemblyline.PortableResult{
					JobID: job.ID,
					Candidate: strings.Join([]string{
						"Add audit logging.",
						"Add audit logging. The service is old.",
						"Add CSV export.",
					}, "\n"),
				}, nil
			case assemblyline.WorkRepositoryRequirementCandidateAuthorization:
				authorizationCalls++
				return assemblyline.PortableResult{
					JobID:     job.ID,
					Candidate: assemblyline.RepositoryRequirementCandidateRequiresChange,
				}, nil
			case assemblyline.WorkRepositoryRequirementCandidateRelation:
				pairCalls++
				return assemblyline.PortableResult{
					JobID:     job.ID,
					Candidate: assemblyline.RepositoryRequirementCandidatesDistinctChanges,
				}, nil
			default:
				t.Fatalf("unexpected work kind=%q", job.Kind)
				return assemblyline.PortableResult{}, nil
			}
		},
	}
	applicationContext, err := assemblyline.BootstrapApplicationContext(
		request,
		assemblyline.ApplicationWorkspaceExisting,
	)
	if err != nil {
		t.Fatal(err)
	}
	requirements, err := interpretRepositoryRequirements(
		runtime,
		"qwen-stable",
		request,
		applicationContext,
		nil,
	)
	if err != nil {
		t.Fatal(err)
	}
	if authorizationCalls != 2 || pairCalls != 1 || !reflect.DeepEqual(requirements, []string{
		"Add audit logging.",
		"Add CSV export.",
	}) {
		t.Fatalf(
			"authorization calls=%d pair calls=%d requirements=%q",
			authorizationCalls,
			pairCalls,
			requirements,
		)
	}
}

func TestRepositoryRequirementQueueDropsSemanticParaphraseWithoutRevisitingAccepted(
	t *testing.T,
) {
	t.Parallel()
	const request = "Add audit logging. Include an audit trail. Add CSV export."
	pairCalls := 0
	runtime := typedWorkerRuntime{
		Context:     context.Background(),
		MaxAttempts: 3,
		Execute: func(
			job assemblyline.PortableJob,
			_ string,
		) (assemblyline.PortableResult, error) {
			switch job.Kind {
			case assemblyline.WorkRepositoryRequirementInventory:
				return assemblyline.PortableResult{
					JobID: job.ID,
					Candidate: strings.Join([]string{
						"Add audit logging.",
						"Include an audit trail.",
						"Add CSV export.",
					}, "\n"),
				}, nil
			case assemblyline.WorkRepositoryRequirementCandidateAuthorization:
				return assemblyline.PortableResult{
					JobID:     job.ID,
					Candidate: assemblyline.RepositoryRequirementCandidateRequiresChange,
				}, nil
			case assemblyline.WorkRepositoryRequirementCandidateRelation:
				pairCalls++
				var input assemblyline.RepositoryRequirementCandidateRelationInput
				if err := json.Unmarshal(job.Payload, &input); err != nil {
					t.Fatal(err)
				}
				relation := assemblyline.RepositoryRequirementCandidatesDistinctChanges
				if input.Candidate == "Include an audit trail." {
					relation = assemblyline.RepositoryRequirementCandidatesSameChange
				}
				return assemblyline.PortableResult{JobID: job.ID, Candidate: relation}, nil
			default:
				t.Fatalf("unexpected work kind=%q", job.Kind)
				return assemblyline.PortableResult{}, nil
			}
		},
	}
	applicationContext, err := assemblyline.BootstrapApplicationContext(
		request,
		assemblyline.ApplicationWorkspaceExisting,
	)
	if err != nil {
		t.Fatal(err)
	}
	requirements, err := interpretRepositoryRequirements(
		runtime,
		"qwen-stable",
		request,
		applicationContext,
		nil,
	)
	if err != nil {
		t.Fatal(err)
	}
	if pairCalls != 2 || !reflect.DeepEqual(requirements, []string{
		"Add audit logging.",
		"Add CSV export.",
	}) {
		t.Fatalf("pair calls=%d requirements=%q", pairCalls, requirements)
	}
}

func TestRepositoryRequirementQueueFailsWhenNoCandidateRequiresChange(
	t *testing.T,
) {
	t.Parallel()
	const request = "The service is old."
	runtime := typedWorkerRuntime{
		Context:     context.Background(),
		MaxAttempts: 3,
		Execute: func(
			job assemblyline.PortableJob,
			_ string,
		) (assemblyline.PortableResult, error) {
			candidate := "The service is old."
			if job.Kind == assemblyline.WorkRepositoryRequirementCandidateAuthorization {
				candidate = assemblyline.RepositoryRequirementCandidateNoChange
			}
			return assemblyline.PortableResult{JobID: job.ID, Candidate: candidate}, nil
		},
	}
	applicationContext, err := assemblyline.BootstrapApplicationContext(
		request,
		assemblyline.ApplicationWorkspaceExisting,
	)
	if err != nil {
		t.Fatal(err)
	}
	_, err = interpretRepositoryRequirements(
		runtime,
		"qwen-stable",
		request,
		applicationContext,
		nil,
	)
	if err == nil || !strings.Contains(err.Error(), "queue exhausted") {
		t.Fatalf("error=%v", err)
	}
}

func TestRepositoryRequirementInventoryAndCandidateRelationReplay(t *testing.T) {
	t.Parallel()
	const request = "Add audit logging."
	applicationContext, err := assemblyline.BootstrapApplicationContext(
		request,
		assemblyline.ApplicationWorkspaceExisting,
	)
	if err != nil {
		t.Fatal(err)
	}
	authority := assemblyline.RepositoryRequirementInterpretationInput{
		UserRequest: request,
		Context:     applicationContext,
	}
	inventoryJob, err := assemblyline.NewRepositoryRequirementInventoryJob(authority)
	if err != nil {
		t.Fatal(err)
	}
	if handled, err := replayRepositorySemanticLeaf(
		inventoryJob,
		request,
	); err != nil || !handled {
		t.Fatalf("inventory replay handled=%v error=%v", handled, err)
	}
	inventory, err := assemblyline.DecodeRepositoryRequirementInventory(authority, request)
	if err != nil {
		t.Fatal(err)
	}
	authorizationJob, err := assemblyline.NewRepositoryRequirementCandidateAuthorizationJob(
		assemblyline.RepositoryRequirementCandidateAuthorizationInput{
			Authority: authority, Inventory: inventory, CandidateIndex: 0,
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if handled, err := replayRepositorySemanticLeaf(
		authorizationJob,
		assemblyline.RepositoryRequirementCandidateRequiresChange,
	); err != nil || !handled {
		t.Fatalf("candidate authorization replay handled=%v error=%v", handled, err)
	}
	pairInput := assemblyline.RepositoryRequirementCandidateRelationInput{
		Candidate: "Include an audit trail.", AcceptedRequirement: request,
	}
	pairJob, err := assemblyline.NewRepositoryRequirementCandidateRelationJob(pairInput)
	if err != nil {
		t.Fatal(err)
	}
	if handled, err := replayRepositorySemanticLeaf(
		pairJob,
		assemblyline.RepositoryRequirementCandidatesSameChange,
	); err != nil || !handled {
		t.Fatalf("candidate relation replay handled=%v error=%v", handled, err)
	}
}
