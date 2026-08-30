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

func TestApplicationFrontDoorSkipsCeremonialReviewForEmptyWorkspace(t *testing.T) {
	t.Parallel()
	const request = "Build a browser counter in ARTIFACT_1 that shows the count and can increment, decrement, and reset it."
	const authoritativeRequest = "Build a browser counter in ui/private-counter.ts that shows the count and can increment, decrement, and reset it."
	const productContext = "Resolved counter product identity."
	applicationContext, err := assemblyline.BootstrapApplicationContext(
		request, assemblyline.ApplicationWorkspaceEmpty,
	)
	if err != nil {
		t.Fatal(err)
	}
	requestAuthority, err := newDirectCodingApplicationRequestAuthority(
		authoritativeRequest, request,
	)
	if err != nil {
		t.Fatal(err)
	}
	counts := make(map[assemblyline.WorkKind]int)
	runtime := typedWorkerRuntime{
		Context: context.Background(), MaxAttempts: 3,
		Execute: func(job assemblyline.PortableJob, modelName string) (assemblyline.PortableResult, error) {
			prompt, err := assemblyline.RenderPortableJob(job)
			if err != nil {
				return assemblyline.PortableResult{}, err
			}
			if strings.Contains(prompt, "ui/private-counter.ts") ||
				strings.Contains(string(job.Payload), "ui/private-counter.ts") {
				return assemblyline.PortableResult{}, fmt.Errorf(
					"semantic station received the unredacted authoritative request",
				)
			}
			if (job.Kind == assemblyline.WorkApplicationRequirementCoverage ||
				job.Kind == assemblyline.WorkApplicationRequirement) &&
				(strings.Contains(prompt, productContext) ||
					strings.Contains(prompt, "PRODUCT CONTEXT:")) {
				return assemblyline.PortableResult{}, fmt.Errorf(
					"requirement station received redundant product context",
				)
			}
			counts[job.Kind]++
			var candidate string
			switch job.Kind {
			case assemblyline.WorkApplicationClassify:
				candidate = string(assemblyline.ApplicationSurfaceBrowser)
			case assemblyline.WorkApplicationProductContext:
				candidate = productContext
			case assemblyline.WorkApplicationRequirementCoverage:
				var input assemblyline.ApplicationRequirementCoverageInput
				if err := json.Unmarshal(job.Payload, &input); err != nil {
					return assemblyline.PortableResult{}, err
				}
				if input.AcceptedRequirements == nil {
					return assemblyline.PortableResult{}, fmt.Errorf("application requirement coverage received a nil accepted set")
				}
				if input.ExcludedCandidates == nil {
					return assemblyline.PortableResult{}, fmt.Errorf("application requirement coverage received a nil excluded set")
				}
				if input.ZeroDeltas == nil {
					return assemblyline.PortableResult{}, fmt.Errorf("application requirement coverage received a nil zero-delta set")
				}
				if len(input.AcceptedRequirements) < 4 {
					candidate = assemblyline.ApplicationRequirementRemains
				} else {
					candidate = assemblyline.ApplicationNoUncoveredRequirement
				}
			case assemblyline.WorkApplicationRequirement:
				var input assemblyline.ApplicationRequirementCandidateInput
				if err := json.Unmarshal(job.Payload, &input); err != nil {
					return assemblyline.PortableResult{}, err
				}
				if input.Authority.AcceptedRequirements == nil {
					return assemblyline.PortableResult{}, fmt.Errorf("application requirement received a nil accepted set")
				}
				if err := input.Coverage.ValidateFor(input.Authority); err != nil ||
					input.Coverage.Relation != assemblyline.ApplicationRequirementRemains {
					return assemblyline.PortableResult{}, fmt.Errorf("application requirement received invalid coverage authority: %v", err)
				}
				candidate = []string{
					"Show the current count.",
					"Increment the current count.",
					"Decrement the current count.",
					"Reset the current count.",
				}[counts[job.Kind]-1]
			case assemblyline.WorkApplicationRequirementCandidateKind:
				var input assemblyline.ApplicationRequirementCandidateKindInput
				if err := json.Unmarshal(job.Payload, &input); err != nil {
					return assemblyline.PortableResult{}, err
				}
				if strings.TrimSpace(input.Candidate) == "" {
					return assemblyline.PortableResult{}, fmt.Errorf("application requirement kind received an empty candidate")
				}
				candidate = assemblyline.ApplicationRequirementCandidateTaskLocal
			case assemblyline.WorkApplicationRequirementCandidateCardinality:
				var input assemblyline.ApplicationRequirementCandidateCardinalityInput
				if err := json.Unmarshal(job.Payload, &input); err != nil {
					return assemblyline.PortableResult{}, err
				}
				if strings.TrimSpace(input.Candidate) == "" {
					return assemblyline.PortableResult{}, fmt.Errorf("application requirement cardinality received an empty candidate")
				}
				candidate = assemblyline.ApplicationRequirementOneRuntimeOutcome
			case assemblyline.WorkApplicationRequirementCandidateResultRelation:
				candidate = assemblyline.ApplicationRequirementNoDerivedResult
			case assemblyline.WorkApplicationRequirementCandidateOutcomeRelation:
				candidate = assemblyline.ApplicationRequirementDistinctRuntimeOutcomes
			case assemblyline.WorkApplicationRequirementCandidateSplit:
				return assemblyline.PortableResult{}, fmt.Errorf("atomic fixture unexpectedly requested candidate splitting")
			default:
				return assemblyline.PortableResult{}, fmt.Errorf("unexpected semantic work kind %q", job.Kind)
			}
			return assemblyline.PortableResult{JobID: job.ID, Candidate: candidate}, nil
		},
	}
	interpretation, err := runDirectCodingApplicationInterpreter(
		runtime, "intent-model", "surface-model", "artifact-model",
		requestAuthority, applicationContext, nil,
	)
	if err != nil {
		t.Fatal(err)
	}
	specification := interpretation.Specification
	if counts[assemblyline.WorkApplicationContextNeedCoverage] != 0 ||
		counts[assemblyline.WorkApplicationProductContext] != 1 ||
		counts[assemblyline.WorkApplicationRequirementCoverage] != 5 ||
		counts[assemblyline.WorkApplicationRequirement] != 4 ||
		counts[assemblyline.WorkApplicationRequirementCandidateKind] != 4 ||
		counts[assemblyline.WorkApplicationRequirementCandidateCardinality] != 4 ||
		counts[assemblyline.WorkApplicationRequirementCandidateOutcomeRelation] != 6 ||
		counts[assemblyline.WorkApplicationRequirementCandidateResultRelation] != 4 ||
		counts[assemblyline.WorkApplicationRequirementCandidateResultRelationGrounding] != 0 ||
		counts[assemblyline.WorkApplicationRequirementCandidateResultRelationCorrection] != 0 ||
		counts[assemblyline.WorkApplicationRequirementCandidateSplit] != 0 ||
		counts[assemblyline.WorkApplicationRequirementCandidateSplitCorrection] != 0 ||
		counts[assemblyline.WorkApplicationClassify] != 1 {
		t.Fatalf("front-door calls=%v", counts)
	}
	want := []assemblyline.Requirement{
		{ID: "requirement_001", SourceQuote: "Show the current count."},
		{ID: "requirement_002", SourceQuote: "Increment the current count."},
		{ID: "requirement_003", SourceQuote: "Decrement the current count."},
		{ID: "requirement_004", SourceQuote: "Reset the current count."},
	}
	if specification.ProductQuote != productContext ||
		!reflect.DeepEqual(specification.Requirements, want) {
		t.Fatalf("specification=%+v", specification)
	}
	if len(interpretation.AcceptedRequirements) != len(want) {
		t.Fatalf("accepted requirement receipts=%+v", interpretation.AcceptedRequirements)
	}
	if interpretation.RequestSHA256 != assemblyline.ExactObjectiveContextSHA(authoritativeRequest) {
		t.Fatalf("interpretation request provenance=%q", interpretation.RequestSHA256)
	}
	for _, accepted := range interpretation.AcceptedRequirements {
		if accepted.RequestSHA256 != assemblyline.ExactObjectiveContextSHA(authoritativeRequest) ||
			accepted.ResultRelation.Relation != assemblyline.ApplicationRequirementNoDerivedResult ||
			accepted.ResultRelation.CandidateSHA256 != assemblyline.ExactObjectiveContextSHA(accepted.Statement) {
			t.Fatalf("accepted requirement lost result-relation receipt: %+v", accepted)
		}
	}
}

func TestApplicationFrontDoorFailsLoudlyWhenEvidenceNeedsAreUnresolved(t *testing.T) {
	t.Parallel()
	const request = "Extend the existing application with the established reporting behavior."
	applicationContext, err := assemblyline.BootstrapApplicationContext(
		request, assemblyline.ApplicationWorkspaceExisting,
	)
	if err != nil {
		t.Fatal(err)
	}
	requestAuthority, err := newDirectCodingApplicationRequestAuthority(request, request)
	if err != nil {
		t.Fatal(err)
	}
	coverageCalls := 0
	runtime := typedWorkerRuntime{
		Context: context.Background(), MaxAttempts: 1,
		Execute: testPortableExecutor(func(kind string, _ string, _ string) (string, error) {
			switch assemblyline.WorkKind(kind) {
			case assemblyline.WorkApplicationContextNeedCoverage:
				coverageCalls++
				if coverageCalls == 1 {
					return assemblyline.ApplicationContextNeedRemains, nil
				}
				return assemblyline.ApplicationNoUncoveredContextNeed, nil
			case assemblyline.WorkApplicationContextNeedQuestion:
				return "What verified behavior is meant by the established reporting behavior?", nil
			default:
				return "", fmt.Errorf("unexpected semantic work kind %q", kind)
			}
		}),
	}
	_, err = runDirectCodingApplicationInterpreter(
		runtime, "intent-model", "surface-model", "artifact-model",
		requestAuthority, applicationContext, nil,
	)
	if err == nil {
		t.Fatal("unresolved evidence need silently continued")
	}
}

func TestApplicationFrontDoorRejectsUnauthenticatedRequestBeforeSemanticWork(t *testing.T) {
	t.Parallel()
	const request = "Build a browser status display."
	applicationContext, err := assemblyline.BootstrapApplicationContext(
		request, assemblyline.ApplicationWorkspaceEmpty,
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
		runtime, "intent-model", "surface-model", "artifact-model",
		authority, applicationContext, nil,
	)
	if err == nil || !strings.Contains(err.Error(), "not authenticated") || calls != 0 {
		t.Fatalf("semantic calls=%d error=%v", calls, err)
	}
}
