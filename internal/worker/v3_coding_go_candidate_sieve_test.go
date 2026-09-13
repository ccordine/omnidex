package worker

import (
	"fmt"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/assemblyline"
)

func TestGoCandidateSieveRetainsUsableBodyWithoutAnotherModelCall(t *testing.T) {
	for _, fixture := range []struct{ name, signature, behavior, body string }{
		{"numeric", "func Double(value int) int", "Return twice value.", "return value * 2"},
		{"text", "func Enclose(value string) string", "Surround value with square brackets.", `return "[" + value + "]"`},
	} {
		t.Run(fixture.name, func(t *testing.T) {
			input := assemblyline.FragmentGenerationInput{
				Language: "go", Dialect: "Go 1.24.0", Signature: fixture.signature, Behavior: fixture.behavior,
			}
			response := "```go\n" + fixture.signature + " { return missing(value) }\n```\n" +
				"```go\n" + fixture.signature + " { " + fixture.body + " }\n```"
			calls, accepted := 0, 0
			runtime := typedWorkerRuntime{
				Context: t.Context(), MaxAttempts: assemblyline.MaxSourceBodyAttempts,
				Execute: func(job assemblyline.PortableJob, _ string) (assemblyline.PortableResult, error) {
					calls++
					return exactSourceBodyTestResult(t, job, response), nil
				},
				Correct: func(assemblyline.PortableJob, string, assemblyline.SourceBodyCorrection) (assemblyline.PortableResult, error) {
					return assemblyline.PortableResult{}, fmt.Errorf("a usable candidate must not cause another model call")
				},
				Release: func(assemblyline.PortableJob) error { return nil },
				Finalize: func(job assemblyline.PortableJob, result assemblyline.PortableResult, err error) error {
					if err != nil {
						return err
					}
					body, err := assemblyline.ExtractFragmentGenerationSourceBody(job, result.Candidate)
					if err != nil || body != fixture.body {
						return fmt.Errorf("retained source %q differs from selected body: %v", body, err)
					}
					accepted++
					return nil
				},
			}
			source, err := runDirectCodingLanguageFragmentWorker(runtime, "fixture-model", directCodingLanguageGenerationJob{
				Subject: "source.001", Input: input, Validate: validateDirectCodingGoFragment,
			})
			if err != nil || !strings.Contains(source, fixture.body) || calls != 1 || accepted != 1 {
				t.Fatalf("source=%s error=%v calls=%d accepted=%d", source, err, calls, accepted)
			}
		})
	}
}
