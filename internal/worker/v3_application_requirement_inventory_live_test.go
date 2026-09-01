package worker

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/assemblyline"
	"github.com/gryph/omnidex/internal/llm"
	"github.com/gryph/omnidex/internal/model"
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
	ctx := t.Context()
	client := ollama.New(baseURL, requirementsModel, "", llm.MaximumModelRequestDuration)

	for _, fixture := range []struct {
		name             string
		request          string
		wantRequirements int
	}{
		{
			name:             "single confirmation outcome",
			request:          "The finished software lets a user confirm the item.",
			wantRequirements: 1,
		},
		{
			name: "image transformation and measurement",
			request: "Build software that rotates supplied images by a supplied angle " +
				"and reports the dimensions of each rotated image.",
			wantRequirements: 2,
		},
		{
			name: "appointment creation and cancellation",
			request: "Build software that schedules appointments at supplied times " +
				"and cancels selected appointments.",
			wantRequirements: 2,
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
			proposals, err := resolveDirectCodingApplicationPlan(
				runtime,
				directCodingApplicationIntentModels{
					Requirements: requirementsModel, ResultRelation: resultRelationModel,
				},
				assemblyline.ApplicationIntentInput{
					UserRequest: fixture.request,
					Context:     inventoryInput.Context,
				},
				model.CodingScopeModeNormal,
				nil,
			)
			if err != nil {
				t.Fatal(err)
			}
			if fixture.wantRequirements > 1 && !strings.Contains(rawInventory, "\n") {
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
			if len(inventory.Candidates) != fixture.wantRequirements {
				t.Fatalf(
					"raw candidates=%d want %d; raw_inventory=%q",
					len(inventory.Candidates),
					fixture.wantRequirements,
					rawInventory,
				)
			}
			if len(proposals) != fixture.wantRequirements {
				t.Fatalf(
					"retained requirements=%d want %d; raw_inventory=%q proposals=%+v",
					len(proposals),
					fixture.wantRequirements,
					rawInventory,
					proposals,
				)
			}
			t.Logf(
				"requirements_model=%s result_relation_model=%s raw_candidates=%d retained=%d calls=%v",
				requirementsModel,
				resultRelationModel,
				len(inventory.Candidates),
				len(proposals),
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
		ScopeMode:   model.CodingScopeModeNormal,
	}
}
