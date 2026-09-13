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

// Constructed source proves the production publication/verification mechanics,
// not interpretation of a user request or autonomous application construction.
func TestVerifiedTaskFilesSurviveIndependentBehavioralFailure(t *testing.T) {
	databaseURL := strings.TrimSpace(os.Getenv("OMNI_TEST_DATABASE_URL"))
	if databaseURL == "" {
		t.Skip("OMNI_TEST_DATABASE_URL is required for verified publication evidence")
	}
	for _, failingTask := range []int{1, 2} {
		t.Run(fmt.Sprintf("failing_task_%d", failingTask), func(t *testing.T) {
			_, repository := freshWorkerEvidenceRepository(t, databaseURL)
			root, scratch := t.TempDir(), t.TempDir()
			t.Setenv("TMPDIR", scratch)
			if err := os.WriteFile(filepath.Join(root, "user.txt"), []byte("retained user data\n"), 0o600); err != nil {
				t.Fatal(err)
			}
			ctx := context.Background()
			job, err := repository.EnqueueCodingJob(ctx, "exercise constructed publication fixtures", root)
			if err != nil {
				t.Fatal(err)
			}
			claim, err := repository.ClaimNextStep(ctx, "publication-fixture")
			if err != nil || claim == nil || claim.Job.ID != job.ID {
				t.Fatalf("claim=%#v err=%v", claim, err)
			}
			fixtures := []compiledLanguageBehaviorFixture{
				{"Return the number of command-line arguments as text.",
					`return normalizeTaskResult({ output: String(input.arguments.length), error: "", exitCode: 0, state: {} });`,
					`const result = run({ arguments: ["one", "two"], standardInput: "" }, {}); assert.strictEqual(result.output, "2");`},
				{"Return standard-input text converted to uppercase.",
					`return normalizeTaskResult({ output: input.standardInput.toUpperCase(), error: "", exitCode: 0, state: {} });`,
					`const result = run({ arguments: [], standardInput: "Mixed" }, {}); assert.strictEqual(result.output, "MIXED");`},
				{"Return the first command-line argument as text.",
					`return normalizeTaskResult({ output: input.arguments[0], error: "", exitCode: 0, state: {} });`,
					`const result = run({ arguments: ["supplied"], standardInput: "" }, {}); assert.strictEqual(result.output, "supplied");`},
			}
			fixtures[failingTask-1].implementation = `return normalizeTaskResult({ output: "incorrect", error: "", exitCode: 0, state: {} });`
			program := compiledLanguageBehaviorWorkloadFixture(t, genericJavaScriptCommandLineAdapter, fixtures,
				directCodingCapabilityGraph{"requirement_001": nil, "requirement_002": nil, "requirement_003": nil})
			declarations := program.Generated
			program.Generated = make(map[string]string)
			runtime := &nativeRuntimeV3{ctx: ctx, claim: claim, svc: &Service{
				repo: repository, hostDirectoryAccess: workspacefacts.NewHostDirectoryAccess(root),
			}}
			if err := runtime.acquireWorkspaceMutationFence(root); err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() {
				if err := runtime.releaseWorkspaceMutationFence(); err != nil {
					t.Error(err)
				}
			})
			session := &directCodingSession{root: root, runtime: runtime, program: &program}
			factory := program.Project.Stack.NewSourceGenerator
			program.Project.Stack.NewSourceGenerator = func(session *directCodingSession, selected directCodingProgram) (directCodingProjectSourceGenerator, error) {
				generator, err := factory(session, selected)
				if err != nil {
					return nil, err
				}
				return &publicationFixtureExecutor{
					directCodingProjectSourceGenerator: generator, declarations: declarations,
					before: func(context assemblyline.ApplicationTaskContext) {
						for sequence := 1; sequence <= 3; sequence++ {
							if fmt.Sprintf("task_%03d", sequence) >= context.Task.TaskID {
								break
							}
							publicationFixtureFile(t, root, sequence, sequence != failingTask)
						}
					},
				}, nil
			}
			err = session.runDirectCodingApplicationTaskLifecycle(program.Workload, &program)
			if err == nil || !strings.Contains(err.Error(), "exited") || !strings.Contains(err.Error(), fmt.Sprintf("task_%03d", failingTask)) {
				t.Fatalf("actual assertion failure was not reported: %v", err)
			}
			for sequence := 1; sequence <= 3; sequence++ {
				publicationFixtureFile(t, root, sequence, sequence != failingTask)
			}
			if len(program.Generated) != 4 || len(session.mutationJournal) == 0 {
				t.Fatalf("independent accepted source did not reach the workspace: bodies=%d mutations=%d", len(program.Generated), len(session.mutationJournal))
			}
			if err := validateDirectCodingAssemblyAtRoot(root, session.publishedAssembly); err != nil {
				t.Fatal(err)
			}
			if _, err := os.Lstat(filepath.Join(root, "main.mjs")); !os.IsNotExist(err) {
				t.Fatal("unverified application composition was published")
			}
			content, err := os.ReadFile(filepath.Join(root, "user.txt"))
			if err != nil || string(content) != "retained user data\n" {
				t.Fatalf("unrelated user file changed: %q %v", content, err)
			}
			failures, combined := 0, 0
			for _, record := range compiledLanguageCommandEvidence(t, repository, job.ID) {
				if record.Phase == queue.VerificationIsolatedFinal || directCodingVerificationPhaseUsesHostRoot(record.Phase) {
					t.Fatalf("incomplete workload claimed complete verification: %+v", record)
				}
				if record.Status == queue.VerificationCommandExitFailed {
					failures++
				}
				if len(record.Argv) == 5 && record.Argv[0] == "node" && record.Argv[1] == "--test" && record.Status == queue.VerificationCommandSucceeded {
					combined++
				}
			}
			if failures != 1 || combined != 1 {
				t.Fatalf("missing failed-task or combined-publication evidence: failures=%d combined=%d", failures, combined)
			}
			if entries, err := os.ReadDir(scratch); err != nil || len(entries) != 0 {
				t.Fatalf("experiment left temporary files: %v %v", entries, err)
			}
		})
	}
}

func publicationFixtureFile(t *testing.T, root string, sequence int, present bool) {
	t.Helper()
	for _, suffix := range []string{".mjs", ".test.mjs"} {
		path := fmt.Sprintf("feature%03d%s", sequence, suffix)
		content, err := os.ReadFile(filepath.Join(root, path))
		if present && (err != nil || len(content) == 0) {
			t.Fatalf("verified file %s is unavailable before later work: %v", path, err)
		}
		if !present && !os.IsNotExist(err) {
			t.Fatalf("failed file %s was published: %v", path, err)
		}
	}
}

type publicationFixtureExecutor struct {
	directCodingProjectSourceGenerator
	declarations map[string]string
	before       func(assemblyline.ApplicationTaskContext)
}

func (executor *publicationFixtureExecutor) GenerateBlock(context assemblyline.ApplicationTaskContext, _ *directCodingProgram, ref assemblyline.SourceBlockRef) (string, error) {
	executor.before(context)
	return executor.declarations[ref.Block.ID], nil
}
