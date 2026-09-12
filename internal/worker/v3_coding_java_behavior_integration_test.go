package worker

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/queue"
)

func TestJavaBehavioralFailuresStopBeforeWorkspaceWrites(t *testing.T) {
	databaseURL := strings.TrimSpace(os.Getenv("OMNI_TEST_DATABASE_URL"))
	if databaseURL == "" {
		t.Skip("OMNI_TEST_DATABASE_URL is required for real Java behavioral execution evidence")
	}
	for _, fixture := range []struct{ name, requirement, implementation, verification string }{
		{
			"arithmetic", "Return the sum of the two decimal command-line arguments as text.",
			`List<?> arguments = (List<?>) input.get("arguments"); int total = Integer.parseInt((String) arguments.get(0)) + Integer.parseInt((String) arguments.get(1)); return Runtime.result(Integer.toString(total), "", 0, Map.of());`,
			`Map<String, Object> result = run(Map.of("arguments", List.of("8", "-3"), "standardInput", ""), Map.of()); assert result.get("output").equals("5");`,
		},
		{
			"text", "Return the complete standard-input text converted to uppercase.",
			`return Runtime.result(((String) input.get("standardInput")).toUpperCase(), "", 0, Map.of());`,
			`Map<String, Object> result = run(Map.of("arguments", List.of(), "standardInput", "Mixed Case"), Map.of()); assert result.get("output").equals("MIXED CASE");`,
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
				job, err := repository.EnqueueCodingJob(ctx, "exercise constructed Java behavioral fixture", root)
				if err != nil {
					t.Fatal(err)
				}
				claim, err := repository.ClaimNextStep(ctx, "java-behavior-fixture-worker")
				if err != nil || claim == nil || claim.Job.ID != job.ID {
					t.Fatalf("claim=%#v err=%v", claim, err)
				}
				implementation := fixture.implementation
				if broken {
					implementation = `return Runtime.result("incorrect", "", 0, Map.of());`
				}
				program := compiledLanguageBehaviorWorkloadFixture(t, genericJavaCommandLineAdapter,
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
					t.Fatal("wrong Java result became accepted source")
				}
				tests := 0
				for _, record := range compiledLanguageCommandEvidence(t, repository, job.ID) {
					if len(record.Argv) < 2 || record.Argv[0] != "java" || record.Argv[1] != "-ea" {
						if record.Status != queue.VerificationCommandSucceeded {
							t.Fatalf("fixture failed before its behavioral test: %+v", record)
						}
						continue
					}
					tests++
					if !record.StdoutComplete || !record.StderrComplete || record.ExitCode == nil || record.Argv[len(record.Argv)-1] != "Feature001Test" {
						t.Fatalf("Java test has incomplete observation: %+v", record)
					}
					if broken {
						if record.Phase != queue.VerificationIsolatedTask || record.Status != queue.VerificationCommandExitFailed || *record.ExitCode != 1 || !strings.Contains(string(record.Stderr), "java.lang.AssertionError") || !strings.Contains(string(record.Stderr), "Feature001Test.verifyFeature001") {
							t.Fatalf("wrong result was not an actual Java assertion failure: %+v", record)
						}
					} else if record.Status != queue.VerificationCommandSucceeded || *record.ExitCode != 0 || string(record.Stdout) != "verifyFeature001 passed\n" {
						t.Fatalf("correct Java behavior failed: %+v", record)
					}
				}
				wantTests := 2
				if broken {
					wantTests = 1
				}
				if tests != wantTests {
					t.Fatalf("ran %d Java behavioral tests; want %d", tests, wantTests)
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
