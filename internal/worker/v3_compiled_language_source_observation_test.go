package worker

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/queue"
)

func TestDockerCompiledSourceObservationRejectsSuccessfulMutation(t *testing.T) {
	databaseURL := strings.TrimSpace(os.Getenv("OMNI_TEST_DATABASE_URL"))
	if databaseURL == "" {
		t.Skip("OMNI_TEST_DATABASE_URL is required for Docker source observation")
	}
	for _, fixture := range []struct{ name, script string }{
		{"source", `require("node:fs").appendFileSync("runtime.mjs", "\n// changed\n")`},
		{"manifest", `require("node:fs").writeFileSync("package.json", "{}\n")`},
	} {
		t.Run(fixture.name, func(t *testing.T) {
			rejectNativeCompiledLanguageTools(t)
			_, repository := freshWorkerEvidenceRepository(t, databaseURL)
			ctx := context.Background()
			root := t.TempDir()
			job, err := repository.EnqueueCodingJob(ctx, "exercise constructed source observation", root)
			if err != nil {
				t.Fatal(err)
			}
			claim, err := repository.ClaimNextStep(ctx, "docker-source-observation")
			if err != nil || claim == nil || claim.Job.ID != job.ID {
				t.Fatalf("claim=%+v, %v", claim, err)
			}
			program := testCompiledLanguageVerificationProgram(t, genericJavaScriptCommandLineAdapter)
			session := &directCodingSession{root: root, program: &program, runtime: &nativeRuntimeV3{ctx: ctx, claim: claim, svc: &Service{repo: repository}}}
			workspace, err := newDirectCodingCompiledLanguageWorkspace(session, program)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() {
				if err := workspace.Close(); err != nil {
					t.Error(err)
				}
			})
			if err := workspace.VerifyFinal(&program); err != nil {
				t.Fatal(err)
			}
			assembly, err := directCodingAssemblyFromProgram(program)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := workspace.run(queue.VerificationIsolatedFinal, directCodingVerificationCommand{Argv: []string{"node", "-e", fixture.script}, Timeout: defaultDirectCodingVerificationTimeout}); err != nil {
				t.Fatal(err)
			}
			if err := workspace.validateSource(assembly); err == nil || !strings.Contains(err.Error(), "changed during command execution") {
				t.Fatalf("mutated checked source was not rejected: %v", err)
			}
			if entries, err := os.ReadDir(root); err != nil || len(entries) != 0 {
				t.Fatalf("unverified source reached host workspace: %v %v", entries, err)
			}
			if err := workspace.Close(); err != nil {
				t.Fatal(err)
			}
			assertCompiledLanguageContainersRemoved(t, compiledLanguageCommandEvidence(t, repository, job.ID))
		})
	}
}
