package worker

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/queue"
)

func TestJavaScriptBehavioralFailuresStopBeforeWorkspaceWrites(t *testing.T) {
	databaseURL := strings.TrimSpace(os.Getenv("OMNI_TEST_DATABASE_URL"))
	if databaseURL == "" {
		t.Skip("OMNI_TEST_DATABASE_URL is required for real behavioral execution evidence")
	}
	for _, fixture := range []struct{ name, requirement, implementation, verification string }{
		{
			"arithmetic", "Return the sum of the two decimal command-line arguments as text.",
			`return normalizeTaskResult({ output: String(Number(input.arguments[0]) + Number(input.arguments[1])), error: "", exitCode: 0, state: {} });`,
			`const result = run({ arguments: ["8", "-3"], standardInput: "" }, {}); assert.strictEqual(result.output, "5");`,
		},
		{
			"text", "Return the complete standard-input text converted to uppercase.",
			`return normalizeTaskResult({ output: input.standardInput.toUpperCase(), error: "", exitCode: 0, state: {} });`,
			`const result = run({ arguments: [], standardInput: "Mixed Case" }, {}); assert.strictEqual(result.output, "MIXED CASE");`,
		},
	} {
		for _, broken := range []bool{false, true} {
			name := fixture.name + "/success"
			if broken {
				name = fixture.name + "/wrong-result"
			}
			t.Run(name, func(t *testing.T) {
				_, repository := freshWorkerEvidenceRepository(t, databaseURL)
				root, scratch := t.TempDir(), t.TempDir()
				t.Setenv("TMPDIR", scratch)
				ctx := context.Background()
				job, err := repository.EnqueueCodingJob(ctx, "exercise constructed behavioral fixture", root)
				if err != nil {
					t.Fatal(err)
				}
				claim, err := repository.ClaimNextStep(ctx, "behavior-fixture-worker")
				if err != nil || claim == nil || claim.Job.ID != job.ID {
					t.Fatalf("claim=%#v err=%v", claim, err)
				}
				implementation := fixture.implementation
				if broken {
					implementation = `return normalizeTaskResult({ output: "incorrect", error: "", exitCode: 0, state: {} });`
				}
				program := compiledLanguageBehaviorWorkloadFixture(t, genericJavaScriptCommandLineAdapter,
					[]compiledLanguageBehaviorFixture{{fixture.requirement, implementation, fixture.verification}}, directCodingCapabilityGraph{"requirement_001": nil})
				declarations := program.Generated
				program.Generated = make(map[string]string)
				factory := program.Project.Stack.NewSourceGenerator
				program.Project.Stack.NewSourceGenerator = func(session *directCodingSession, selected directCodingProgram) (directCodingProjectSourceGenerator, error) {
					executor, err := factory(session, selected)
					if err != nil {
						return nil, err
					}
					return &compiledLanguageBehaviorFixtureExecutor{directCodingProjectSourceGenerator: executor, declarations: declarations}, nil
				}
				session := &directCodingSession{root: root, program: &program, runtime: &nativeRuntimeV3{ctx: ctx, claim: claim, svc: &Service{repo: repository}}}
				err = session.runDirectCodingApplicationTaskLifecycle(program.Workload, &program)
				if (err != nil) != broken {
					t.Fatalf("broken=%t lifecycle err=%v", broken, err)
				}
				if broken && len(program.Generated) != 0 {
					t.Fatal("wrong result became accepted source")
				}
				records := compiledLanguageCommandEvidence(t, repository, job.ID)
				tests := 0
				for _, record := range records {
					if len(record.Argv) < 2 || record.Argv[0] != "node" || record.Argv[1] != "--test" {
						if record.Status != queue.VerificationCommandSucceeded {
							t.Fatalf("fixture failed before its behavioral test: %+v", record)
						}
						continue
					}
					tests++
					if !record.StdoutComplete || !record.StderrComplete || record.ExitCode == nil || !strings.Contains(string(record.Stdout), "feature.001") {
						t.Fatalf("behavioral command has incomplete observation: %+v", record)
					}
					if broken {
						if record.Phase != queue.VerificationIsolatedTask || record.Status != queue.VerificationCommandExitFailed || *record.ExitCode != 1 || !strings.Contains(string(record.Stdout), "ERR_ASSERTION") {
							t.Fatalf("wrong result was not an actual assertion failure: %+v", record)
						}
					} else if record.Status != queue.VerificationCommandSucceeded || *record.ExitCode != 0 {
						t.Fatalf("correct behavior failed its assertion: %+v", record)
					}
				}
				wantTests := 2
				if broken {
					wantTests = 1
				}
				if tests != wantTests {
					t.Fatalf("ran %d behavioral tests; want %d", tests, wantTests)
				}
				for _, directory := range []string{root, scratch} {
					if entries, err := os.ReadDir(directory); err != nil || len(entries) != 0 {
						t.Fatalf("stage wrote authoritative files or leaked temporary data: %v %v", entries, err)
					}
				}
			})
		}
	}
}
