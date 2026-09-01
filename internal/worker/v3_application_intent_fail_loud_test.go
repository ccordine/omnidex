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

func TestApplicationIntentValidNegativeDoesNotStopIndependentCandidate(t *testing.T) {
	t.Parallel()
	const request = "Build an image resizer."
	const addedMechanism = "The finished software resizes images through a device camera."
	const coreOutcome = "The finished software resizes images."
	applicationContext, err := assemblyline.BootstrapApplicationContext(request)
	if err != nil {
		t.Fatal(err)
	}
	authority := assemblyline.ApplicationIntentInput{
		UserRequest: request,
		Context:     applicationContext,
	}
	var authorizationSubjects []string
	var scopeModes []model.CodingScopeMode
	runtime := typedWorkerRuntime{
		Context: context.Background(),
		Execute: func(job assemblyline.PortableJob, _ string) (assemblyline.PortableResult, error) {
			candidate := ""
			switch job.Kind {
			case assemblyline.WorkApplicationRequirementInventory:
				candidate = addedMechanism + "\n" + coreOutcome
			case assemblyline.WorkApplicationRequirementCandidateAuthorization:
				var input assemblyline.ApplicationRequirementCandidateAuthorizationInput
				if err := json.Unmarshal(job.Payload, &input); err != nil {
					return assemblyline.PortableResult{}, err
				}
				authorizationSubjects = append(authorizationSubjects, input.Candidate)
				candidate = "A"
				if input.Candidate == addedMechanism {
					candidate = "B"
				}
			case assemblyline.WorkApplicationRequirementCandidateScopeRelation:
				var input assemblyline.ApplicationRequirementCandidateScopeRelationInput
				if err := json.Unmarshal(job.Payload, &input); err != nil {
					return assemblyline.PortableResult{}, err
				}
				scopeModes = append(scopeModes, input.ScopeMode)
				candidate = "C"
			case assemblyline.WorkApplicationRequirementCandidateKind:
				var input assemblyline.ApplicationRequirementCandidateContentPresenceInput
				if err := json.Unmarshal(job.Payload, &input); err != nil {
					return assemblyline.PortableResult{}, err
				}
				candidate = "A"
				if input.Dimension == assemblyline.ApplicationRequirementCandidateNonRuntimeContentDimension {
					candidate = "B"
				}
			case assemblyline.WorkApplicationRequirementCandidateCardinality:
				candidate = "A"
			case assemblyline.WorkApplicationRequirementCandidateResultRelation:
				candidate = "A"
			case assemblyline.WorkApplicationRequirementCandidateOutcomeRelation:
				candidate = "B"
			case assemblyline.WorkApplicationProductContext:
				candidate = "image resizer"
			default:
				return assemblyline.PortableResult{}, fmt.Errorf("unexpected work kind %q", job.Kind)
			}
			return assemblyline.PortableResult{JobID: job.ID, Candidate: candidate}, nil
		},
	}

	proposals, err := resolveDirectCodingApplicationPlan(
		runtime,
		directCodingApplicationIntentModels{
			Requirements: "requirements-model", ResultRelation: "result-model",
		},
		authority,
		model.CodingScopeModeExpansive,
		nil,
	)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(authorizationSubjects, []string{addedMechanism, coreOutcome}) {
		t.Fatalf("authorization subjects=%q", authorizationSubjects)
	}
	if !reflect.DeepEqual(scopeModes, []model.CodingScopeMode{model.CodingScopeModeExpansive}) {
		t.Fatalf("scope critic modes=%q", scopeModes)
	}
	if len(proposals) != 2 ||
		proposals[0].Annotation != model.CodingPlanAnnotationConcreteConflict ||
		proposals[0].Statement != addedMechanism || proposals[1].Statement != coreOutcome {
		t.Fatalf("proposals=%+v", proposals)
	}
}

func TestApplicationIntentMalformedCandidateStationFailsLoudly(t *testing.T) {
	t.Parallel()
	const request = "Build a barcode scanner."
	applicationContext, err := assemblyline.BootstrapApplicationContext(request)
	if err != nil {
		t.Fatal(err)
	}
	runtime := typedWorkerRuntime{
		Context: context.Background(),
		Execute: func(job assemblyline.PortableJob, _ string) (assemblyline.PortableResult, error) {
			candidate := "The finished software scans barcodes."
			if job.Kind == assemblyline.WorkApplicationRequirementCandidateAuthorization {
				candidate = "MALFORMED_AUTHORIZATION_RELATION"
			}
			return assemblyline.PortableResult{JobID: job.ID, Candidate: candidate}, nil
		},
	}
	_, err = resolveDirectCodingApplicationPlan(
		runtime,
		directCodingApplicationIntentModels{
			Requirements: "requirements-model", ResultRelation: "result-model",
		},
		assemblyline.ApplicationIntentInput{
			UserRequest: request,
			Context:     applicationContext,
		},
		model.CodingScopeModeNormal,
		nil,
	)
	if err == nil || !strings.Contains(
		err.Error(),
		"invalid application_requirement_candidate_authorization semantic result",
	) {
		t.Fatalf("malformed authorization error=%v", err)
	}
}

func TestArtifactHandlingMalformedStationFailsLoudly(t *testing.T) {
	t.Parallel()
	runtime := malformedSemanticLeafRuntime("MALFORMED_ARTIFACT_RELATION")
	_, err := classifyArtifactHandling(
		runtime,
		"artifact-model",
		"Preserve ARTIFACT_1.",
		[]assemblyline.ArtifactIdentity{{Token: "ARTIFACT_1", Value: "/opaque/source"}},
	)
	if err == nil || !strings.Contains(err.Error(), "invalid artifact_handling semantic result") {
		t.Fatalf("malformed artifact relation error=%v", err)
	}
}

func TestCapabilityRelationMalformedStationFailsLoudly(t *testing.T) {
	t.Parallel()
	pair := directCodingCapabilityPair{
		LeftIndex:  0,
		RightIndex: 1,
		Input: assemblyline.CapabilityRelationInput{
			LocalContext: "record processor",
			LeftNeed:     "The finished software transforms supplied records.",
			RightNeed:    "The finished software reports transformed records.",
		},
	}
	results := runDirectCodingCapabilityPairs(
		malformedSemanticLeafRuntime("MALFORMED_CAPABILITY_RELATION"),
		"capability-model",
		[]directCodingCapabilityPair{pair},
		1,
	)
	if len(results) != 1 || results[0].Err == nil || !strings.Contains(
		results[0].Err.Error(),
		"invalid capability_relation_001_002 semantic result",
	) {
		t.Fatalf("malformed capability results=%+v", results)
	}
}

func malformedSemanticLeafRuntime(candidate string) typedWorkerRuntime {
	return typedWorkerRuntime{
		Context: context.Background(),
		Execute: func(job assemblyline.PortableJob, _ string) (assemblyline.PortableResult, error) {
			return assemblyline.PortableResult{JobID: job.ID, Candidate: candidate}, nil
		},
	}
}
