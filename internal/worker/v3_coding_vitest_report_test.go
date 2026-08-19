package worker

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/assemblyline"
)

func TestRealVitestBehaviorFailureRoutesToImplementationOwner(t *testing.T) {
	if os.Getenv("OMNIDEX_NODE_INTEGRATION") != "1" {
		t.Skip("set OMNIDEX_NODE_INTEGRATION=1 to install the pinned Vitest toolchain")
	}
	blueprint := assemblyline.TypeScriptBlueprint{Documents: []assemblyline.TypeScriptDocument{
		{
			ID: "feature", Path: "src/feature.tsx",
			Header: "import type { ReactElement } from 'react';",
			Blocks: []assemblyline.TypeScriptBlock{{
				ID: "feature.render", Signature: "function Feature(): ReactElement",
				Contract: "Render the observable value.", API: "function Feature(): ReactElement",
				Export: true,
			}},
		},
		{
			ID: "acceptance", Path: "src/feature.test.tsx",
			Header: `import '@testing-library/jest-dom/vitest';
import React from 'react';
import { render, screen } from '@testing-library/react';
import { Feature } from './feature';`,
			Blocks: []assemblyline.TypeScriptBlock{
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
	for _, file := range typeScriptBrowserStaticFiles("vitest-route", "Vitest route", "") {
		if file.Path == "package.json" || file.Path == "tsconfig.json" || file.Path == "vite.config.ts" {
			staticFiles = append(staticFiles, file)
		}
	}
	program := directCodingProgram{
		Adapter: "browser_typescript", PackageName: "vitest-route", TypeScript: blueprint,
		StaticFiles: staticFiles,
		Generated: map[string]string{
			"feature.render":     `function Feature(): ReactElement { return <p>actual</p>; }`,
			"feature.acceptance": `async function Verify(): Promise<void> { expect(screen.getByText('expected')).toBeInTheDocument(); }`,
		},
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
	if diagnostic.BlockID != "feature.render" {
		t.Fatalf("real behavior failure targeted %s", diagnostic.BlockID)
	}
	if !strings.HasPrefix(diagnostic.ModelFeedback, "TestingLibraryElementError: Unable to find") {
		t.Fatalf("real behavior failure lost its exact structured problem: %q", diagnostic.ModelFeedback)
	}
}

func TestApplicationTaskAndFullStageTypecheckBeforeStructuredVitest(t *testing.T) {
	input, frozen, program := applicationTaskLifecycleFixture(t)
	context, err := assemblyline.ProjectApplicationTaskContext(input, frozen, "task_001")
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
			"schema":"omnidex.vitest-report.v2","reason":"passed","unhandled_errors":[],"modules":[]
		}`,
		"incomplete error": `{
			"schema":"omnidex.vitest-report.v2","reason":"failed","unhandled_errors":[],
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
	return `{
		"schema":"omnidex.vitest-report.v2","reason":"failed","unhandled_errors":[],
		"modules":[{"path":"src/feature.test.tsx","errors":[],"tests":[{
			"state":"failed","errors":[{
				"name":"` + errorName + `","message":"observed failure","stack":"at Verify (src/feature.test.tsx:12:3)",
				"stacks":[{"method":"Verify","file":"src/feature.test.tsx","line":12,"column":3}]
			}]
		}]}]
	}`
}
