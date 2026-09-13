package worker

import (
	"fmt"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/assemblyline"
)

func TestSourceCorrectionSieveSelectsUsableExpressionWithoutAnotherCall(t *testing.T) {
	for _, fixture := range []struct{ signature, behavior, expression string }{
		{"func Double(value int) int", "Return twice value.", "retained * 2"},
		{"func Enclose(value string) string", "Surround value with square brackets.", `"[" + retained + "]"`},
	} {
		input := assemblyline.FragmentGenerationInput{Language: "go", Dialect: "Go 1.24.0", Signature: fixture.signature, Behavior: fixture.behavior}
		body := "retained := value\nreturn broken(retained)"
		var key assemblyline.PortableJobKey
		calls := 0
		runtime := typedWorkerRuntime{
			Context: t.Context(), MaxAttempts: assemblyline.MaxSourceBodyAttempts,
			Execute: func(job assemblyline.PortableJob, _ string) (assemblyline.PortableResult, error) {
				calls++
				key = job.Key()
				return exactSourceBodyTestResult(t, job, body), nil
			},
			Correct: func(job assemblyline.PortableJob, model string, correction assemblyline.SourceBodyCorrection) (assemblyline.PortableResult, error) {
				calls++
				if job.Key() != key || model != "fixture-model" || correction.Mutable() != "broken(retained)" {
					return assemblyline.PortableResult{}, fmt.Errorf("correction changed its authority")
				}
				prompt, err := correction.ModelInput()
				if err != nil || strings.Contains(prompt, "retained := value") || strings.Contains(prompt, fixture.signature) {
					return assemblyline.PortableResult{}, fmt.Errorf("correction leaked accepted source")
				}
				return exactSourceBodyTestResult(t, job, "```go\nunavailable(retained)\n```\n```go\n"+fixture.expression+"\n```"), nil
			},
			Release:  func(assemblyline.PortableJob) error { return nil },
			Finalize: func(assemblyline.PortableJob, assemblyline.PortableResult, error) error { return nil },
		}
		validate := func(input assemblyline.FragmentGenerationInput, candidate string) (string, error) {
			source, err := validateDirectCodingGoFragment(input, candidate)
			start := strings.Index(candidate, "broken(retained)")
			if start < 0 || err == nil {
				return source, err
			}
			defect, defectErr := assemblyline.NewSourceBodyDefect(candidate, start, start+len("broken(retained)"), "What expression produces the required value from retained?", err)
			if defectErr != nil {
				return "", defectErr
			}
			return "", defect
		}
		source, err := runDirectCodingLanguageFragmentWorker(runtime, "fixture-model", directCodingLanguageGenerationJob{Subject: "value.001", Input: input, Validate: validate})
		if err != nil || calls != 2 || !strings.Contains(source, "retained := value") || !strings.Contains(source, "return "+fixture.expression) {
			t.Fatalf("calls=%d source=%s error=%v", calls, source, err)
		}
	}
}
