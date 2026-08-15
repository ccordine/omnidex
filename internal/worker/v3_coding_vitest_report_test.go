package worker

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/assemblyline"
)

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
