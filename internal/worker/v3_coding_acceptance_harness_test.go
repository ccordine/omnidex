package worker

import (
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/assemblyline"
)

func TestGeneratedAcceptanceCannotRenderTestAuthoredProductUI(t *testing.T) {
	t.Parallel()

	fixtures := []struct {
		name, product, requirement, criterion string
		inventedSource                        string
	}{
		{
			name: "inventory row", product: "inventory browser", requirement: "show stock items",
			criterion: "The stock-item collection is visible.",
			inventedSource: `async function VerifyFeature001(): Promise<void> {
  render(<><Feature001 runtime={createFeatureRuntime(createApplicationRuntime(), "capability_001")} /><div role="row">Invented stock item</div></>);
  expect(screen.getByRole("row")).toBeInTheDocument();
}`,
		},
		{
			name: "appointment button", product: "appointment schedule", requirement: "show appointments",
			criterion: "The appointment schedule is visible.",
			inventedSource: `async function VerifyFeature001(): Promise<void> {
  render(<section><Feature001 runtime={createFeatureRuntime(createApplicationRuntime(), "capability_001")} /><button>Invented appointment</button></section>);
  expect(screen.getByRole("button", { name: "Invented appointment" })).toBeInTheDocument();
}`,
		},
	}
	for _, fixture := range fixtures {
		fixture := fixture
		t.Run(fixture.name, func(t *testing.T) {
			t.Parallel()
			program := directCodingGroundingFixtureProgram(
				t, fixture.product, fixture.requirement, []string{fixture.criterion},
				`expect(screen.getByText("Visible product behavior")).toBeInTheDocument();`,
			)
			block, exists := directCodingTypeScriptBlueprintBlock(program.TypeScript, "acceptance.001")
			if !exists {
				t.Fatal("acceptance.001 is missing")
			}
			_, err := assemblyline.ParseTypeScriptFunction(assemblyline.TypeScriptFunctionContract{
				Signature: block.Signature, TSX: true, Policy: block.Policy,
			}, fixture.inventedSource)
			if err == nil || !strings.Contains(err.Error(), "forbidden direct identifier") {
				t.Fatalf("test-authored render was not rejected by the acceptance block: %v", err)
			}

			program.Generated["acceptance.001"] = fixture.inventedSource
			diagnostic, routeErr := routeDirectCodingAcceptanceFailure(program, &directCodingStageDiagnostic{
				BlockID: "acceptance.001", FailureClass: directCodingStageFailureVitestBehavior,
				DeclarationLine: 3, DeclarationColumn: 10,
			})
			if routeErr != nil {
				t.Fatal(routeErr)
			}
			if diagnostic.BlockID != "acceptance.001" {
				t.Fatalf("unaccepted test-authored UI gained implementation authority: %s", diagnostic.BlockID)
			}
		})
	}
}

func TestBrowserAcceptanceHarnessOwnsTheOnlyFeatureRender(t *testing.T) {
	t.Parallel()

	program := directCodingGroundingFixtureProgram(
		t, "inventory browser", "show stock items",
		[]string{"The stock-item collection is visible."},
		`expect(screen.getByText("Stock items")).toBeInTheDocument();`,
	)
	generated, exists := directCodingTypeScriptBlueprintBlock(program.TypeScript, "acceptance.001")
	if !exists {
		t.Fatal("acceptance.001 is missing")
	}
	for _, forbidden := range []string{
		"render", "createApplicationRuntime", "createFeatureRuntime", "Feature001",
	} {
		if stringSliceContains(generated.Globals, forbidden) ||
			!stringSliceContains(generated.Policy.ForbiddenIdentifiers, forbidden) {
			t.Fatalf("generated acceptance authority for %s: globals=%v forbidden=%v",
				forbidden, generated.Globals, generated.Policy.ForbiddenIdentifiers)
		}
	}
	if len(generated.DependsOn) != 0 || len(generated.Capabilities) != 0 ||
		len(generated.Policy.RequiredCalls) != 1 ||
		!stringSliceContains(generated.Policy.RequiredCalls[0].Callees, "expect") {
		t.Fatalf("generated acceptance retained render/dependency authority: %+v", generated)
	}
	for _, required := range []string{
		"direct screen queries", "standalone throwing observations", "expect subjects",
		"fireEvent targets", "static arguments and event payloads", "Await findBy, findAllBy, and waitFor",
		"Do not create aliases, local proof values, UI, arbitrary calls, assignments, or control flow",
	} {
		if !strings.Contains(generated.Contract, required) {
			t.Fatalf("generated acceptance contract hides enforced observer grammar %q: %s", required, generated.Contract)
		}
	}

	harness, exists := directCodingTypeScriptBlueprintBlock(program.TypeScript, "acceptance.harness.001")
	if !exists || !strings.Contains(harness.Static, `render(<Feature001 runtime={createFeatureRuntime(createApplicationRuntime(), "capability_001")} />);`) ||
		strings.Count(harness.Static, "render(") != 1 || !strings.Contains(harness.Static, "await VerifyFeature001();") {
		t.Fatalf("code-owned acceptance harness is not exact: %+v", harness)
	}
	registration, exists := directCodingTypeScriptBlueprintBlock(program.TypeScript, "acceptance.register.001")
	if !exists || !strings.Contains(registration.Static, "RunFeature001Acceptance") ||
		strings.Contains(registration.Static, ", VerifyFeature001)") {
		t.Fatalf("registration bypasses code-owned harness: %+v", registration)
	}
}

func stringSliceContains(values []string, expected string) bool {
	for _, value := range values {
		if value == expected {
			return true
		}
	}
	return false
}
