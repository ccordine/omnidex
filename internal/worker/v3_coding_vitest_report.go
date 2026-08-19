package worker

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const (
	directCodingVitestReporterFile   = ".omnidex-vitest-reporter.mjs"
	directCodingVitestReportFile     = ".omnidex-vitest-report.json"
	directCodingVitestReportSchemaV2 = "omnidex.vitest-report.v2"
	maxDirectCodingVitestReportBytes = 1024 * 1024
)

const directCodingVitestReporterSource = `import { writeFile } from 'node:fs/promises';

const reportURL = new URL('./.omnidex-vitest-report.json', import.meta.url);

function errorRecord(error) {
  return {
    name: typeof error?.name === 'string' ? error.name : '',
    message: typeof error?.message === 'string' ? error.message : '',
    stack: typeof error?.stack === 'string' ? error.stack : '',
    stacks: Array.isArray(error?.stacks) ? error.stacks
      .filter((frame) => typeof frame?.file === 'string' && frame.file.length > 0 &&
        Number.isInteger(frame?.line) && frame.line > 0 &&
        Number.isInteger(frame?.column) && frame.column > 0)
      .map((frame) => ({
        method: typeof frame?.method === 'string' ? frame.method : '',
        file: frame.file,
        line: frame.line,
        column: frame.column,
      })) : [],
  };
}

export default class OmnidexVitestReporter {
  async onTestRunEnd(testModules, unhandledErrors, reason) {
    const modules = [];
    for (const testModule of testModules) {
      const tests = [];
      for (const testCase of testModule.children.allTests()) {
        const result = testCase.result();
        tests.push({
          state: result.state,
          errors: (result.errors ?? []).map(errorRecord),
        });
      }
      modules.push({
        path: testModule.relativeModuleId,
        errors: testModule.errors().map(errorRecord),
        tests,
      });
    }
    await writeFile(reportURL, JSON.stringify({
      schema: 'omnidex.vitest-report.v2',
      reason,
      unhandled_errors: unhandledErrors.map(errorRecord),
      modules,
    }), { encoding: 'utf8' });
  }
}
`

type directCodingStageFailureClass string

const (
	directCodingStageFailureUnclassified   directCodingStageFailureClass = ""
	directCodingStageFailureVitestBehavior directCodingStageFailureClass = "vitest_behavior"
)

type directCodingVitestFailureReceipt struct {
	FailureClass directCodingStageFailureClass
	Output       string
	Locations    []directCodingVitestSourceLocation
	Failures     []directCodingVitestFailureEvidence
}

type directCodingVitestFailureEvidence struct {
	FailureClass directCodingStageFailureClass
	Name         string
	Message      string
	Output       string
	Locations    []directCodingVitestSourceLocation
}

type directCodingVitestSourceLocation struct {
	File   string
	Line   int
	Column int
}

type directCodingVitestReport struct {
	Schema          *string                           `json:"schema"`
	Reason          *string                           `json:"reason"`
	UnhandledErrors *[]directCodingVitestErrorRecord  `json:"unhandled_errors"`
	Modules         *[]directCodingVitestModuleRecord `json:"modules"`
}

type directCodingVitestModuleRecord struct {
	Path   *string                          `json:"path"`
	Errors *[]directCodingVitestErrorRecord `json:"errors"`
	Tests  *[]directCodingVitestTestRecord  `json:"tests"`
}

type directCodingVitestTestRecord struct {
	State  *string                          `json:"state"`
	Errors *[]directCodingVitestErrorRecord `json:"errors"`
}

type directCodingVitestErrorRecord struct {
	Name    *string                          `json:"name"`
	Message *string                          `json:"message"`
	Stack   *string                          `json:"stack"`
	Stacks  *[]directCodingVitestStackRecord `json:"stacks"`
}

type directCodingVitestStackRecord struct {
	Method *string `json:"method"`
	File   *string `json:"file"`
	Line   *int    `json:"line"`
	Column *int    `json:"column"`
}

func directCodingStructuredVitestCommand(path string) []string {
	args := []string{"test", "--", "--reporter=./" + directCodingVitestReporterFile}
	if path != "" {
		args = append(args, path)
	}
	return args
}

func directCodingStageCommandUsesVitestReport(args []string) bool {
	want := directCodingStructuredVitestCommand("")
	if len(args) != len(want) && len(args) != len(want)+1 {
		return false
	}
	for index := range want {
		if args[index] != want[index] {
			return false
		}
	}
	return len(args) == len(want) || strings.TrimSpace(args[len(want)]) == args[len(want)] && args[len(want)] != ""
}

func writeDirectCodingVitestReporter(root string) error {
	if root == "" || !filepath.IsAbs(root) {
		return fmt.Errorf("write structured Vitest reporter requires one absolute stage root")
	}
	if err := os.WriteFile(
		filepath.Join(root, directCodingVitestReporterFile),
		[]byte(directCodingVitestReporterSource),
		0o600,
	); err != nil {
		return fmt.Errorf("write structured Vitest reporter: %w", err)
	}
	return nil
}

func clearDirectCodingVitestReport(root string) error {
	if root == "" || !filepath.IsAbs(root) {
		return fmt.Errorf("clear structured Vitest report requires one absolute stage root")
	}
	if err := os.Remove(filepath.Join(root, directCodingVitestReportFile)); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("clear structured Vitest report: %w", err)
	}
	return nil
}

func readDirectCodingVitestFailureClass(root string) (directCodingStageFailureClass, error) {
	receipt, err := readDirectCodingVitestFailureReceipt(root)
	return receipt.FailureClass, err
}

func readDirectCodingVitestFailureReceipt(root string) (directCodingVitestFailureReceipt, error) {
	var zero directCodingVitestFailureReceipt
	if root == "" || !filepath.IsAbs(root) {
		return zero, fmt.Errorf("read structured Vitest report requires one absolute stage root")
	}
	raw, err := os.ReadFile(filepath.Join(root, directCodingVitestReportFile))
	if err != nil {
		return zero, fmt.Errorf("read structured Vitest report: %w", err)
	}
	if len(raw) == 0 || len(raw) > maxDirectCodingVitestReportBytes {
		return zero, fmt.Errorf("structured Vitest report must contain 1..%d bytes", maxDirectCodingVitestReportBytes)
	}
	var report directCodingVitestReport
	if err := json.Unmarshal(raw, &report); err != nil {
		return zero, fmt.Errorf("decode structured Vitest report: %w", err)
	}
	if report.Schema == nil || report.Reason == nil || report.UnhandledErrors == nil || report.Modules == nil {
		return zero, fmt.Errorf("structured Vitest report omits required fields")
	}
	if *report.Schema != directCodingVitestReportSchemaV2 {
		return zero, fmt.Errorf("structured Vitest report schema must be %q", directCodingVitestReportSchemaV2)
	}
	if *report.Reason != "failed" && *report.Reason != "interrupted" {
		return zero, fmt.Errorf("structured Vitest failure has contradictory reason %q", *report.Reason)
	}

	output := make([]string, 0)
	locations := make([]directCodingVitestSourceLocation, 0)
	failures := make([]directCodingVitestFailureEvidence, 0)
	unhandledCount := len(*report.UnhandledErrors)
	for index, failure := range *report.UnhandledErrors {
		evidence, err := decodeDirectCodingVitestFailureEvidence(
			failure, directCodingStageFailureUnclassified,
		)
		if err != nil {
			return zero, fmt.Errorf("structured Vitest unhandled error %d: %w", index, err)
		}
		failures = append(failures, evidence)
		output = append(output, evidence.Output)
		locations = append(locations, evidence.Locations...)
	}
	moduleErrorCount := 0
	failedTests := make([]directCodingVitestTestRecord, 0)
	for moduleIndex, module := range *report.Modules {
		if module.Path == nil || strings.TrimSpace(*module.Path) == "" || module.Errors == nil || module.Tests == nil {
			return zero, fmt.Errorf("structured Vitest module %d is incomplete", moduleIndex)
		}
		for errorIndex, failure := range *module.Errors {
			moduleErrorCount++
			evidence, err := decodeDirectCodingVitestFailureEvidence(
				failure, directCodingStageFailureUnclassified,
			)
			if err != nil {
				return zero, fmt.Errorf("structured Vitest module error %d.%d: %w", moduleIndex, errorIndex, err)
			}
			failures = append(failures, evidence)
			output = append(output, evidence.Output)
			locations = append(locations, evidence.Locations...)
		}
		for testIndex, test := range *module.Tests {
			if test.State == nil || test.Errors == nil {
				return zero, fmt.Errorf("structured Vitest test %d.%d is incomplete", moduleIndex, testIndex)
			}
			switch *test.State {
			case "passed", "skipped", "pending":
			case "failed":
				failedTests = append(failedTests, test)
			default:
				return zero, fmt.Errorf("structured Vitest test %d.%d has unsupported state %q", moduleIndex, testIndex, *test.State)
			}
			for errorIndex, failure := range *test.Errors {
				failureClass := directCodingStageFailureUnclassified
				if *report.Reason == "failed" && *test.State == "failed" &&
					directCodingVitestBehaviorError(failure) {
					failureClass = directCodingStageFailureVitestBehavior
				}
				evidence, err := decodeDirectCodingVitestFailureEvidence(failure, failureClass)
				if err != nil {
					return zero, fmt.Errorf("structured Vitest test error %d.%d.%d: %w", moduleIndex, testIndex, errorIndex, err)
				}
				failures = append(failures, evidence)
				output = append(output, evidence.Output)
				locations = append(locations, evidence.Locations...)
			}
		}
	}

	classification := directCodingStageFailureUnclassified
	if *report.Reason == "failed" && unhandledCount == 0 && moduleErrorCount == 0 && len(failedTests) == 1 {
		errors := *failedTests[0].Errors
		if len(errors) == 1 && directCodingVitestBehaviorError(errors[0]) {
			classification = directCodingStageFailureVitestBehavior
		}
	}
	return directCodingVitestFailureReceipt{
		FailureClass: classification,
		Output:       strings.Join(output, "\n"),
		Locations:    locations,
		Failures:     failures,
	}, nil
}

func decodeDirectCodingVitestFailureEvidence(
	failure directCodingVitestErrorRecord,
	failureClass directCodingStageFailureClass,
) (directCodingVitestFailureEvidence, error) {
	if failure.Name == nil || failure.Message == nil {
		return directCodingVitestFailureEvidence{}, fmt.Errorf("error record omits name or message")
	}
	name := strings.TrimSpace(*failure.Name)
	message := strings.TrimSpace(*failure.Message)
	if name == "" || message == "" {
		return directCodingVitestFailureEvidence{}, fmt.Errorf(
			"error record name and message must both be non-empty",
		)
	}
	output := make([]string, 0, 2)
	locations := make([]directCodingVitestSourceLocation, 0)
	if err := appendDirectCodingVitestError(&output, &locations, failure); err != nil {
		return directCodingVitestFailureEvidence{}, err
	}
	return directCodingVitestFailureEvidence{
		FailureClass: failureClass,
		Name:         name,
		Message:      message,
		Output:       strings.Join(output, "\n"),
		Locations:    locations,
	}, nil
}

func appendDirectCodingVitestError(
	output *[]string,
	locations *[]directCodingVitestSourceLocation,
	failure directCodingVitestErrorRecord,
) error {
	if failure.Name == nil || failure.Message == nil || failure.Stack == nil || failure.Stacks == nil {
		return fmt.Errorf("error record omits name, message, stack, or parsed stacks")
	}
	if strings.TrimSpace(*failure.Message) != "" {
		*output = append(*output, *failure.Message)
	}
	if strings.TrimSpace(*failure.Stack) != "" {
		*output = append(*output, *failure.Stack)
	}
	for index, frame := range *failure.Stacks {
		if frame.Method == nil || frame.File == nil || frame.Line == nil || frame.Column == nil {
			return fmt.Errorf("parsed stack frame %d is incomplete", index)
		}
		file := strings.TrimSpace(*frame.File)
		if file == "" || *frame.Line <= 0 || *frame.Column <= 0 {
			return fmt.Errorf("parsed stack frame %d has invalid source coordinates", index)
		}
		location := directCodingVitestSourceLocation{File: file, Line: *frame.Line, Column: *frame.Column}
		*locations = append(*locations, location)
		*output = append(*output, fmt.Sprintf("%s:%d:%d", location.File, location.Line, location.Column))
	}
	return nil
}

func directCodingVitestBehaviorError(failure directCodingVitestErrorRecord) bool {
	if failure.Name == nil {
		return false
	}
	switch *failure.Name {
	case "AssertionError", "TestingLibraryElementError":
		return true
	default:
		return false
	}
}
