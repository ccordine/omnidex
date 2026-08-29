package worker

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/assemblyline"
	"github.com/gryph/omnidex/internal/modelcontext"
)

func TestRealVitestBehaviorFailureRoutesToImplementationOwner(t *testing.T) {
	if os.Getenv("OMNIDEX_NODE_INTEGRATION") != "1" {
		t.Skip("set OMNIDEX_NODE_INTEGRATION=1 to install the pinned Vitest toolchain")
	}
	blueprint := assemblyline.SourceBlueprint{Documents: []assemblyline.SourceDocument{
		{
			ID: "feature", Path: "src/feature.tsx",
			Preamble: "import type { ReactElement } from 'react';",
			Blocks: []assemblyline.SourceBlock{{
				ID: "feature.render", Signature: "function Feature(): ReactElement",
				Contract: "Render the observable value.", API: "function Feature(): ReactElement",
				Export: true,
			}},
		},
		{
			ID: "acceptance", Path: "src/feature.test.tsx",
			Preamble: `import '@testing-library/jest-dom/vitest';
import { computeAccessibleName } from 'dom-accessibility-api';
import React from 'react';
import { configure, getRoles, render, screen } from '@testing-library/react';
import { Feature } from './feature';

` + genericBrowserTestingLibraryRoleObservationSupport,
			Blocks: []assemblyline.SourceBlock{
				{
					ID: "feature.acceptance", Signature: "async function Verify(): Promise<void>",
					Contract: "Verify the observable value.", API: "async function Verify(): Promise<void>",
					DependsOn: []string{"feature.render"}, Globals: []string{"screen", "expect"},
				},
				{
					ID: "acceptance.harness", Static: `async function RunAcceptance(): Promise<void> {
  render(<Feature />);
  await Verify();
}`,
					API:       "async function RunAcceptance(): Promise<void>",
					DependsOn: []string{"feature.render", "feature.acceptance"},
				},
				{
					ID: "acceptance.register", Static: `it('observes the expected value', RunAcceptance);`,
					API: "registered acceptance", DependsOn: []string{"acceptance.harness"},
				},
				{
					ID: "acceptance.unrelated", Static: `it('reports an unrelated runtime defect', () => { throw new TypeError('unrelated runtime defect'); });`,
					API: "registered unrelated runtime defect",
				},
			},
		},
	}}
	staticFiles := make([]directCodingFileTask, 0, 3)
	fixtureFiles, err := typeScriptBrowserStaticFiles(
		requireDirectCodingVersionProfile(t, typeScriptBrowserVersionProfileV1),
		"vitest-route", "Vitest route", "",
	)
	if err != nil {
		t.Fatal(err)
	}
	for _, file := range fixtureFiles {
		if file.Path == "package.json" || file.Path == "package-lock.json" || file.Path == "tsconfig.json" || file.Path == "vite.config.ts" {
			staticFiles = append(staticFiles, file)
		}
	}
	program := directCodingProgram{
		StackID: genericTypeScriptBrowserAdapter, VersionProfileID: typeScriptBrowserVersionProfileV1,
		Source:      blueprint,
		StaticFiles: staticFiles,
		Generated: map[string]string{
			"feature.render":     `function Feature(): ReactElement { return <button>actual</button>; }`,
			"feature.acceptance": `async function Verify(): Promise<void> { expect(screen.getByRole('button', { name: /restock/i })).toBeInTheDocument(); }`,
		},
	}
	stack, err := directCodingProjectStackByID(program.StackID)
	if err != nil {
		t.Fatal(err)
	}
	program.Source, err = bindDirectCodingSourceBlueprintAdapters(stack, program.Source)
	if err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	if err := writeDirectCodingVitestReporter(root); err != nil {
		t.Fatal(err)
	}
	if err := writeDirectCodingTypeScriptStage(root, program); err != nil {
		t.Fatal(err)
	}
	output, err := runDirectCodingStageCommand(
		context.Background(), root, directCodingTypeScriptInstallTimeout,
		"npm", directCodingNPMInstallArgs()...,
	)
	if err != nil {
		t.Fatalf("install pinned Vitest toolchain: %v\n%s", err, output)
	}
	diagnostic, err := verifyDirectCodingTypeScriptStageCommands(
		context.Background(), root, program,
		[][]string{directCodingStructuredVitestCommand("src/feature.test.tsx")},
	)
	if err != nil {
		t.Fatal(err)
	}
	if diagnostic == nil {
		t.Fatal("real mixed Vitest failure did not produce one mapped diagnostic")
	}
	receipt, err := readDirectCodingVitestFailureReceipt(root)
	if err != nil {
		t.Fatal(err)
	}
	if receipt.FailureClass != directCodingStageFailureUnclassified || len(receipt.Failures) != 2 ||
		receipt.Failures[0].FailureClass != directCodingStageFailureVitestBehavior ||
		receipt.Failures[1].FailureClass != directCodingStageFailureUnclassified {
		t.Fatalf("real mixed failure receipt=%+v", receipt)
	}
	observation := receipt.Failures[0].AccessibilityObservation
	if observation == nil || observation.Schema != directCodingTestingLibraryRoleObservationSchemaV1 ||
		observation.RequestedRole != "button" ||
		observation.Visibility != directCodingTestingLibraryRoleVisibilityAccessible ||
		observation.Status != directCodingTestingLibraryRoleObservationStatusComplete ||
		observation.ElementCount != 1 || !reflect.DeepEqual(observation.Names, []string{"actual"}) {
		t.Fatalf("real behavior failure observation=%#v", observation)
	}
	if diagnostic.BlockID != "feature.render" {
		t.Fatalf("real behavior failure targeted %s", diagnostic.BlockID)
	}
	if !strings.HasPrefix(diagnostic.ModelFeedback, "TestingLibraryElementError: Unable to find") {
		t.Fatalf("real behavior failure lost its exact structured problem: %q", diagnostic.ModelFeedback)
	}
	if !strings.Contains(diagnostic.ModelFeedback, `plain text "restock" (case-insensitive)`) ||
		!strings.Contains(diagnostic.ModelFeedback, `computed accessible name exact text "actual"`) ||
		strings.Contains(diagnostic.ModelFeedback, "/restock/i") ||
		modelcontext.ContainsPathIdentity(diagnostic.ModelFeedback) {
		t.Fatalf("real behavior failure retained an unsafe regex matcher: %q", diagnostic.ModelFeedback)
	}
	failure, err := directCodingTypeScriptStageModelFeedback(diagnostic)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := assemblyline.NewFragmentRepairGuidanceJob(
		assemblyline.FragmentRepairGuidanceInput{
			Language: "typescript", Dialect: "TypeScript function syntax",
			Signature:          "function Feature(): ReactElement",
			CurrentDeclaration: program.Generated["feature.render"],
			Diagnostic:         failure,
		},
	); err != nil {
		t.Fatalf("real Vitest regex diagnostic did not reach repair guidance: %v", err)
	}
}

func TestApplicationTaskAndFullStageTypecheckBeforeStructuredVitest(t *testing.T) {
	_, frozen, program := applicationTaskLifecycleFixture(t)
	context, err := assemblyline.ProjectApplicationTaskContext(frozen, "task_001")
	if err != nil {
		t.Fatal(err)
	}
	stage, err := projectDirectCodingApplicationTaskStage(program, context)
	if err != nil {
		t.Fatal(err)
	}
	commands, err := directCodingApplicationTaskStageCommands(stage, context)
	if err != nil {
		t.Fatal(err)
	}
	wantTask := [][]string{
		{"run", "typecheck"},
		{"test", "--", "--reporter=./" + directCodingVitestReporterFile, "src/features/Feature001.test.tsx"},
	}
	if !reflect.DeepEqual(commands, wantTask) {
		t.Fatalf("task commands=%v want=%v", commands, wantTask)
	}
	wantFull := [][]string{
		{"run", "typecheck"},
		{"test", "--", "--reporter=./" + directCodingVitestReporterFile},
		{"run", "build"},
	}
	if got := directCodingFullStageCommands(); !reflect.DeepEqual(got, wantFull) {
		t.Fatalf("full commands=%v want=%v", got, wantFull)
	}
}

func TestVitestReportUsesStructuredErrorTypeForBehaviorRouting(t *testing.T) {
	root := t.TempDir()
	writeVitestReport := func(raw string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(root, directCodingVitestReportFile), []byte(raw), 0o600); err != nil {
			t.Fatal(err)
		}
	}

	for _, name := range []string{"AssertionError", "TestingLibraryElementError"} {
		t.Run(name, func(t *testing.T) {
			writeVitestReport(vitestFailureReport(name))
			classification, err := readDirectCodingVitestFailureClass(root)
			if err != nil || classification != directCodingStageFailureVitestBehavior {
				t.Fatalf("behavior classification=%q error=%v", classification, err)
			}
		})
	}
	for _, name := range []string{"ReferenceError", "TypeError", "Error"} {
		t.Run(name, func(t *testing.T) {
			writeVitestReport(vitestFailureReport(name))
			classification, err := readDirectCodingVitestFailureClass(root)
			if err != nil || classification != directCodingStageFailureUnclassified {
				t.Fatalf("runtime defect classification=%q error=%v", classification, err)
			}
		})
	}
}

func TestVitestReporterDeclaresOneV3AccessibilityObservationTransport(t *testing.T) {
	if strings.Count(directCodingVitestReporterSource, directCodingVitestReportSchema) != 1 {
		t.Fatalf("reporter source does not contain exactly one v3 schema: %q", directCodingVitestReporterSource)
	}
	for _, required := range []string{
		"omnidexTestingLibraryRoleObservation",
		"Object.prototype.propertyIsEnumerable.call",
		"accessibility_observation: accessibilityObservationRecord(error)",
	} {
		if !strings.Contains(directCodingVitestReporterSource, required) {
			t.Fatalf("reporter source omits %q", required)
		}
	}
}

func TestVitestReportCarriesValidatedTestingLibraryRoleObservation(t *testing.T) {
	root := t.TempDir()
	report := vitestFailureReportWithObservation(
		"TestingLibraryElementError",
		`{
			"schema":"omnidex.testing-library-role-observation.v1",
			"requested_role":"button","visibility":"accessible","status":"complete",
			"element_count":2,"names":["Save",""]
		}`,
	)
	if err := os.WriteFile(filepath.Join(root, directCodingVitestReportFile), []byte(report), 0o600); err != nil {
		t.Fatal(err)
	}
	receipt, err := readDirectCodingVitestFailureReceipt(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(receipt.Failures) != 1 {
		t.Fatalf("failure evidence count=%d want=1", len(receipt.Failures))
	}
	want := &directCodingTestingLibraryRoleObservation{
		Schema:        directCodingTestingLibraryRoleObservationSchemaV1,
		RequestedRole: "button",
		Visibility:    directCodingTestingLibraryRoleVisibilityAccessible,
		Status:        directCodingTestingLibraryRoleObservationStatusComplete,
		ElementCount:  2,
		Names:         []string{"Save", ""},
	}
	if got := receipt.Failures[0].AccessibilityObservation; !reflect.DeepEqual(got, want) {
		t.Fatalf("accessibility observation=%#v want=%#v", got, want)
	}
}

func TestVitestReportAcceptsBoundedNoncompleteRoleObservations(t *testing.T) {
	for _, fixture := range []struct {
		name   string
		status string
		count  string
	}{
		{name: "limit exceeded preserves actual count", status: "limit_exceeded", count: "101"},
		{name: "capture failed permits JS-safe maximum", status: "capture_failed", count: "9007199254740991"},
	} {
		t.Run(fixture.name, func(t *testing.T) {
			root := t.TempDir()
			observation := `{
				"schema":"omnidex.testing-library-role-observation.v1",
				"requested_role":"` + strings.Repeat("r", 64) + `",
				"visibility":"available","status":"` + fixture.status + `",
				"element_count":` + fixture.count + `,"names":[]
			}`
			if err := os.WriteFile(
				filepath.Join(root, directCodingVitestReportFile),
				[]byte(vitestFailureReportWithObservation("TestingLibraryElementError", observation)),
				0o600,
			); err != nil {
				t.Fatal(err)
			}
			receipt, err := readDirectCodingVitestFailureReceipt(root)
			if err != nil {
				t.Fatal(err)
			}
			if got := receipt.Failures[0].AccessibilityObservation; got == nil ||
				got.Status != directCodingTestingLibraryRoleObservationStatus(fixture.status) {
				t.Fatalf("accessibility observation=%#v", got)
			}
		})
	}
}

func TestVitestReportRejectsInvalidAccessibilityObservations(t *testing.T) {
	validPrefix := `"schema":"omnidex.testing-library-role-observation.v1",` +
		`"requested_role":"button","visibility":"accessible",`
	for _, fixture := range []struct {
		name      string
		errorName string
		body      string
	}{
		{name: "wrong error type", errorName: "AssertionError", body: `{` + validPrefix + `"status":"complete","element_count":0,"names":[]}`},
		{name: "nonexact error name", errorName: " TestingLibraryElementError ", body: `{` + validPrefix + `"status":"complete","element_count":0,"names":[]}`},
		{name: "missing required field", errorName: "TestingLibraryElementError", body: `{` + validPrefix + `"status":"complete","element_count":0}`},
		{name: "wrong schema", errorName: "TestingLibraryElementError", body: `{"schema":"wrong","requested_role":"button","visibility":"accessible","status":"complete","element_count":0,"names":[]}`},
		{name: "empty role", errorName: "TestingLibraryElementError", body: `{"schema":"omnidex.testing-library-role-observation.v1","requested_role":"","visibility":"accessible","status":"complete","element_count":0,"names":[]}`},
		{name: "overlong role", errorName: "TestingLibraryElementError", body: `{"schema":"omnidex.testing-library-role-observation.v1","requested_role":"` + strings.Repeat("r", 65) + `","visibility":"accessible","status":"complete","element_count":0,"names":[]}`},
		{name: "invalid visibility", errorName: "TestingLibraryElementError", body: `{"schema":"omnidex.testing-library-role-observation.v1","requested_role":"button","visibility":"hidden","status":"complete","element_count":0,"names":[]}`},
		{name: "invalid status", errorName: "TestingLibraryElementError", body: `{` + validPrefix + `"status":"partial","element_count":0,"names":[]}`},
		{name: "negative count", errorName: "TestingLibraryElementError", body: `{` + validPrefix + `"status":"capture_failed","element_count":-1,"names":[]}`},
		{name: "unsafe count", errorName: "TestingLibraryElementError", body: `{` + validPrefix + `"status":"limit_exceeded","element_count":9007199254740992,"names":[]}`},
		{name: "complete count over limit", errorName: "TestingLibraryElementError", body: `{` + validPrefix + `"status":"complete","element_count":101,"names":[]}`},
		{name: "complete count mismatch", errorName: "TestingLibraryElementError", body: `{` + validPrefix + `"status":"complete","element_count":1,"names":[]}`},
		{name: "noncomplete names", errorName: "TestingLibraryElementError", body: `{` + validPrefix + `"status":"limit_exceeded","element_count":101,"names":["Save"]}`},
		{name: "overlong name", errorName: "TestingLibraryElementError", body: `{` + validPrefix + `"status":"complete","element_count":1,"names":["` + strings.Repeat("n", 257) + `"]}`},
		{name: "name with NUL", errorName: "TestingLibraryElementError", body: `{` + validPrefix + `"status":"complete","element_count":1,"names":["bad\u0000name"]}`},
		{name: "unknown field", errorName: "TestingLibraryElementError", body: `{` + validPrefix + `"status":"complete","element_count":0,"names":[],"extra":true}`},
	} {
		t.Run(fixture.name, func(t *testing.T) {
			root := t.TempDir()
			if err := os.WriteFile(
				filepath.Join(root, directCodingVitestReportFile),
				[]byte(vitestFailureReportWithObservation(fixture.errorName, fixture.body)),
				0o600,
			); err != nil {
				t.Fatal(err)
			}
			if _, err := readDirectCodingVitestFailureReceipt(root); err == nil {
				t.Fatal("invalid accessibility observation was accepted")
			}
		})
	}
}

func TestVitestReportPreservesProviderParsedFrames(t *testing.T) {
	root := t.TempDir()
	report := strings.Replace(
		vitestFailureReport("AssertionError"),
		`"file":"src/feature.test.tsx"`,
		`"file":"`+filepath.ToSlash(filepath.Join(root, "src/feature.test.tsx"))+`"`,
		1,
	)
	if err := os.WriteFile(filepath.Join(root, directCodingVitestReportFile), []byte(report), 0o600); err != nil {
		t.Fatal(err)
	}
	receipt, err := readDirectCodingVitestFailureReceipt(root)
	if err != nil {
		t.Fatal(err)
	}
	want := []directCodingVitestSourceLocation{{
		File: filepath.ToSlash(filepath.Join(root, "src/feature.test.tsx")), Line: 12, Column: 3,
	}}
	if !reflect.DeepEqual(receipt.Locations, want) {
		t.Fatalf("locations=%#v want=%#v", receipt.Locations, want)
	}
	if !strings.Contains(receipt.Output, want[0].File+":12:3") {
		t.Fatalf("receipt output omitted parsed frame:\n%s", receipt.Output)
	}
}

func TestVitestReportRejectsMissingMalformedAndContradictoryReceipts(t *testing.T) {
	root := t.TempDir()
	if _, err := readDirectCodingVitestFailureClass(root); err == nil {
		t.Fatal("missing Vitest report was accepted")
	}
	for name, raw := range map[string]string{
		"malformed": `{`,
		"contradictory": `{
			"schema":"omnidex.vitest-report.v3","reason":"passed","unhandled_errors":[],"modules":[]
		}`,
		"incomplete error": `{
			"schema":"omnidex.vitest-report.v3","reason":"failed","unhandled_errors":[],
			"modules":[{"path":"src/feature.test.tsx","errors":[],"tests":[{
				"state":"failed","errors":[{"name":"AssertionError"}]
			}]}]
		}`,
	} {
		t.Run(name, func(t *testing.T) {
			if err := os.WriteFile(filepath.Join(root, directCodingVitestReportFile), []byte(raw), 0o600); err != nil {
				t.Fatal(err)
			}
			if _, err := readDirectCodingVitestFailureClass(root); err == nil {
				t.Fatal("invalid Vitest report was accepted")
			}
		})
	}
}

func vitestFailureReport(errorName string) string {
	return vitestFailureReportWithObservation(errorName, "")
}

func vitestFailureReportWithObservation(errorName, observation string) string {
	observationField := ""
	if observation != "" {
		observationField = `,"accessibility_observation":` + observation
	}
	return `{
		"schema":"omnidex.vitest-report.v3","reason":"failed","unhandled_errors":[],
		"modules":[{"path":"src/feature.test.tsx","errors":[],"tests":[{
			"state":"failed","errors":[{
				"name":"` + errorName + `","message":"observed failure","stack":"at Verify (src/feature.test.tsx:12:3)",
				"stacks":[{"method":"Verify","file":"src/feature.test.tsx","line":12,"column":3}]` + observationField + `
			}]
		}]}]
	}`
}
