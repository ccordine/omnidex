package worker

import (
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/assemblyline"
)

func TestApplicationTaskLifecycleRejectsMissingVerificationHooks(t *testing.T) {
	for _, missing := range []string{"task", "complete"} {
		t.Run(missing, func(t *testing.T) {
			program := testCompiledLanguageVerificationProgram(t, genericRustCommandLineAdapter)
			bodies := program.Generated
			program.Generated = make(map[string]string)
			calls := 0
			hooks := directCodingApplicationTaskLifecycleHooks{
				BuildBlock: func(_ assemblyline.ApplicationTaskContext, _ *directCodingProgram, ref assemblyline.SourceBlockRef) (string, error) {
					calls++
					return bodies[ref.Block.ID], nil
				},
				VerifyTask:  func(assemblyline.ApplicationTaskContext, *directCodingProgram) error { calls++; return nil },
				FinalStage:  func(*directCodingProgram) error { calls++; return nil },
				PublishTask: func(*directCodingProgram, *directCodingProgram) error { calls++; return nil },
			}
			if missing == "task" {
				hooks.VerifyTask = nil
			} else {
				hooks.FinalStage = nil
			}
			err := runDirectCodingApplicationTaskLifecycle(program.Workload, &program, hooks)
			if err == nil || !strings.Contains(err.Error(), "verification hooks") || calls != 0 || len(program.Generated) != 0 {
				t.Fatalf("missing %s verification advanced work: err=%v calls=%d generated=%d", missing, err, calls, len(program.Generated))
			}
		})
	}
}
