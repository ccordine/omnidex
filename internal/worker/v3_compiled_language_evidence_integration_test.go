package worker

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/assemblyline"
	"github.com/gryph/omnidex/internal/queue"
	workspacefacts "github.com/gryph/omnidex/internal/workspace"
)

func TestCompiledLanguagesRecordRealStageAndHostResults(t *testing.T) {
	databaseURL := strings.TrimSpace(os.Getenv("OMNI_TEST_DATABASE_URL"))
	if databaseURL == "" {
		t.Skip("OMNI_TEST_DATABASE_URL is required for compiler execution evidence")
	}
	for _, stackID := range []string{genericJavaScriptCommandLineAdapter, genericRustCommandLineAdapter, genericJavaCommandLineAdapter} {
		for _, broken := range []bool{false, true} {
			name := stackID + "/success"
			if broken {
				name = stackID + "/compiler-failure"
			}
			t.Run(name, func(t *testing.T) {
				rejectNativeCompiledLanguageTools(t)
				_, repository := freshWorkerEvidenceRepository(t, databaseURL)
				ctx := context.Background()
				root := t.TempDir()
				job, err := repository.EnqueueCodingJob(ctx, "exercise actual compiler evidence", root)
				if err != nil {
					t.Fatal(err)
				}
				claim, err := repository.ClaimNextStep(ctx, "compiled-language-worker")
				if err != nil || claim == nil || claim.Job.ID != job.ID {
					t.Fatalf("claim=%#v err=%v", claim, err)
				}
				access := workspacefacts.NewHostDirectoryAccess("/tmp")
				program := testCompiledLanguageVerificationProgram(t, stackID)
				if broken {
					breakCompiledLanguageFixture(&program)
				}
				runtime := &nativeRuntimeV3{ctx: ctx, claim: claim, svc: &Service{
					repo: repository, hostDirectoryAccess: access,
					runtimeEventChannels: make(map[int64]runtimeEventChannelBinding),
				}}
				session := &directCodingSession{runtime: runtime, root: root, program: &program}
				if err := runtime.acquireWorkspaceMutationFence(root); err != nil {
					t.Fatal(err)
				}
				t.Cleanup(func() {
					if err := runtime.releaseWorkspaceMutationFence(); err != nil {
						t.Error(err)
					}
				})
				scratchParent := t.TempDir()
				t.Setenv("TMPDIR", scratchParent)
				bodies := program.Generated
				program.Generated = make(map[string]string)
				factory := program.Project.Stack.NewSourceGenerator
				program.Project.Stack.NewSourceGenerator = func(session *directCodingSession, selected directCodingProgram) (directCodingProjectSourceGenerator, error) {
					generator, err := factory(session, selected)
					if err != nil {
						return nil, err
					}
					executor := generator.(*directCodingCompiledLanguageExecutor)
					return &compiledLanguageFixtureExecutor{directCodingCompiledLanguageExecutor: executor, bodies: bodies}, nil
				}
				t.Cleanup(func() {
					if entries, err := os.ReadDir(scratchParent); err != nil || len(entries) != 0 {
						t.Errorf("compiler output or stage cache remains: %v, %v", entries, err)
					}
				})
				err = session.runDirectCodingApplicationTaskLifecycle(program.Workload, &program)
				if broken {
					if err == nil || !strings.Contains(err.Error(), "exited") {
						t.Fatalf("actual compiler failure was not reported: %v", err)
					}
					entries, err := os.ReadDir(root)
					if err != nil || len(entries) != 0 {
						t.Fatalf("failed stage changed authoritative workspace: %v, %v", entries, err)
					}
				} else {
					if err != nil {
						t.Fatalf("task/final compilation: %v", err)
					}
					assembly, err := directCodingAssemblyFromProgram(program)
					if err != nil {
						t.Fatal(err)
					}
					prepared, err := session.PrepareAssembly(assembly)
					if err != nil {
						t.Fatal(err)
					}
					if err := session.ApplyAndVerify(prepared); err != nil {
						t.Fatalf("authoritative compilation: %v", err)
					}
					if err := validateDirectCodingAssemblyAtRoot(root, assembly); err != nil {
						t.Fatal(err)
					}
				}
				records := compiledLanguageCommandEvidence(t, repository, job.ID)
				phases := make(map[queue.VerificationCommandPhase]int)
				for index, record := range records {
					phases[record.Phase]++
					if record.DurationNanos < 0 || !record.StdoutComplete || !record.StderrComplete || record.ExitCode == nil {
						t.Fatalf("command %d has incomplete process evidence: %+v", index, record)
					}
					want := queue.VerificationCommandSucceeded
					if broken && index == len(records)-1 {
						want = queue.VerificationCommandExitFailed
						if len(record.Stderr) == 0 || *record.ExitCode == 0 {
							t.Fatal("failed compiler has no actual diagnostic and nonzero exit")
						}
						wantDiagnostic := map[string]string{
							genericJavaScriptCommandLineAdapter: "ERR_MODULE_NOT_FOUND",
							genericRustCommandLineAdapter:       "mismatched types",
							genericJavaCommandLineAdapter:       "incompatible types",
						}[stackID]
						if !strings.Contains(string(record.Stderr), wantDiagnostic) {
							t.Fatalf("compiler failed for another reason: %s", record.Stderr)
						}
					}
					if record.Status != want {
						t.Fatalf("command %v status=%s; want %s", record.Argv, record.Status, want)
					}
					if record.WorkingDirectory != "/workspace" || record.ContainerID == "" || record.ContainerImageID == "" || record.ContainerExecID == "" {
						t.Fatal("compiled command evidence lacks its actual Docker execution authority")
					}
					if broken && directCodingVerificationPhaseUsesHostRoot(record.Phase) {
						t.Fatal("failed stage reached host commands")
					}
					t.Logf("%s: %v exited %d", record.Phase, record.Argv[:min(3, len(record.Argv))], *record.ExitCode)
				}
				for _, phase := range []queue.VerificationCommandPhase{queue.VerificationIsolatedInstall, queue.VerificationIsolatedTask} {
					if phases[phase] == 0 {
						t.Fatalf("no actual commands for %s", phase)
					}
				}
				if !broken && (phases[queue.VerificationIsolatedFinal] == 0 || phases[queue.VerificationHostFinal] == 0) {
					t.Fatal("successful assembly skipped final or authoritative commands")
				}
				assertCompiledLanguageContainersRemoved(t, records)
			})
		}
	}
}

// Within this explicitly constructed workload, only source generation is
// replaced. The production factory, lifecycle, verification, and cleanup run.
type compiledLanguageFixtureExecutor struct {
	*directCodingCompiledLanguageExecutor
	bodies map[string]string
}

func (executor *compiledLanguageFixtureExecutor) GenerateBlock(_ assemblyline.ApplicationTaskContext, _ *directCodingProgram, ref assemblyline.SourceBlockRef) (string, error) {
	return executor.bodies[ref.Block.ID], nil
}

func (executor *compiledLanguageFixtureExecutor) VerifyFinal(program *directCodingProgram) error {
	if err := executor.directCodingCompiledLanguageExecutor.VerifyFinal(program); err != nil {
		return err
	}
	// This external fixture expectation proves entrypoint wiring, not general
	// requirement understanding or production behavioral acceptance.
	workspace := executor.workspace
	command := compiledLanguageFixtureRunCommand(program.Project.Stack.ID, workspace.output)
	result, err := workspace.run(queue.VerificationIsolatedFinal, command)
	if err != nil || string(result.Stdout) != "ready\n" || len(result.Stderr) != 0 {
		return fmt.Errorf("compiled fixture output=%q stderr=%q err=%v", result.Stdout, result.Stderr, err)
	}
	return nil
}

func compiledLanguageFixtureRunCommand(stackID, output string) directCodingVerificationCommand {
	var argv []string
	switch stackID {
	case genericJavaScriptCommandLineAdapter:
		argv = []string{"node", "main.mjs"}
	case genericRustCommandLineAdapter:
		argv = []string{filepath.Join(output, "target", "debug", directCodingPackageName)}
	case genericJavaCommandLineAdapter:
		argv = []string{"java", "-jar", filepath.Join(output, "application.jar")}
	}
	return directCodingVerificationCommand{Argv: argv, Timeout: defaultDirectCodingVerificationTimeout}
}

func compiledLanguageCommandEvidence(t *testing.T, repository *queue.Repository, jobID int64) []queue.VerificationCommandEvidence {
	t.Helper()
	var records []queue.VerificationCommandEvidence
	after := int64(0)
	for {
		page, err := repository.ListVerificationCommandEvidenceForJob(context.Background(), jobID, after, 10)
		if err != nil {
			t.Fatal(err)
		}
		if len(page) == 0 {
			return records
		}
		records = append(records, page...)
		after = page[len(page)-1].ID
	}
}

func breakCompiledLanguageFixture(program *directCodingProgram) {
	switch program.Project.Stack.ID {
	case genericJavaScriptCommandLineAdapter:
		for index, document := range program.Source.Documents {
			if strings.HasSuffix(document.Path, ".mjs") && filepath.Base(document.Path) != "runtime.mjs" {
				program.Source.Documents[index].Preamble = strings.ReplaceAll(document.Preamble, "./runtime.mjs", "./missing.mjs")
			}
		}
	case genericRustCommandLineAdapter:
		program.Generated["feature.001"] = strings.ReplaceAll(program.Generated["feature.001"], `String::from("ready")`, "42")
	case genericJavaCommandLineAdapter:
		program.Generated["feature.001"] = strings.ReplaceAll(program.Generated["feature.001"], `Runtime.result("ready"`, "Runtime.result(42")
	}
}
