package worker

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/assemblyline"
	"github.com/gryph/omnidex/internal/queue"
)

func TestTypeScriptStageRejectsSourceChangedBySuccessfulCommand(t *testing.T) {
	databaseURL := strings.TrimSpace(os.Getenv("OMNI_TEST_DATABASE_URL"))
	if databaseURL == "" {
		t.Skip("OMNI_TEST_DATABASE_URL is required for exact staged-source evidence")
	}
	for _, path := range []string{"src/example.ts", "package.json"} {
		t.Run(path, func(t *testing.T) {
			rejectNativeCompiledLanguageTools(t)
			_, repository := freshWorkerEvidenceRepository(t, databaseURL)
			ctx := context.Background()
			root := t.TempDir()
			job, err := repository.EnqueueCodingJob(ctx, "exercise constructed staged-source boundary", root)
			if err != nil {
				t.Fatal(err)
			}
			claim, err := repository.ClaimNextStep(ctx, "staged-source-fixture")
			if err != nil || claim == nil || claim.Job.ID != job.ID {
				t.Fatalf("claim=%#v err=%v", claim, err)
			}
			program := testTypeScriptBrowserProgram(t, "source fixture", "Display the supplied value.")
			program.Source = assemblyline.SourceBlueprint{Documents: []assemblyline.SourceDocument{{
				ID: "fixture", Path: "src/example.ts", AdapterID: "typescript",
				Blocks: []assemblyline.SourceBlock{{ID: "fixture.value", Static: "export const value = 1;", API: "export const value = 1;"}},
			}}}
			program.TargetTree.Paths = []string{"src/example.ts"}
			// Only the stage's exact-source check is under test. This fixture
			// needs neither dependency installation nor generated behavior.
			session := &directCodingSession{root: root, runtime: &nativeRuntimeV3{ctx: ctx, claim: claim, svc: &Service{repo: repository}}}
			container, err := openDirectCodingExperiment(session, program.Project.Profile)
			if err != nil {
				t.Fatal(err)
			}
			workspace := &directCodingTypeScriptStageWorkspace{root: "/workspace", profile: program.Project.Profile, session: session, experiment: container}
			t.Cleanup(func() {
				if err := workspace.Close(); err != nil {
					t.Error(err)
				}
			})
			err = workspace.Verify(&program, queue.VerificationIsolatedTask, []directCodingVerificationCommand{{
				Argv:    []string{"node", "-e", `require('node:fs').appendFileSync(process.argv[1], '\n// changed after validation\n')`, path},
				Timeout: defaultDirectCodingVerificationTimeout,
			}})
			if err == nil || !strings.Contains(err.Error(), "revalidate exact isolated TypeScript source") || !strings.Contains(err.Error(), path) {
				t.Fatalf("successful command substituted staged source: %v", err)
			}
			records := compiledLanguageCommandEvidence(t, repository, job.ID)
			if len(records) != 2 || records[0].Status != queue.VerificationCommandSucceeded || records[1].Status != queue.VerificationCommandSucceeded {
				t.Fatalf("fixture failed before exact-source validation: %+v", records)
			}
			if entries, err := os.ReadDir(root); err != nil || len(entries) != 0 {
				t.Fatalf("invalid experiment wrote authoritative files: %v %v", entries, err)
			}
			if err := workspace.Close(); err != nil {
				t.Fatal(err)
			}
			assertCompiledLanguageContainersRemoved(t, records)
		})
	}
}
