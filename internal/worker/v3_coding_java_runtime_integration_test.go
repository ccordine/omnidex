package worker

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/queue"
)

func TestJavaNativeRuntimeRejectsDisabledAssertionsAndMissingInput(t *testing.T) {
	databaseURL := strings.TrimSpace(os.Getenv("OMNI_TEST_DATABASE_URL"))
	if databaseURL == "" {
		t.Skip("OMNI_TEST_DATABASE_URL is required for native Java boundary evidence")
	}
	_, repository := freshWorkerEvidenceRepository(t, databaseURL)
	ctx := context.Background()
	root, scratch := t.TempDir(), t.TempDir()
	t.Setenv("TMPDIR", scratch)
	job, err := repository.EnqueueCodingJob(ctx, "exercise constructed Java runtime boundaries", root)
	if err != nil {
		t.Fatal(err)
	}
	claim, err := repository.ClaimNextStep(ctx, "java-runtime-fixture")
	if err != nil || claim == nil || claim.Job.ID != job.ID {
		t.Fatalf("claim=%#v err=%v", claim, err)
	}
	program := javaDependentBehaviorFixture(t)
	session := &directCodingSession{root: root, program: &program, runtime: &nativeRuntimeV3{ctx: ctx, claim: claim, svc: &Service{repo: repository}}}
	selected, err := program.Project.Stack.NewSourceGenerator(session, program)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := selected.Close(); err != nil {
			t.Error(err)
		}
		for _, directory := range []string{root, scratch} {
			if entries, err := os.ReadDir(directory); err != nil || len(entries) != 0 {
				t.Errorf("native verification leaked data: %v %v", entries, err)
			}
		}
	}()
	fixtureExecutor := compiledLanguageBehaviorFixtureExecutor{directCodingProjectSourceGenerator: selected, declarations: program.Generated}
	program.Generated = make(map[string]string)
	if err := runDirectCodingApplicationTaskLifecycle(program.Workload, &program, directCodingApplicationTaskLifecycleHooks{
		BuildBlock: fixtureExecutor.GenerateBlock, VerifyTask: selected.VerifyTask, FinalStage: selected.VerifyFinal,
	}); err != nil {
		t.Fatal(err)
	}
	executor := selected.(*directCodingCompiledLanguageExecutor)
	classes := filepath.Join(executor.workspace.output, "classes")
	result, err := session.runRecordedVerificationCommand(executor.workspace.source, queue.VerificationIsolatedFinal, directCodingVerificationCommand{
		Argv: []string{"java", "-cp", classes, "Feature001Test"}, Timeout: defaultDirectCodingVerificationTimeout,
	})
	if err == nil || !strings.Contains(string(result.Stderr), "behavioral verification requires enabled assertions") || len(result.Stdout) != 0 {
		t.Fatalf("disabled assertions produced apparent success: %+v %v", result, err)
	}

	// This code-owned fixture calls the actual runtime boundary. It is not
	// generated application behavior or evidence of intent understanding.
	boundaryPath := filepath.Join(t.TempDir(), "InputBoundaryTest.java")
	boundarySource := `@SuppressWarnings("auxiliaryclass")
final class InputBoundaryTest {
  public static void main(String[] arguments) {
    for (int missing = 0; missing < 2; missing++) {
      try {
        Runtime.input(missing == 0 ? null : new String[0], missing == 1 ? null : "");
        throw new AssertionError("missing input was silently substituted");
      } catch (IllegalArgumentException expected) {
        if (!expected.getMessage().equals("application input requires arguments and standardInput")) {
          throw new AssertionError("wrong input failure", expected);
        }
      }
    }
    if (!Runtime.input(new String[]{"value"}, "text").get("standardInput").equals("text")) {
      throw new AssertionError("valid input changed");
    }
    System.out.println("input boundary passed");
  }
}`
	if err := os.WriteFile(boundaryPath, []byte(boundarySource), 0o600); err != nil {
		t.Fatal(err)
	}
	release, err := directCodingVersionComponent(program.Project.Profile, "java_release")
	if err != nil {
		t.Fatal(err)
	}
	for _, argv := range [][]string{
		{"javac", "--release", release, "-encoding", "UTF-8", "-Xlint:all", "-Werror", "-cp", classes, "-d", classes, boundaryPath},
		{"java", "-ea", "-cp", classes, "InputBoundaryTest"},
	} {
		result, err = session.runRecordedVerificationCommand(executor.workspace.source, queue.VerificationIsolatedFinal, directCodingVerificationCommand{Argv: argv, Timeout: defaultDirectCodingVerificationTimeout})
		if err != nil {
			t.Fatalf("native input boundary: %+v %v", result, err)
		}
	}
	if string(result.Stdout) != "input boundary passed\n" {
		t.Fatalf("input boundary did not execute: %+v", result)
	}
	tests, disabled := 0, 0
	for _, record := range compiledLanguageCommandEvidence(t, repository, job.ID) {
		if record.ExitCode == nil || !record.StdoutComplete || !record.StderrComplete {
			t.Fatalf("incomplete native record: %+v", record)
		}
		if len(record.Argv) == 5 && record.Argv[0] == "java" && record.Argv[1] == "-ea" && strings.HasPrefix(record.Argv[4], "Feature") {
			tests++
			wantPhase := queue.VerificationIsolatedTask
			if tests > 2 {
				wantPhase = queue.VerificationIsolatedFinal
			}
			if *record.ExitCode != 0 || record.Phase != wantPhase {
				t.Fatalf("dependent native test failed: %+v", record)
			}
		}
		if len(record.Argv) == 4 && record.Argv[0] == "java" && record.Argv[1] == "-cp" {
			disabled++
			if *record.ExitCode != 1 || record.Status != queue.VerificationCommandExitFailed {
				t.Fatalf("disabled assertions were not a recorded failure: %+v", record)
			}
		}
	}
	if tests != 4 || disabled != 1 {
		t.Fatalf("native checks: task/final=%d disabled=%d", tests, disabled)
	}
}
