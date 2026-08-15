package worker

import (
	"errors"
	"fmt"
	"path/filepath"
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
			if !mapped || diagnostic.BlockID != "capability.render" ||
				diagnostic.DeclarationLine != 1 || diagnostic.DeclarationColumn != 31 {
				t.Fatalf("diagnostic=%#v mapped=%t", diagnostic, mapped)
			}
		})
	}
}

func TestStructuredVitestFramesMapExactlyWithoutInventingFailureRouting(t *testing.T) {
	root := t.TempDir()
	for _, testCase := range []struct {
		name, path, errorName, wantBlock string
		failureClass                     directCodingStageFailureClass
	}{
		{
			name: "tsx ungrounded behavior assertion stays acceptance-owned", path: "src/panels/Grouping.test.tsx",
			errorName: "AssertionError", failureClass: directCodingStageFailureVitestBehavior,
			wantBlock: "acceptance.grouping",
		},
		{
			name: "ts runtime exception targets acceptance declaration", path: "src/rules/Normalization.test.ts",
			errorName: "ReferenceError", failureClass: directCodingStageFailureUnclassified,
			wantBlock: "acceptance.normalization",
		},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			acceptanceID := "acceptance.grouping"
			featureID := "feature.grouping"
			if strings.HasSuffix(testCase.path, ".ts") {
				acceptanceID = "acceptance.normalization"
				featureID = "feature.normalization"
			}
			document := assemblyline.TypeScriptDocument{
				ID: "acceptance", Path: testCase.path,
				Header: "import { expect } from 'vitest';",
				Blocks: []assemblyline.TypeScriptBlock{{
					ID: acceptanceID, Signature: "function Verify(): void",
					Contract: "Verify one observable result.", API: "function Verify(): void",
					DependsOn: []string{featureID},
				}},
			}
			composed, err := assemblyline.ComposeTypeScriptDocument(document, map[string]string{
				acceptanceID: "function Verify(): void { expect(true).toBe(false); }",
			})
			if err != nil {
				t.Fatal(err)
			}
			line := composed.Spans[acceptanceID].StartLine
			receipt := directCodingVitestFailureReceipt{
				FailureClass: testCase.failureClass,
				Output:       testCase.errorName + ": observed failure",
				Locations: []directCodingVitestSourceLocation{{
					File: filepath.Join(root, filepath.FromSlash(testCase.path)), Line: line, Column: 7,
				}},
			}
			diagnostic, mapped := mapDirectCodingVitestFailureReceipt(
				root, []assemblyline.ComposedTypeScriptDocument{composed}, receipt,
			)
			if !mapped || diagnostic.BlockID != acceptanceID {
				t.Fatalf("mapped diagnostic=%+v mapped=%t", diagnostic, mapped)
			}
			blueprint := assemblyline.TypeScriptBlueprint{Documents: []assemblyline.TypeScriptDocument{
				document,
				{ID: "feature", Path: "src/feature.tsx", Blocks: []assemblyline.TypeScriptBlock{{
					ID: featureID, Signature: "function Feature(): void", Contract: "Implement one result.", API: "function Feature(): void",
				}}},
			}}
			routed, err := routeDirectCodingAcceptanceFailure(directCodingProgram{TypeScript: blueprint}, diagnostic)
			if err != nil {
				t.Fatal(err)
			}
			if routed.BlockID != testCase.wantBlock {
				t.Fatalf("routed block=%s want=%s", routed.BlockID, testCase.wantBlock)
			}
		})
	}
}

func TestUnmappedStageFailureIncludesStructuredReceiptEvidence(t *testing.T) {
	err := directCodingUnmappedStageFailure(
		[]string{"test", "--", "--reporter=./.omnidex-vitest-reporter.mjs"},
		errors.New("exit code 1"),
		"> fixture@1.0.0 test",
		"TestingLibraryElementError: observed structured receipt\n/tmp/stage/src/panel.test.tsx:18:4",
	)
	if err == nil || !strings.Contains(err.Error(), "observed structured receipt") {
		t.Fatalf("unmapped failure discarded receipt: %v", err)
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

func TestUngroundedRuntimeAcceptanceFailureStaysWithAcceptance(t *testing.T) {
	blueprint := assemblyline.TypeScriptBlueprint{Documents: []assemblyline.TypeScriptDocument{
		{ID: "feature", Path: "src/feature.tsx", Blocks: []assemblyline.TypeScriptBlock{{
			ID: "feature.render", Signature: "function Feature(): ReactElement",
			Contract: "Render a usable feature.", API: "function Feature(): ReactElement",
		}}},
		{ID: "acceptance", Path: "src/feature.test.tsx", Blocks: []assemblyline.TypeScriptBlock{{
			ID: "feature.acceptance", Signature: "async function Verify(): Promise<void>",
			Contract: "Verify observable feature behavior.", API: "async function Verify(): Promise<void>",
			DependsOn: []string{"feature.render"},
		}}},
	}}
	diagnostic, err := routeDirectCodingAcceptanceFailure(directCodingProgram{TypeScript: blueprint}, &directCodingStageDiagnostic{
		BlockID: "feature.acceptance", Message: "expected working, received idle", Output: "failure",
		FailureClass: directCodingStageFailureVitestBehavior,
	})
	if err != nil {
		t.Fatal(err)
	}
	if diagnostic.BlockID != "feature.acceptance" {
		t.Fatalf("runtime acceptance failure targeted %s", diagnostic.BlockID)
	}
}

func TestUnclassifiedAcceptanceFailureStaysWithAcceptanceBlock(t *testing.T) {
	blueprint := assemblyline.TypeScriptBlueprint{Documents: []assemblyline.TypeScriptDocument{
		{ID: "feature", Path: "src/feature.tsx", Blocks: []assemblyline.TypeScriptBlock{{
			ID: "feature.render", Signature: "function Feature(): ReactElement",
			Contract: "Render a usable feature.", API: "function Feature(): ReactElement",
		}}},
		{ID: "acceptance", Path: "src/feature.test.tsx", Blocks: []assemblyline.TypeScriptBlock{{
			ID: "feature.acceptance", Signature: "async function Verify(): Promise<void>",
			Contract: "Verify observable feature behavior.", API: "async function Verify(): Promise<void>",
			DependsOn: []string{"feature.render"},
		}}},
	}}
	diagnostic, err := routeDirectCodingAcceptanceFailure(directCodingProgram{TypeScript: blueprint}, &directCodingStageDiagnostic{
		BlockID: "feature.acceptance", Message: "acceptance declaration failed", Output: "failure",
	})
	if err != nil {
		t.Fatal(err)
	}
	if diagnostic.BlockID != "feature.acceptance" {
		t.Fatalf("unclassified acceptance failure targeted %s", diagnostic.BlockID)
	}
}

func TestTypeScriptModelFailureContainsOnlyTheObservedFailure(t *testing.T) {
	raw := "\x1b[31m FAIL src/capability.test.tsx > capability > reacts to input\x1b[0m\n" +
		"AssertionError: expected 1 to be 2\n" +
		" ❯ src/capability.test.tsx:24:18\n" +
		" ❯ /tmp/isolated/node_modules/runner.js:9:2\n"
	feedback := directCodingTypeScriptTestModelFailure(raw)
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
	feedback := directCodingTypeScriptTestModelFailure(raw)
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
