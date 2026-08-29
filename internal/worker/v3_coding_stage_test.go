package worker

import (
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/assemblyline"
	"github.com/gryph/omnidex/internal/modelcontext"
)

func TestTypeScriptStageMapsCompilerLocationsToOneGeneratedBlock(t *testing.T) {
	document := assemblyline.SourceDocument{
		ID: "capability", Path: "src/capability.tsx",
		Blocks: []assemblyline.SourceBlock{{
			ID: "capability.render", Signature: "function Capability(): ReactElement",
			Contract: "Render one usable capability.", API: "function Capability(): ReactElement",
		}},
	}
	composed, err := assemblyline.ComposeTypeScriptDocument(document, assemblyline.SourceComposition{
		Generated:  map[string]string{"capability.render": "function Capability(): ReactElement { return <section />; }"},
		Interfaces: map[string]string{},
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
				[]assemblyline.ComposedSourceDocument{composed},
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
			name: "tsx behavior assertion routes to its generated owner", path: "src/panels/Grouping.test.tsx",
			errorName: "AssertionError", failureClass: directCodingStageFailureVitestBehavior,
			wantBlock: "feature.grouping",
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
			document := assemblyline.SourceDocument{
				ID: "acceptance", Path: testCase.path,
				Preamble: "import { expect } from 'vitest';",
				Blocks: []assemblyline.SourceBlock{{
					ID: acceptanceID, Signature: "function Verify(): void",
					Contract: "Verify one observable result.", API: "function Verify(): void",
					DependsOn: []string{featureID},
				}},
			}
			composed, err := assemblyline.ComposeTypeScriptDocument(document, assemblyline.SourceComposition{
				Generated:  map[string]string{acceptanceID: "function Verify(): void { expect(true).toBe(false); }"},
				Interfaces: map[string]string{},
			})
			if err != nil {
				t.Fatal(err)
			}
			line := composed.Spans[acceptanceID].StartLine
			receipt := directCodingVitestFailureReceipt{
				Failures: []directCodingVitestFailureEvidence{{
					FailureClass: testCase.failureClass,
					Name:         testCase.errorName,
					Message:      "observed failure",
					Output:       testCase.errorName + ": observed failure",
					Locations: []directCodingVitestSourceLocation{{
						File: filepath.Join(root, filepath.FromSlash(testCase.path)), Line: line, Column: 7,
					}},
				}},
			}
			diagnostic, mapped, err := mapDirectCodingVitestFailureReceipt(
				root, []assemblyline.ComposedSourceDocument{composed}, receipt,
			)
			if err != nil {
				t.Fatal(err)
			}
			if !mapped || diagnostic.BlockID != acceptanceID {
				t.Fatalf("mapped diagnostic=%+v mapped=%t", diagnostic, mapped)
			}
			blueprint := assemblyline.SourceBlueprint{Documents: []assemblyline.SourceDocument{
				document,
				{ID: "feature", Path: "src/feature.tsx", Blocks: []assemblyline.SourceBlock{{
					ID: featureID, Signature: "function Feature(): void", Contract: "Implement one result.", API: "function Feature(): void",
				}}},
			}}
			routed, err := routeDirectCodingAcceptanceFailure(directCodingProgram{Source: blueprint}, diagnostic)
			if err != nil {
				t.Fatal(err)
			}
			if routed.BlockID != testCase.wantBlock {
				t.Fatalf("routed block=%s want=%s", routed.BlockID, testCase.wantBlock)
			}
		})
	}
}

func TestStructuredVitestRegexProjectionIsBlockLocalAndRepairGuidanceReady(t *testing.T) {
	t.Parallel()
	fixtures := []struct {
		name               string
		literal            string
		featureID          string
		expectedProjection string
	}{
		{name: "inventory", literal: `/out of stock/i`, featureID: "inventory.render", expectedProjection: `plain text "out of stock" (case-insensitive)`},
		{name: "schedule", literal: `/^monday\/tuesday$/u`, featureID: "schedule.render", expectedProjection: "regular expression pattern formed from ordered components"},
	}
	for _, fixture := range fixtures {
		fixture := fixture
		t.Run(fixture.name, func(t *testing.T) {
			t.Parallel()
			root := t.TempDir()
			acceptanceID := fixture.name + ".acceptance"
			document := assemblyline.SourceDocument{
				ID: fixture.name + ".verification", Path: "src/" + fixture.name + ".test.ts",
				Preamble: "const preambleMatcher = /preamble-only/i;",
				Blocks: []assemblyline.SourceBlock{
					{
						ID: acceptanceID, Signature: "function Verify(): void",
						Contract: "Verify one observable result.", API: "function Verify(): void",
						DependsOn: []string{fixture.featureID}, Globals: []string{"expect"},
					},
					{
						ID:     "sibling." + fixture.name,
						Static: "function Sibling(): boolean { return /sibling-only/i.test('value'); }",
						API:    "function Sibling(): boolean",
					},
				},
			}
			acceptanceSource := "function Verify(): void { expect(" + fixture.literal + ".test('value')).toBe(true); }"
			composed, err := assemblyline.ComposeTypeScriptDocument(
				document,
				assemblyline.SourceComposition{
					Generated:  map[string]string{acceptanceID: acceptanceSource},
					Interfaces: map[string]string{},
				},
			)
			if err != nil {
				t.Fatal(err)
			}
			line := composed.Spans[acceptanceID].StartLine
			message := "Unable to find matcher `" + fixture.literal + "`; unrelated `/sibling-only/i` and `/preamble-only/i`."
			diagnostic, mapped, err := mapDirectCodingVitestFailureReceipt(
				root,
				[]assemblyline.ComposedSourceDocument{composed},
				directCodingVitestFailureReceipt{Failures: []directCodingVitestFailureEvidence{{
					FailureClass: directCodingStageFailureVitestBehavior,
					Name:         "TestingLibraryElementError",
					Message:      message,
					Output:       message,
					Locations: []directCodingVitestSourceLocation{{
						File: filepath.Join(root, filepath.FromSlash(document.Path)), Line: line, Column: 7,
					}},
				}}},
			)
			if err != nil {
				t.Fatal(err)
			}
			if !mapped || diagnostic.BlockID != acceptanceID {
				t.Fatalf("mapped diagnostic=%+v mapped=%t", diagnostic, mapped)
			}
			feedback := diagnostic.ModelFeedback
			if modelcontext.ContainsPathIdentity(feedback) || strings.Contains(feedback, fixture.literal) {
				t.Fatalf("feedback retained matcher path syntax: %q", feedback)
			}
			if strings.Count(feedback, fixture.expectedProjection) != 1 ||
				strings.Contains(feedback, `source text "sibling"`) || strings.Contains(feedback, `source text "preamble"`) {
				t.Fatalf("feedback used nonlocal regex authority: %q", feedback)
			}

			feature := assemblyline.SourceBlock{
				ID: fixture.featureID, Signature: "function Feature(): string",
				Contract: "Return one observable result.", API: "function Feature(): string",
			}
			blueprint := assemblyline.SourceBlueprint{Documents: []assemblyline.SourceDocument{
				document,
				{ID: fixture.name + ".feature", Path: "src/" + fixture.name + ".ts", Blocks: []assemblyline.SourceBlock{feature}},
			}}
			diagnostic, err = routeDirectCodingAcceptanceFailure(
				directCodingProgram{Source: blueprint}, diagnostic,
			)
			if err != nil {
				t.Fatal(err)
			}
			if diagnostic.BlockID != fixture.featureID {
				t.Fatalf("behavior failure routed to %s", diagnostic.BlockID)
			}
			failure, err := directCodingTypeScriptStageModelFeedback(diagnostic)
			if err != nil {
				t.Fatal(err)
			}
			job, err := assemblyline.NewFragmentRepairGuidanceJob(
				assemblyline.FragmentRepairGuidanceInput{
					Language: "typescript", Dialect: "TypeScript function syntax",
					Signature:          feature.Signature,
					CurrentDeclaration: "function Feature(): string { return 'observed'; }",
					Diagnostic:         failure,
				},
			)
			if err != nil {
				t.Fatal(err)
			}
			prompt, err := assemblyline.RenderPortableJob(job)
			if err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(prompt, fixture.expectedProjection) ||
				strings.Contains(prompt, fixture.literal) || strings.Contains(prompt, "regex_literals") {
				t.Fatalf("repair prompt has invalid regex projection:\n%s", prompt)
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
	blueprint := assemblyline.SourceBlueprint{Documents: []assemblyline.SourceDocument{{
		ID: "application", Path: "src/application.tsx",
		Blocks: []assemblyline.SourceBlock{
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

func TestRuntimeAcceptanceBehaviorFailureRoutesToExactGeneratedOwner(t *testing.T) {
	blueprint := assemblyline.SourceBlueprint{Documents: []assemblyline.SourceDocument{
		{ID: "feature", Path: "src/feature.tsx", Blocks: []assemblyline.SourceBlock{{
			ID: "feature.render", Signature: "function Feature(): ReactElement",
			Contract: "Render a usable feature.", API: "function Feature(): ReactElement",
		}}},
		{ID: "acceptance", Path: "src/feature.test.tsx", Blocks: []assemblyline.SourceBlock{{
			ID: "feature.acceptance", Signature: "async function Verify(): Promise<void>",
			Contract: "Verify observable feature behavior.", API: "async function Verify(): Promise<void>",
			DependsOn: []string{"feature.render"},
		}}},
	}}
	diagnostic, err := routeDirectCodingAcceptanceFailure(directCodingProgram{Source: blueprint}, &directCodingStageDiagnostic{
		BlockID: "feature.acceptance", Message: "expected working, received idle", Output: "failure",
		FailureClass: directCodingStageFailureVitestBehavior,
	})
	if err != nil {
		t.Fatal(err)
	}
	if diagnostic.BlockID != "feature.render" {
		t.Fatalf("runtime acceptance failure targeted %s", diagnostic.BlockID)
	}
}

func TestRuntimeAcceptanceBehaviorFailureRequiresOneExactGeneratedOwner(t *testing.T) {
	t.Parallel()
	blueprint := assemblyline.SourceBlueprint{Documents: []assemblyline.SourceDocument{
		{ID: "first", Path: "src/first.tsx", Blocks: []assemblyline.SourceBlock{{
			ID: "feature.first", Signature: "function First(): ReactElement",
			Contract: "Render the first result.", API: "function First(): ReactElement",
		}}},
		{ID: "second", Path: "src/second.tsx", Blocks: []assemblyline.SourceBlock{{
			ID: "feature.second", Signature: "function Second(): ReactElement",
			Contract: "Render the second result.", API: "function Second(): ReactElement",
		}}},
		{ID: "acceptance", Path: "src/feature.test.tsx", Blocks: []assemblyline.SourceBlock{{
			ID: "feature.acceptance", Signature: "async function Verify(): Promise<void>",
			Contract: "Verify observable behavior.", API: "async function Verify(): Promise<void>",
			DependsOn: []string{"feature.first", "feature.second"},
		}}},
	}}
	_, err := routeDirectCodingAcceptanceFailure(directCodingProgram{Source: blueprint}, &directCodingStageDiagnostic{
		BlockID: "feature.acceptance", Message: "expected working, received idle",
		FailureClass: directCodingStageFailureVitestBehavior,
	})
	if err == nil || !strings.Contains(err.Error(), "exactly one generated direct owner") {
		t.Fatalf("ambiguous behavior owner error=%v", err)
	}
}

func TestUnclassifiedAcceptanceFailureStaysWithAcceptanceBlock(t *testing.T) {
	blueprint := assemblyline.SourceBlueprint{Documents: []assemblyline.SourceDocument{
		{ID: "feature", Path: "src/feature.tsx", Blocks: []assemblyline.SourceBlock{{
			ID: "feature.render", Signature: "function Feature(): ReactElement",
			Contract: "Render a usable feature.", API: "function Feature(): ReactElement",
		}}},
		{ID: "acceptance", Path: "src/feature.test.tsx", Blocks: []assemblyline.SourceBlock{{
			ID: "feature.acceptance", Signature: "async function Verify(): Promise<void>",
			Contract: "Verify observable feature behavior.", API: "async function Verify(): Promise<void>",
			DependsOn: []string{"feature.render"},
		}}},
	}}
	diagnostic, err := routeDirectCodingAcceptanceFailure(directCodingProgram{Source: blueprint}, &directCodingStageDiagnostic{
		BlockID: "feature.acceptance", Message: "acceptance declaration failed", Output: "failure",
	})
	if err != nil {
		t.Fatal(err)
	}
	if diagnostic.BlockID != "feature.acceptance" {
		t.Fatalf("unclassified acceptance failure targeted %s", diagnostic.BlockID)
	}
}
