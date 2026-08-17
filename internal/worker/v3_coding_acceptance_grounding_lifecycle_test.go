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

func TestAcceptanceGroundingReviewsEachNewSourceOnceAndReusesCurrentReceipt(t *testing.T) {
	t.Parallel()

	program := directCodingGroundingFixtureProgram(
		t, "inventory browser", "show stock items",
		[]string{"The stock-item collection is visible."},
		`expect(screen.getByText("Stock items")).toBeInTheDocument();`,
	)
	reviewCalls := 0
	runtime := typedWorkerRuntime{
		Context: context.Background(),
		Execute: func(job assemblyline.PortableJob, _ string) (assemblyline.PortableResult, error) {
			if job.Kind != assemblyline.WorkApplicationAcceptanceGroundingReview {
				return assemblyline.PortableResult{}, fmt.Errorf("unexpected work kind %s", job.Kind)
			}
			reviewCalls++
			var input assemblyline.ApplicationAcceptanceGroundingReviewInput
			if err := json.Unmarshal(job.Payload, &input); err != nil {
				return assemblyline.PortableResult{}, err
			}
			return directCodingPortableCandidate(job, directCodingAcceptedGroundingJSON(t, input)), nil
		},
	}
	if err := ensureDirectCodingAcceptanceGrounding(runtime, "reviewer", "corrector", &program); err != nil {
		t.Fatal(err)
	}
	if reviewCalls != 1 {
		t.Fatalf("initial grounding calls=%d want 1", reviewCalls)
	}

	program.Generated["feature.001"] = strings.Replace(
		program.Generated["feature.001"], "section", "article", 1,
	)
	if err := ensureDirectCodingAcceptanceGrounding(runtime, "reviewer", "corrector", &program); err != nil {
		t.Fatal(err)
	}
	if reviewCalls != 1 {
		t.Fatalf("feature-only correction caused %d grounding calls", reviewCalls)
	}

	program.Generated["acceptance.001"] = strings.Replace(
		program.Generated["acceptance.001"], "Stock items", "Available stock items", 1,
	)
	if err := ensureDirectCodingAcceptanceGrounding(runtime, "reviewer", "corrector", &program); err != nil {
		t.Fatal(err)
	}
	if reviewCalls != 2 {
		t.Fatalf("changed acceptance source caused %d grounding calls want 2", reviewCalls)
	}
}

func TestInventedAcceptanceAssumptionsStayAcceptanceOwnedAndAreCorrected(t *testing.T) {
	t.Parallel()

	fixtures := []struct {
		name, product, requirement, criterion string
		unsupportedOperation                  string
		inventedBody, retainedBody            string
	}{
		{
			name: "inventory count", product: "inventory browser", requirement: "show stock items",
			criterion:            "The stock-item collection is visible.",
			unsupportedOperation: "expect_matcher:toHaveLength",
			inventedBody:         `expect(screen.getAllByRole("row")).toHaveLength(4);`,
			retainedBody:         `expect(screen.getByText("Stock items")).toBeInTheDocument();`,
		},
		{
			name: "schedule role", product: "appointment schedule", requirement: "show appointments",
			criterion:            "The appointment schedule is visible.",
			unsupportedOperation: "testing_library_query:getByRole",
			inventedBody:         `expect(screen.getByRole("group", { name: "Appointments" })).toBeInTheDocument();`,
			retainedBody:         `expect(screen.getByText("Appointments")).toBeInTheDocument();`,
		},
	}
	for _, fixture := range fixtures {
		fixture := fixture
		t.Run(fixture.name, func(t *testing.T) {
			t.Parallel()
			program := directCodingGroundingFixtureProgram(
				t, fixture.product, fixture.requirement, []string{fixture.criterion},
				fixture.inventedBody+"\n"+fixture.retainedBody,
			)
			featureBefore := program.Generated["feature.001"]
			before, err := routeDirectCodingAcceptanceFailure(program, &directCodingStageDiagnostic{
				BlockID: "acceptance.001", FailureClass: directCodingStageFailureVitestBehavior,
			})
			if err != nil || before.BlockID != "acceptance.001" {
				t.Fatalf("unreviewed failure route=%+v error=%v", before, err)
			}

			calls := []assemblyline.WorkKind{}
			reviews := 0
			unsupportedSiteID := ""
			runtime := typedWorkerRuntime{
				Context: context.Background(),
				Execute: func(job assemblyline.PortableJob, _ string) (assemblyline.PortableResult, error) {
					calls = append(calls, job.Kind)
					switch job.Kind {
					case assemblyline.WorkApplicationAcceptanceGroundingReview:
						reviews++
						var input assemblyline.ApplicationAcceptanceGroundingReviewInput
						if err := json.Unmarshal(job.Payload, &input); err != nil {
							return assemblyline.PortableResult{}, err
						}
						if reviews == 1 {
							unsupportedSiteID = directCodingGroundingSiteIDForOperation(t, input, fixture.unsupportedOperation)
							return directCodingPortableCandidate(job, directCodingGroundingMatrixJSON(
								t, input, func(actualSiteID string, _ string) bool {
									return actualSiteID != unsupportedSiteID
								},
							)), nil
						}
						return directCodingPortableCandidate(job, directCodingAcceptedGroundingJSON(t, input)), nil
					default:
						return assemblyline.PortableResult{}, fmt.Errorf("unexpected work kind %s", job.Kind)
					}
				},
			}
			if err := ensureDirectCodingAcceptanceGrounding(runtime, "reviewer", "corrector", &program); err != nil {
				t.Fatal(err)
			}
			wantCalls := []assemblyline.WorkKind{
				assemblyline.WorkApplicationAcceptanceGroundingReview,
				assemblyline.WorkApplicationAcceptanceGroundingReview,
			}
			if !reflect.DeepEqual(calls, wantCalls) {
				t.Fatalf("calls=%v want=%v", calls, wantCalls)
			}
			if program.Generated["feature.001"] != featureBefore ||
				strings.Contains(program.Generated["acceptance.001"], fixture.inventedBody) ||
				!strings.Contains(program.Generated["acceptance.001"], fixture.retainedBody) {
				t.Fatalf("grounding repair did not make the exact code-owned source transition: feature=%q acceptance=%q", program.Generated["feature.001"], program.Generated["acceptance.001"])
			}
		})
	}
}
