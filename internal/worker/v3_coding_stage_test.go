package worker

import (
	"fmt"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/assemblyline"
)

func TestTypeScriptStageMapsCompilerLocationsToOneGeneratedBlock(t *testing.T) {
	document := assemblyline.TypeScriptDocument{
		ID: "capability", Path: "src/capability.tsx",
		Blocks: []assemblyline.TypeScriptBlock{{
			ID: "capability.render", Signature: "function Capability(): ReactElement",
			Contract: "Render one usable capability.", API: "function Capability(): ReactElement",
		}},
	}
	composed, err := assemblyline.ComposeTypeScriptDocument(document, map[string]string{
		"capability.render": "function Capability(): ReactElement { return <section />; }",
	})
	if err != nil {
		t.Fatal(err)
	}
	line := composed.Spans["capability.render"].StartLine
	for name, output := range map[string]string{
		"colon": "src/capability.tsx:" + fmt.Sprint(line) + ":31 error TS2345: wrong value",
		"paren": "src/capability.tsx(" + fmt.Sprint(line) + ",31): error TS2345: wrong value",
		"ansi":  " \x1b[36m❯\x1b[0m src/capability.tsx:\x1b[2m" + fmt.Sprint(line) + ":31\x1b[0m error TS2345: wrong value",
	} {
		t.Run(name, func(t *testing.T) {
			diagnostic, mapped := mapDirectCodingTypeScriptStageDiagnostic(
				[]assemblyline.ComposedTypeScriptDocument{composed},
				output,
			)
			if !mapped || diagnostic.BlockID != "capability.render" {
				t.Fatalf("diagnostic=%#v mapped=%t", diagnostic, mapped)
			}
		})
	}
}

func TestTypeScriptStageNeverDelegatesCodeOwnedFailures(t *testing.T) {
	blueprint := assemblyline.TypeScriptBlueprint{Documents: []assemblyline.TypeScriptDocument{{
		ID: "application", Path: "src/application.tsx",
		Blocks: []assemblyline.TypeScriptBlock{
			{
				ID: "capability.render", Signature: "function Capability(): ReactElement",
				Contract: "Render one usable capability.", API: "function Capability(): ReactElement",
			},
			{ID: "application.render", Static: "function App(): ReactElement { return <Capability />; }", API: "function App(): ReactElement"},
		},
	}}}
	target, err := directCodingTypeScriptCorrectionBlock(blueprint, "capability.render")
	if err != nil || target.ID != "capability.render" {
		t.Fatalf("generated target=%#v err=%v", target, err)
	}
	if _, err := directCodingTypeScriptCorrectionBlock(blueprint, "application.render"); err == nil || !strings.Contains(err.Error(), "code-owned") {
		t.Fatalf("static adapter defect was delegated: %v", err)
	}
	if _, err := directCodingTypeScriptCorrectionBlock(blueprint, "missing"); err == nil || !strings.Contains(err.Error(), "unknown") {
		t.Fatalf("unknown block was accepted: %v", err)
	}
}

func TestRuntimeAcceptanceFailureTargetsImplementationNotTheTest(t *testing.T) {
	blueprint := assemblyline.TypeScriptBlueprint{Documents: []assemblyline.TypeScriptDocument{
		{ID: "feature", Path: "src/feature.tsx", Blocks: []assemblyline.TypeScriptBlock{{
			ID: "feature.render", Signature: "function Feature(): ReactElement",
			Contract: "Render a usable feature.", API: "function Feature(): ReactElement",
		}}},
		{ID: "acceptance", Path: "src/feature.test.tsx", Blocks: []assemblyline.TypeScriptBlock{{
			ID: "feature.acceptance", Signature: "async function Verify(): Promise<void>",
			Contract: "Verify observable feature behavior.", API: "async function Verify(): Promise<void>",
			DependsOn: []string{"feature.render"}, FailureTarget: "feature.render",
		}}},
	}}
	diagnostic, err := routeDirectCodingAcceptanceFailure(blueprint, &directCodingStageDiagnostic{
		BlockID: "feature.acceptance", Message: "expected working, received idle", Output: "failure",
	})
	if err != nil {
		t.Fatal(err)
	}
	if diagnostic.BlockID != "feature.render" {
		t.Fatalf("runtime acceptance failure targeted %s", diagnostic.BlockID)
	}
}

func TestTypeScriptModelFailureContainsOnlyTheObservedFailure(t *testing.T) {
	raw := "\x1b[31m FAIL src/capability.test.tsx > capability > reacts to input\x1b[0m\n" +
		"AssertionError: expected 1 to be 2\n" +
		" ❯ src/capability.test.tsx:24:18\n" +
		" ❯ /tmp/isolated/node_modules/runner.js:9:2\n"
	feedback := directCodingTypeScriptModelFailure(raw)
	for _, required := range []string{
		"capability > reacts to input", "expected 1 to be 2",
	} {
		if !strings.Contains(feedback, required) {
			t.Fatalf("feedback omitted %q:\n%s", required, feedback)
		}
	}
	for _, forbidden := range []string{
		"capability.test.tsx", "/tmp/", "node_modules", "className", "button", "workspace", "filename", "\x1b",
	} {
		if strings.Contains(feedback, forbidden) {
			t.Fatalf("feedback leaked or prescribed %q:\n%s", forbidden, feedback)
		}
	}
}

func TestTypeScriptModelFailureCutsNeighborFailuresAndSourceFrames(t *testing.T) {
	raw := `
 FAIL  src/checks.test.tsx > first capability > responds
AssertionError: expected false to be true
 ❯ src/checks.test.tsx:35:93
     35| expect(secretImplementation()).toBe(true);

 FAIL  src/checks.test.tsx > second capability > persists
TestingLibraryElementError: Unable to find an accessible element
`
	feedback := directCodingTypeScriptModelFailure(raw)
	for _, required := range []string{"first capability > responds", "expected false to be true"} {
		if !strings.Contains(feedback, required) {
			t.Fatalf("focused feedback omitted %q:\n%s", required, feedback)
		}
	}
	for _, forbidden := range []string{"second capability", "secretImplementation", "checks.test.tsx"} {
		if strings.Contains(feedback, forbidden) {
			t.Fatalf("focused feedback leaked %q:\n%s", forbidden, feedback)
		}
	}
}
