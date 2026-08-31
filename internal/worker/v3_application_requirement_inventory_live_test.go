package worker

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/gryph/omnidex/internal/assemblyline"
	"github.com/gryph/omnidex/internal/ollama"
)

const liveRequirementsModelEnv = "OMNIDEX_TEST_CODING_REQUIREMENTS_MODEL"

func TestLiveRequirementInventoryAuthorizationQualification(t *testing.T) {
	requirementsModel := strings.TrimSpace(os.Getenv(liveRequirementsModelEnv))
	if requirementsModel == "" {
		t.Skip(liveRequirementsModelEnv + " is not set")
	}
	resultRelationModel := strings.TrimSpace(os.Getenv(liveRequirementResultRelationModelEnv))
	if resultRelationModel == "" {
		t.Skip(liveRequirementResultRelationModelEnv + " is not set")
	}
	baseURL := strings.TrimSpace(os.Getenv("OMNIDEX_TEST_OLLAMA_URL"))
	if baseURL == "" {
		t.Fatal("OMNIDEX_TEST_OLLAMA_URL is required")
	}
	contextTokens, err := strconv.Atoi(strings.TrimSpace(os.Getenv("OMNIDEX_TEST_OLLAMA_CONTEXT")))
	if err != nil || contextTokens <= 0 {
		t.Fatal("OMNIDEX_TEST_OLLAMA_CONTEXT must be a positive integer")
	}
	ctx, cancel := context.WithTimeout(t.Context(), 20*time.Minute)
	t.Cleanup(cancel)
	client := ollama.New(baseURL, requirementsModel, "", 5*time.Minute)

	for _, fixture := range []struct {
		name    string
		request string
	}{
		{
			name: "image transformation and measurement",
			request: "Build software that rotates supplied images by a supplied angle " +
				"and reports the dimensions of each rotated image.",
		},
		{
			name: "appointment creation and cancellation",
			request: "Build software that schedules appointments at supplied times " +
				"and cancels selected appointments.",
		},
	} {
		fixture := fixture
		t.Run(fixture.name, func(t *testing.T) {
			inventoryInput := liveRequirementInventoryInput(t, fixture.request)
			var rawInventory string
			var calls []assemblyline.WorkKind
			runtime := typedWorkerRuntime{
				Context: ctx,
				Execute: func(
					job assemblyline.PortableJob,
					requestedModel string,
				) (assemblyline.PortableResult, error) {
					calls = append(calls, job.Kind)
					wantModel := requirementsModel
					if job.Kind == assemblyline.WorkApplicationRequirementCandidateResultRelation {
						wantModel = resultRelationModel
					}
					if requestedModel != wantModel {
						return assemblyline.PortableResult{}, fmt.Errorf(
							"work kind %s used model %q, want %q",
							job.Kind,
							requestedModel,
							wantModel,
						)
					}
					result, err := executeLiveRequirementsSemanticJob(
						ctx,
						client,
						contextTokens,
						requestedModel,
						job,
						t,
					)
					if err == nil && job.Kind == assemblyline.WorkApplicationRequirementInventory {
						rawInventory = result.Candidate
					}
					return result, err
				},
			}
			resolution, err := resolveDirectCodingApplicationIntent(
				runtime,
				directCodingApplicationIntentModels{
					Requirements: requirementsModel, ResultRelation: resultRelationModel,
				},
				assemblyline.ApplicationIntentInput{
					UserRequest: fixture.request,
					Context:     inventoryInput.Context,
				},
				nil,
			)
			if err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(rawInventory, "\n") {
				t.Fatalf("inventory was not a genuine multiline response: %q", rawInventory)
			}
			inventory, err := assemblyline.DecodeApplicationRequirementInventory(
				inventoryInput,
				rawInventory,
			)
			if err != nil {
				t.Fatal(err)
			}
			if inventory.RawSHA256 != assemblyline.ExactObjectiveContextSHA(rawInventory) {
				t.Fatalf("inventory raw hash=%q, want exact response hash", inventory.RawSHA256)
			}
			if len(resolution.Requirements) != 2 {
				t.Fatalf(
					"retained requirements=%d want 2; raw_inventory=%q resolution=%+v",
					len(resolution.Requirements),
					rawInventory,
					resolution,
				)
			}
			t.Logf(
				"requirements_model=%s result_relation_model=%s raw_candidates=%d retained=2 calls=%v",
				requirementsModel,
				resultRelationModel,
				len(inventory.Candidates),
				calls,
			)
		})
	}
}

func liveRequirementInventoryInput(
	t testing.TB,
	request string,
) assemblyline.ApplicationRequirementInventoryInput {
	t.Helper()
	applicationContext, err := assemblyline.BootstrapApplicationContext(request)
	if err != nil {
		t.Fatal(err)
	}
	return assemblyline.ApplicationRequirementInventoryInput{
		UserRequest: request,
		Context:     applicationContext,
	}
}
