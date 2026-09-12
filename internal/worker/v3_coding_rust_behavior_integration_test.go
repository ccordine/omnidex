package worker

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/queue"
)

func TestRustBehavioralFailuresStopBeforeWorkspaceWrites(t *testing.T) {
	databaseURL := strings.TrimSpace(os.Getenv("OMNI_TEST_DATABASE_URL"))
	if databaseURL == "" {
		t.Skip("OMNI_TEST_DATABASE_URL is required for real Rust behavioral execution evidence")
	}
	for _, fixture := range []struct{ name, requirement, implementation, verification string }{
		{
			"arithmetic", "Return the sum of the two decimal command-line arguments as text.",
			`let total = input.arguments[0].parse::<i64>().unwrap() + input.arguments[1].parse::<i64>().unwrap(); TaskResult { output: total.to_string(), ..TaskResult::default() }`,
			`let result = run(&TaskInput { arguments: vec!["8".to_string(), "-3".to_string()], standard_input: String::new() }, &CapabilityResults::new()); assert_eq!(result.output, "5");`,
		},
		{
			"text", "Return the complete standard-input text converted to uppercase.",
			`TaskResult { output: input.standard_input.to_uppercase(), ..TaskResult::default() }`,
			`let result = run(&TaskInput { arguments: vec![], standard_input: String::from("Mixed Case") }, &CapabilityResults::new()); assert_eq!(result.output, "MIXED CASE");`,
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
				job, err := repository.EnqueueCodingJob(ctx, "exercise constructed Rust behavioral fixture", root)
				if err != nil {
					t.Fatal(err)
				}
				claim, err := repository.ClaimNextStep(ctx, "rust-behavior-fixture-worker")
				if err != nil || claim == nil || claim.Job.ID != job.ID {
					t.Fatalf("claim=%#v err=%v", claim, err)
				}
				implementation := fixture.implementation
				if broken {
					implementation = `TaskResult { output: String::from("incorrect"), ..TaskResult::default() }`
				}
				program := compiledLanguageBehaviorWorkloadFixture(t, genericRustCommandLineAdapter,
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
					t.Fatal("wrong Rust result became accepted source")
				}
				tests := 0
				for _, record := range compiledLanguageCommandEvidence(t, repository, job.ID) {
					if len(record.Argv) < 2 || record.Argv[0] != "cargo" || record.Argv[1] != "test" {
						if record.Status != queue.VerificationCommandSucceeded {
							t.Fatalf("fixture failed before its behavioral test: %+v", record)
						}
						continue
					}
					tests++
					if !record.StdoutComplete || !record.StderrComplete || record.ExitCode == nil || !strings.Contains(string(record.Stdout), "feature001_test::checks_feature_001") {
						t.Fatalf("Rust test has incomplete observation: %+v", record)
					}
					if broken {
						if record.Phase != queue.VerificationIsolatedTask || record.Status != queue.VerificationCommandExitFailed || *record.ExitCode != 101 || !strings.Contains(string(record.Stdout), "assertion `left == right` failed") {
							t.Fatalf("wrong result was not an actual Rust assertion failure: %+v", record)
						}
					} else if record.Status != queue.VerificationCommandSucceeded || *record.ExitCode != 0 {
						t.Fatalf("correct Rust behavior failed: %+v", record)
					}
				}
				wantTests := 2
				if broken {
					wantTests = 1
				}
				if tests != wantTests {
					t.Fatalf("ran %d Rust behavioral tests; want %d", tests, wantTests)
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
