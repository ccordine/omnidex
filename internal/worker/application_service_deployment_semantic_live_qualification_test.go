package worker

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/gryph/omnidex/internal/assemblyline"
	"github.com/gryph/omnidex/internal/ollama"
)

const (
	liveServiceDeploymentModelEnv = "OMNIDEX_TEST_CODING_SERVICE_DEPLOYMENT_INTENT_MODEL"
	liveServiceDeploymentModel    = "phi4:14b"
	liveServiceDeploymentScope    = "live-coding-service-deployment-semantic-split-v1"
)

type liveServiceDeploymentCase struct {
	name         string
	request      string
	availability assemblyline.ApplicationServiceContinuedAvailabilityCandidateID
	destination  assemblyline.ApplicationServicePersistenceDestinationCandidateID
	disposition  assemblyline.ApplicationServiceDeploymentDisposition
}

func TestLiveApplicationServiceDeploymentSemanticSplitQualification(t *testing.T) {
	modelName := strings.TrimSpace(os.Getenv(liveServiceDeploymentModelEnv))
	if modelName == "" {
		t.Skip(liveServiceDeploymentModelEnv + " is not set")
	}
	if modelName != liveServiceDeploymentModel {
		t.Fatalf("%s=%q want %q", liveServiceDeploymentModelEnv, modelName, liveServiceDeploymentModel)
	}
	baseURL := requireLiveCodingQualificationEnv(t, "OMNIDEX_TEST_OLLAMA_URL")
	contextTokens, err := strconv.Atoi(requireLiveCodingQualificationEnv(t, "OMNIDEX_TEST_OLLAMA_CONTEXT"))
	if err != nil || contextTokens <= 0 {
		t.Fatal("OMNIDEX_TEST_OLLAMA_CONTEXT must be a positive integer")
	}
	ctx, cancel := context.WithTimeout(t.Context(), 15*time.Minute)
	defer cancel()
	client := ollama.New(baseURL, modelName, "", 10*time.Minute, contextTokens)
	transport, err := newLiveCodingQualificationTransport(
		ctx, client, modelName, contextTokens, liveServiceDeploymentScope,
	)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf(
		"service_deployment_semantic_qualification model=%s backend=%s backend_version=%s model_digest=%s quantization=%s context_tokens=%d",
		modelName, transport.expected.Backend, transport.expected.BackendVersion,
		transport.expected.Digest, transport.expected.Quantization, contextTokens,
	)

	for _, testCase := range liveServiceDeploymentCases() {
		t.Run(testCase.name, func(t *testing.T) {
			start := transport.callCount()
			var observedAvailability assemblyline.ApplicationServiceContinuedAvailabilityResult
			var observedDestination assemblyline.ApplicationServicePersistenceDestinationResult
			runtime := typedWorkerRuntime{
				Context: ctx, MaxAttempts: 9, CorrectionModel: "forbidden",
				Execute: func(job assemblyline.PortableJob, selectedModel string) (assemblyline.PortableResult, error) {
					if selectedModel != modelName {
						return assemblyline.PortableResult{}, fmt.Errorf("deployment semantic model authority changed")
					}
					prompt, _, renderErr := assemblyline.RenderPortableJob(job)
					if renderErr != nil {
						return assemblyline.PortableResult{}, renderErr
					}
					if strings.Count(prompt, testCase.request) != 1 {
						return assemblyline.PortableResult{}, fmt.Errorf("deployment semantic request authority changed")
					}
					result, executeErr := transport.execute(ctx, job, selectedModel)
					if executeErr != nil {
						return assemblyline.PortableResult{}, executeErr
					}
					switch job.Kind {
					case assemblyline.WorkApplicationServiceContinuedAvailability:
						observedAvailability, renderErr = assemblyline.DecodeApplicationServiceContinuedAvailabilityResult(
							assemblyline.ApplicationServiceContinuedAvailabilityInput{UserRequest: testCase.request},
							result.Candidate,
						)
					case assemblyline.WorkApplicationServicePersistenceDestination:
						observedDestination, renderErr = assemblyline.DecodeApplicationServicePersistenceDestinationResult(
							assemblyline.ApplicationServicePersistenceDestinationInput{
								UserRequest: testCase.request, ContinuedAvailability: observedAvailability,
							},
							result.Candidate,
						)
					default:
						return assemblyline.PortableResult{}, fmt.Errorf("deployment semantic work kind changed")
					}
					if renderErr != nil {
						return assemblyline.PortableResult{}, renderErr
					}
					return result, nil
				},
			}

			resolution, resolveErr := resolveDirectCodingServiceDeploymentDisposition(
				runtime, modelName, modelName, testCase.request, nil,
			)
			if testCase.disposition == assemblyline.ApplicationServiceDeploymentTargetUnresolved {
				if resolveErr == nil || !strings.Contains(resolveErr.Error(), "outside the registered current-host authority") {
					t.Fatalf("unresolved destination error=%v", resolveErr)
				}
			} else if resolveErr != nil {
				t.Fatal(resolveErr)
			} else if resolution.Disposition != testCase.disposition {
				t.Fatalf("disposition=%q want=%q", resolution.Disposition, testCase.disposition)
			}
			if observedAvailability.CandidateID != testCase.availability ||
				observedDestination.CandidateID != testCase.destination {
				t.Fatalf(
					"availability/destination=%q/%q want=%q/%q",
					observedAvailability.CandidateID, observedDestination.CandidateID,
					testCase.availability, testCase.destination,
				)
			}
			calls := transport.callsFrom(start)
			wantCalls := 1
			if testCase.destination != "" {
				wantCalls = 2
			}
			if len(calls) != wantCalls {
				t.Fatalf("deployment semantic calls=%d want=%d", len(calls), wantCalls)
			}
			if calls[0].kind != assemblyline.WorkApplicationServiceContinuedAvailability ||
				(wantCalls == 2 && calls[1].kind != assemblyline.WorkApplicationServicePersistenceDestination) {
				t.Fatalf("deployment semantic call order=%+v", calls)
			}
			for _, call := range calls {
				assertLiveServiceDeploymentCallEvidence(t, call)
				var semanticResponseSHA256 string
				switch call.kind {
				case assemblyline.WorkApplicationServiceContinuedAvailability:
					semanticResponseSHA256, err = directCodingDeploymentSemanticResultSHA256(
						observedAvailability,
					)
				case assemblyline.WorkApplicationServicePersistenceDestination:
					semanticResponseSHA256, err = directCodingDeploymentSemanticResultSHA256(
						observedDestination,
					)
				default:
					t.Fatalf("unexpected deployment semantic evidence kind %q", call.kind)
				}
				if err != nil {
					t.Fatal(err)
				}
				t.Logf(
					"service_deployment_semantic_qualification case=%s kind=%s model=%s job_sha256=%s prompt_sha256=%s request_sha256=%s response_sha256=%s semantic_response_sha256=%s prompt_bytes=%d prompt_tokens=%d output_tokens=%d provider_ms=%d wall_ms=%d",
					testCase.name, call.kind, modelName, call.jobSHA256, call.promptSHA256,
					call.requestSHA256, call.responseSHA256, semanticResponseSHA256,
					call.promptBytes, call.promptTokens, call.outputTokens,
					call.providerDuration.Milliseconds(), call.wallDuration.Milliseconds(),
				)
			}
		})
	}
}

func liveServiceDeploymentCases() []liveServiceDeploymentCase {
	return []liveServiceDeploymentCase{
		{
			name:         "command-line utility without persistence",
			request:      "Create a command-line formatter that reads CSV records and writes aligned text.",
			availability: assemblyline.ApplicationServiceAvailabilityNotRequiredCandidate,
			disposition:  assemblyline.ApplicationServiceDeploymentVerifyOnly,
		},
		{
			name:         "browser behavior without persistence",
			request:      "Build a browser diagram editor with a live preview and export controls.",
			availability: assemblyline.ApplicationServiceAvailabilityNotRequiredCandidate,
			disposition:  assemblyline.ApplicationServiceDeploymentVerifyOnly,
		},
		{
			name:         "current build environment persistence",
			request:      "Build a warehouse status endpoint and keep it running in the environment where it is built.",
			availability: assemblyline.ApplicationServiceAvailabilityRequiredCandidate,
			destination:  assemblyline.ApplicationServiceBuildEnvironmentDestinationCandidate,
			disposition:  assemblyline.ApplicationServiceDeploymentPersistCurrentHost,
		},
		{
			name:         "separate destination persistence",
			request:      "Build a public transit alert service and keep it running in a separate hosted environment.",
			availability: assemblyline.ApplicationServiceAvailabilityRequiredCandidate,
			destination:  assemblyline.ApplicationServiceBuildEnvironmentNotEstablishedCandidate,
			disposition:  assemblyline.ApplicationServiceDeploymentTargetUnresolved,
		},
		{
			name:         "unstated build environment identity",
			request:      "Build a laboratory equipment monitor and keep it running after verification.",
			availability: assemblyline.ApplicationServiceAvailabilityRequiredCandidate,
			destination:  assemblyline.ApplicationServiceBuildEnvironmentNotEstablishedCandidate,
			disposition:  assemblyline.ApplicationServiceDeploymentTargetUnresolved,
		},
		{
			name:         "named destination with unresolved identity",
			request:      "Build a municipal bulletin service and keep it running on the server.",
			availability: assemblyline.ApplicationServiceAvailabilityRequiredCandidate,
			destination:  assemblyline.ApplicationServiceBuildEnvironmentNotEstablishedCandidate,
			disposition:  assemblyline.ApplicationServiceDeploymentTargetUnresolved,
		},
	}
}

func assertLiveServiceDeploymentCallEvidence(t *testing.T, call liveCodingQualificationCall) {
	t.Helper()
	for name, digest := range map[string]string{
		"job": call.jobSHA256, "prompt": call.promptSHA256,
		"request": call.requestSHA256, "response": call.responseSHA256,
	} {
		decoded, err := hex.DecodeString(digest)
		if err != nil || len(decoded) != sha256.Size {
			t.Fatalf("%s digest=%q", name, digest)
		}
	}
	if call.promptBytes < 1 || call.promptTokens < 1 || call.outputTokens < 1 ||
		call.providerDuration <= 0 || call.wallDuration <= 0 {
		t.Fatalf("incomplete deployment semantic call evidence: %+v", call)
	}
}
