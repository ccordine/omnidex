package worker

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/assemblyline"
)

func TestDeploymentDispositionRunsOnlyRequiredBoundedStations(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name            string
		availabilityRaw string
		destinationRaw  string
		wantCalls       int
		wantDisposition assemblyline.ApplicationServiceDeploymentDisposition
		wantError       string
	}{
		{
			name: "availability not required",
			availabilityRaw: continuedAvailabilityResultJSON(
				assemblyline.ApplicationServiceAvailabilityNotRequiredCandidate,
			),
			wantCalls: 1, wantDisposition: assemblyline.ApplicationServiceDeploymentVerifyOnly,
		},
		{
			name: "build environment is explicit destination",
			availabilityRaw: continuedAvailabilityResultJSON(
				assemblyline.ApplicationServiceAvailabilityRequiredCandidate,
			),
			destinationRaw: persistenceDestinationResultJSON(
				assemblyline.ApplicationServiceBuildEnvironmentDestinationCandidate,
			),
			wantCalls: 2, wantDisposition: assemblyline.ApplicationServiceDeploymentPersistCurrentHost,
		},
		{
			name: "destination is not explicit build environment",
			availabilityRaw: continuedAvailabilityResultJSON(
				assemblyline.ApplicationServiceAvailabilityRequiredCandidate,
			),
			destinationRaw: persistenceDestinationResultJSON(
				assemblyline.ApplicationServiceBuildEnvironmentNotEstablishedCandidate,
			),
			wantCalls: 2, wantError: "outside the registered current-host authority",
		},
		{
			name:            "malformed availability",
			availabilityRaw: `{"schema":"bad","candidate_id":"AVAILABILITY_CANDIDATE_2"}`,
			wantCalls:       1, wantError: "resolve service continued availability",
		},
		{
			name: "malformed destination",
			availabilityRaw: continuedAvailabilityResultJSON(
				assemblyline.ApplicationServiceAvailabilityRequiredCandidate,
			),
			destinationRaw: `{"schema":"bad","candidate_id":"DESTINATION_CANDIDATE_1"}`,
			wantCalls:      2, wantError: "resolve service persistence destination",
		},
	}
	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			request := "Keep the completed service available in this environment."
			availabilityJob, err := assemblyline.NewApplicationServiceContinuedAvailabilityJob(
				assemblyline.ApplicationServiceContinuedAvailabilityInput{UserRequest: request},
			)
			if err != nil {
				t.Fatal(err)
			}
			destinationJob, err := assemblyline.NewApplicationServicePersistenceDestinationJob(
				assemblyline.ApplicationServicePersistenceDestinationInput{
					UserRequest: request,
					ContinuedAvailability: assemblyline.ApplicationServiceContinuedAvailabilityResult{
						Schema:      assemblyline.ApplicationServiceContinuedAvailabilitySchemaV1,
						CandidateID: assemblyline.ApplicationServiceAvailabilityRequiredCandidate,
					},
				},
			)
			if err != nil {
				t.Fatal(err)
			}
			calls := 0
			runtime := typedWorkerRuntime{
				Context: context.Background(), MaxAttempts: 9, CorrectionModel: "forbidden",
				Execute: func(job assemblyline.PortableJob, model string) (assemblyline.PortableResult, error) {
					calls++
					prompt, schema, renderErr := assemblyline.RenderPortableJob(job)
					if renderErr != nil {
						return assemblyline.PortableResult{}, renderErr
					}
					var raw string
					switch calls {
					case 1:
						if job.Kind != assemblyline.WorkApplicationServiceContinuedAvailability {
							t.Fatalf("continued-availability kind=%q", job.Kind)
						}
						assertContinuedAvailabilityCall(
							t, job.ID, model, prompt, schema, availabilityJob.ID,
						)
						raw = testCase.availabilityRaw
					case 2:
						if job.Kind != assemblyline.WorkApplicationServicePersistenceDestination {
							t.Fatalf("persistence-destination kind=%q", job.Kind)
						}
						assertPersistenceDestinationCall(
							t, job.ID, model, prompt, schema, destinationJob.ID,
						)
						raw = testCase.destinationRaw
					default:
						t.Fatalf("unexpected semantic call %d", calls)
					}
					return assemblyline.PortableResult{JobID: job.ID, Candidate: raw}, nil
				},
			}
			resolution, err := resolveDirectCodingServiceDeploymentDisposition(
				runtime, "availability-model", "destination-model", request, nil,
			)
			if testCase.wantError != "" {
				if err == nil || !strings.Contains(err.Error(), testCase.wantError) {
					t.Fatalf("error=%v want containing %q", err, testCase.wantError)
				}
			} else if err != nil {
				t.Fatal(err)
			}
			if calls != testCase.wantCalls {
				t.Fatalf("calls=%d want=%d", calls, testCase.wantCalls)
			}
			if testCase.wantError == "" && resolution.Disposition != testCase.wantDisposition {
				t.Fatalf("disposition=%q want=%q", resolution.Disposition, testCase.wantDisposition)
			}
			if testCase.wantError == "" {
				assertDeploymentSemanticAuthority(t, resolution, testCase.wantCalls)
			}
		})
	}
}

func assertContinuedAvailabilityCall(
	t *testing.T, jobID, model, prompt string, schema map[string]any, wantJobID string,
) {
	t.Helper()
	if jobID != wantJobID || model != "availability-model" ||
		!strings.Contains(prompt, "AVAILABILITY_CANDIDATE_2") {
		t.Fatalf("continued-availability call job=%q model=%q prompt=%q", jobID, model, prompt)
	}
	if strings.Contains(prompt, "DESTINATION_CANDIDATE_") ||
		strings.Contains(prompt, assemblyline.ApplicationServicePersistenceDestinationSchemaV1) {
		t.Fatalf("continued-availability prompt contains destination responsibility: %s", prompt)
	}
	if strings.Count(prompt, "Keep the completed service available in this environment.") != 1 {
		t.Fatalf("continued-availability prompt did not contain immutable request exactly once: %s", prompt)
	}
	assertServiceDeploymentSchema(t, schema, assemblyline.ApplicationServiceContinuedAvailabilitySchemaV1)
}

func assertPersistenceDestinationCall(
	t *testing.T, jobID, model, prompt string, schema map[string]any, wantJobID string,
) {
	t.Helper()
	if jobID != wantJobID || model != "destination-model" ||
		!strings.Contains(prompt, "DESTINATION_CANDIDATE_2") {
		t.Fatalf("persistence-destination call job=%q model=%q prompt=%q", jobID, model, prompt)
	}
	if strings.Contains(prompt, "AVAILABILITY_CANDIDATE_") ||
		strings.Contains(prompt, assemblyline.ApplicationServiceContinuedAvailabilitySchemaV1) {
		t.Fatalf("persistence-destination prompt contains availability responsibility: %s", prompt)
	}
	if strings.Count(prompt, "Keep the completed service available in this environment.") != 1 {
		t.Fatalf("persistence-destination prompt did not contain immutable request exactly once: %s", prompt)
	}
	assertServiceDeploymentSchema(t, schema, assemblyline.ApplicationServicePersistenceDestinationSchemaV1)
}

func assertServiceDeploymentSchema(t *testing.T, schema map[string]any, want string) {
	t.Helper()
	properties, ok := schema["properties"].(map[string]any)
	if !ok {
		t.Fatalf("schema properties=%#v", schema["properties"])
	}
	schemaProperty, ok := properties["schema"].(map[string]any)
	if !ok || schemaProperty["const"] != want {
		t.Fatalf("schema const=%#v want=%q", schemaProperty["const"], want)
	}
}

func assertDeploymentSemanticAuthority(
	t *testing.T, resolution directCodingServiceDeploymentResolution, calls int,
) {
	t.Helper()
	if len(resolution.ContinuedAvailabilityJobID) != 64 ||
		len(resolution.ContinuedAvailabilityResponseSHA256) != 64 ||
		len(resolution.DispositionJobID) != 64 ||
		len(resolution.DispositionResponseSHA256) != 64 {
		t.Fatalf("continued-availability authority=%+v", resolution)
	}
	if calls == 1 {
		if resolution.PersistenceDestinationJobID != "" ||
			resolution.PersistenceDestinationResponseSHA256 != "" {
			t.Fatalf("verify-only resolution invented destination authority: %+v", resolution)
		}
		return
	}
	if len(resolution.PersistenceDestinationJobID) != 64 ||
		len(resolution.PersistenceDestinationResponseSHA256) != 64 ||
		resolution.PersistenceDestinationJobID == resolution.ContinuedAvailabilityJobID ||
		resolution.PersistenceDestinationResponseSHA256 == resolution.ContinuedAvailabilityResponseSHA256 {
		t.Fatalf("persistent resolution did not retain both authorities: %+v", resolution)
	}
}

func continuedAvailabilityResultJSON(
	candidate assemblyline.ApplicationServiceContinuedAvailabilityCandidateID,
) string {
	return fmt.Sprintf(`{"schema":%q,"candidate_id":%q}`,
		assemblyline.ApplicationServiceContinuedAvailabilitySchemaV1, candidate,
	)
}

func persistenceDestinationResultJSON(
	candidate assemblyline.ApplicationServicePersistenceDestinationCandidateID,
) string {
	return fmt.Sprintf(`{"schema":%q,"candidate_id":%q}`,
		assemblyline.ApplicationServicePersistenceDestinationSchemaV1, candidate,
	)
}
