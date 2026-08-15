package worker

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/assemblyline"
)

func TestGeneratedAcceptanceHasNoUnconditionalImplementationFailureTarget(t *testing.T) {
	t.Parallel()

	fixtures := []struct {
		name        string
		product     string
		requirement string
	}{
		{name: "inventory collection", product: "inventory browser", requirement: "show stock items"},
		{name: "appointment selection", product: "schedule board", requirement: "select an appointment"},
	}
	for _, fixture := range fixtures {
		fixture := fixture
		t.Run(fixture.name, func(t *testing.T) {
			t.Parallel()

			specification := assemblyline.ApplicationSpecification{
				Surface:      assemblyline.ApplicationSurfaceBrowser,
				ProductQuote: fixture.product,
				Requirements: []assemblyline.Requirement{{
					ID: "requirement_001", SourceQuote: fixture.requirement,
				}},
			}
			_, blueprint, _, err := compileGenericTypeScriptBrowserBlueprint(
				"unseen", specification, genericBrowserSkillBindings(specification),
				genericBrowserWorkload(t, specification),
				genericBrowserCapabilityBindings(specification),
			)
			if err != nil {
				t.Fatal(err)
			}
			acceptance, exists := directCodingTypeScriptBlueprintBlock(blueprint, "acceptance.001")
			if !exists {
				t.Fatal("acceptance.001 is missing")
			}
			diagnostic, err := routeDirectCodingAcceptanceFailure(directCodingProgram{TypeScript: blueprint}, &directCodingStageDiagnostic{
				BlockID: acceptance.ID, FailureClass: directCodingStageFailureVitestBehavior,
			})
			if err != nil {
				t.Fatal(err)
			}
			if diagnostic.BlockID != acceptance.ID {
				t.Fatalf("unreviewed generated acceptance targeted %s", diagnostic.BlockID)
			}
		})
	}
}

func TestRetiredTypeScriptFailureTargetAuthorityIsAbsent(t *testing.T) {
	t.Parallel()

	_, testFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolve acceptance-grounding test path")
	}
	workerDir := filepath.Dir(testFile)
	paths := []string{
		filepath.Join(workerDir, "..", "assemblyline", "typescript_blueprint.go"),
		filepath.Join(workerDir, "v3_coding_browser_acceptance.go"),
		filepath.Join(workerDir, "v3_coding_typescript_stage.go"),
	}
	for _, path := range paths {
		raw, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(raw), "FailureTarget") {
			t.Fatalf("production source retains retired unconditional failure authority in %s", path)
		}
	}
}

func TestCurrentGroundingReceiptAlonePermitsBehaviorFailureToTargetFeature(t *testing.T) {
	t.Parallel()

	program := directCodingGroundingFixtureProgram(
		t, "inventory browser", "show stock items",
		[]string{"The stock-item collection is visible."},
		`await waitFor(() => expect(screen.getByText("Stock items")).toBeInTheDocument());`,
	)
	input := directCodingGroundingInput(t, program, "acceptance.001")
	review, err := assemblyline.DecodeApplicationAcceptanceGroundingReview(
		input, directCodingAcceptedGroundingJSON(t, input),
	)
	if err != nil {
		t.Fatal(err)
	}
	receipt, err := assemblyline.AcceptApplicationAcceptanceGroundingReview(input, review)
	if err != nil {
		t.Fatal(err)
	}
	program.AcceptanceGrounding["acceptance.001"] = receipt

	line, column := directCodingSourceLocation(t, program.Generated["acceptance.001"], "screen.getByText")
	behavior := &directCodingStageDiagnostic{
		BlockID: "acceptance.001", FailureClass: directCodingStageFailureVitestBehavior,
		DeclarationLine: line, DeclarationColumn: column,
	}
	routed, err := routeDirectCodingAcceptanceFailure(program, behavior)
	if err != nil {
		t.Fatal(err)
	}
	if routed.BlockID != "feature.001" {
		t.Fatalf("current grounded behavior failure targeted %s", routed.BlockID)
	}
	matcherLine, matcherColumn := directCodingSourceLocation(t, program.Generated["acceptance.001"], "toBeInTheDocument")
	matcherFailure := *behavior
	matcherFailure.DeclarationLine = matcherLine
	matcherFailure.DeclarationColumn = matcherColumn
	routed, err = routeDirectCodingAcceptanceFailure(program, &matcherFailure)
	if err != nil {
		t.Fatal(err)
	}
	if routed.BlockID != "feature.001" {
		t.Fatalf("grounded matcher failure targeted %s", routed.BlockID)
	}
	renderLine, renderColumn := directCodingSourceLocation(t, program.Generated["acceptance.001"], "waitFor(")
	platformOnly := *behavior
	platformOnly.DeclarationLine = renderLine
	platformOnly.DeclarationColumn = renderColumn
	routed, err = routeDirectCodingAcceptanceFailure(program, &platformOnly)
	if err != nil {
		t.Fatal(err)
	}
	if routed.BlockID != "acceptance.001" {
		t.Fatalf("platform-only wait observation failure targeted %s", routed.BlockID)
	}
	unmapped := *behavior
	unmapped.DeclarationLine = 0
	unmapped.DeclarationColumn = 0
	routed, err = routeDirectCodingAcceptanceFailure(program, &unmapped)
	if err != nil {
		t.Fatal(err)
	}
	if routed.BlockID != "acceptance.001" {
		t.Fatalf("unmapped observation failure targeted %s", routed.BlockID)
	}

	runtimeFailure := *behavior
	runtimeFailure.FailureClass = directCodingStageFailureUnclassified
	routed, err = routeDirectCodingAcceptanceFailure(program, &runtimeFailure)
	if err != nil {
		t.Fatal(err)
	}
	if routed.BlockID != "acceptance.001" {
		t.Fatalf("unclassified acceptance defect targeted %s", routed.BlockID)
	}

	program.Generated["acceptance.001"] = strings.Replace(
		program.Generated["acceptance.001"], "Stock items", "Available items", 1,
	)
	routed, err = routeDirectCodingAcceptanceFailure(program, behavior)
	if err != nil {
		t.Fatal(err)
	}
	if routed.BlockID != "acceptance.001" {
		t.Fatalf("stale grounding receipt targeted %s", routed.BlockID)
	}
}
