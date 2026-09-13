package worker

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/queue"
)

// Supplied test declarations prove publication and verification mechanics.
// They provide no evidence of intent interpretation or autonomous construction.
func runConstructedDockerPublicationFixture(t *testing.T, program directCodingProgram, broken, clientOwned bool) []queue.VerificationCommandEvidence {
	t.Helper()
	databaseURL := strings.TrimSpace(os.Getenv("OMNI_TEST_DATABASE_URL"))
	if databaseURL == "" {
		t.Skip("OMNI_TEST_DATABASE_URL is required for Docker publication evidence")
	}
	rejectNativeCompiledLanguageTools(t)
	_, repository := freshWorkerEvidenceRepository(t, databaseURL)
	ctx := context.Background()
	root := t.TempDir()
	for _, directory := range []string{"node_modules", "dist", ".vite"} {
		if err := os.Mkdir(filepath.Join(root, directory), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(root, directory, "retained.txt"), []byte("user data"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	runtime := constructedPublicationRuntime(t, repository, ctx, root, clientOwned)
	job := runtime.claim.Job
	session := &directCodingSession{root: root, runtime: runtime, program: &program}
	declarations := program.Generated
	program.Generated = make(map[string]string)
	factory := program.Project.Stack.NewSourceGenerator
	program.Project.Stack.NewSourceGenerator = func(session *directCodingSession, selected directCodingProgram) (directCodingProjectSourceGenerator, error) {
		generator, err := factory(session, selected)
		if err != nil {
			return nil, err
		}
		if browser, ok := generator.(*directCodingTypeScriptProjectStageExecutor); ok {
			return &browserDockerFixtureExecutor{directCodingTypeScriptProjectStageExecutor: browser, declarations: declarations}, nil
		}
		return &compiledLanguageBehaviorFixtureExecutor{directCodingProjectSourceGenerator: generator, declarations: declarations}, nil
	}
	err := session.runDirectCodingApplicationTaskLifecycle(program.Workload, &program)
	if broken {
		if err == nil || !strings.Contains(err.Error(), "exited") {
			t.Fatalf("actual behavioral failure was not reported: %v", err)
		}
		if len(session.publishedAssembly.Files) != 0 || len(session.mutationJournal) != 0 {
			t.Fatal("failed task published source")
		}
		for _, path := range program.TargetTree.Paths {
			if _, err := os.Lstat(filepath.Join(root, filepath.FromSlash(path))); !os.IsNotExist(err) {
				t.Fatalf("failed source %s reached host: %v", path, err)
			}
		}
	} else {
		if err != nil {
			t.Fatalf("Docker task lifecycle: %v", err)
		}
		if len(session.publishedAssembly.Files) == 0 || len(session.mutationJournal) == 0 {
			t.Fatal("verified task files were not published progressively")
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
			t.Fatalf("verify authoritative source through Docker: %v", err)
		}
		if err := validateDirectCodingAssemblyAtRoot(root, assembly); err != nil {
			t.Fatal(err)
		}
	}
	for _, directory := range []string{"node_modules", "dist", ".vite"} {
		entries, err := os.ReadDir(filepath.Join(root, directory))
		if err != nil || len(entries) != 1 || entries[0].Name() != "retained.txt" {
			t.Fatalf("host tool output directory changed: %s %v %v", directory, entries, err)
		}
		content, err := os.ReadFile(filepath.Join(root, directory, "retained.txt"))
		if err != nil || string(content) != "user data" {
			t.Fatalf("host marker changed: %s %q %v", directory, content, err)
		}
	}
	records := compiledLanguageCommandEvidence(t, repository, job.ID)
	phases := make(map[queue.VerificationCommandPhase]int)
	failures := 0
	for _, record := range records {
		phases[record.Phase]++
		if record.ContainerID == "" || record.ContainerImageID == "" || record.ContainerExecID == "" || record.ContainerNetworkEnabled == nil || record.WorkingDirectory != "/workspace" || record.ExitCode == nil || !record.StdoutComplete || !record.StderrComplete {
			t.Fatalf("incomplete Docker process evidence: %+v", record)
		}
		if record.Status == queue.VerificationCommandExitFailed {
			failures++
		} else if record.Status != queue.VerificationCommandSucceeded {
			t.Fatalf("unexpected command failure: %+v", record)
		}
		if *record.ContainerNetworkEnabled && record.Phase != queue.VerificationIsolatedInstall && record.Phase != queue.VerificationHostInstall {
			t.Fatalf("application command retained acquisition network: %+v", record)
		}
		if broken && directCodingVerificationPhaseUsesHostRoot(record.Phase) {
			t.Fatal("failed stage reached published-source verification")
		}
	}
	if broken && failures != 1 {
		t.Fatalf("expected one observed behavioral failure; received %d", failures)
	}
	if !broken && (failures != 0 || phases[queue.VerificationIsolatedFinal] == 0 || phases[queue.VerificationHostFinal] == 0) {
		t.Fatalf("successful lifecycle omitted final verification: %v failures=%d", phases, failures)
	}
	assertCompiledLanguageContainersRemoved(t, records)
	return records
}
