package worker

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/assemblyline"
	"github.com/gryph/omnidex/internal/queue"
)

func TestJavaScriptTaskVerificationPreservesIndependentAcceptedWork(t *testing.T) {
	databaseURL := strings.TrimSpace(os.Getenv("OMNI_TEST_DATABASE_URL"))
	if databaseURL == "" {
		t.Skip("OMNI_TEST_DATABASE_URL is required for real task-isolation evidence")
	}
	for _, broken := range []bool{false, true} {
		t.Run(fmt.Sprintf("broken=%t", broken), func(t *testing.T) {
			_, repository := freshWorkerEvidenceRepository(t, databaseURL)
			root, scratch := t.TempDir(), t.TempDir()
			t.Setenv("TMPDIR", scratch)
			ctx := context.Background()
			job, err := repository.EnqueueCodingJob(ctx, "exercise constructed dependent-task fixture", root)
			if err != nil {
				t.Fatal(err)
			}
			claim, err := repository.ClaimNextStep(ctx, "javascript-isolation-fixture")
			if err != nil || claim == nil || claim.Job.ID != job.ID {
				t.Fatalf("claim=%#v err=%v", claim, err)
			}
			fixtures := []compiledLanguageBehaviorFixture{
				{
					"Return the first command-line argument as text.",
					`return normalizeTaskResult({ output: input.arguments[0], error: "", exitCode: 0, state: {} });`,
					`const result = run({ arguments: ["Mixed"], standardInput: "" }, {}); assert.strictEqual(result.output, "Mixed");`,
				},
				{
					"Return the supplied text result converted to uppercase.",
					`return normalizeTaskResult({ output: dependencies[feature002Capability001].output.toUpperCase(), error: "", exitCode: 0, state: {} });`,
					`const result = run({ arguments: [], standardInput: "" }, { capability_001: { output: "Mixed", error: "", exitCode: 0, state: {} } }); assert.strictEqual(result.output, "MIXED");`,
				},
				{
					"Return the number of command-line arguments as text.",
					`return normalizeTaskResult({ output: String(input.arguments.length), error: "", exitCode: 0, state: {} });`,
					`const result = run({ arguments: ["one", "two"], standardInput: "" }, {}); assert.strictEqual(result.output, "2");`,
				},
			}
			if broken {
				fixtures[1].implementation = `return normalizeTaskResult({ output: "incorrect", error: "", exitCode: 0, state: {} });`
			}
			program := compiledLanguageBehaviorWorkloadFixture(t, genericJavaScriptCommandLineAdapter, fixtures, directCodingCapabilityGraph{
				"requirement_001": nil,
				"requirement_002": {{RequirementID: "requirement_001", CapabilityID: "capability_001", Purpose: "Supplied text."}},
				"requirement_003": nil,
			})
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
			err = runIsolatedCompiledLanguageFixtureLifecycle(session, &program)
			if (err != nil) != broken {
				t.Fatalf("broken=%t lifecycle err=%v", broken, err)
			}
			for sequence := 1; sequence <= len(fixtures); sequence++ {
				for _, prefix := range []string{"feature", "acceptance"} {
					id := fmt.Sprintf("%s.%03d", prefix, sequence)
					want := declarations[id]
					if broken && sequence == 2 {
						want = ""
					}
					if program.Generated[id] != want {
						t.Fatalf("task-local outcome changed %s: %q", id, program.Generated[id])
					}
				}
			}
			var tests []queue.VerificationCommandEvidence
			for _, record := range compiledLanguageCommandEvidence(t, repository, job.ID) {
				if len(record.Argv) >= 2 && record.Argv[0] == "node" && record.Argv[1] == "--test" {
					tests = append(tests, record)
				}
			}
			wantRuns := 4
			if broken {
				wantRuns = 3
			}
			if len(tests) != wantRuns {
				t.Fatalf("got %d Node runs; want %d", len(tests), wantRuns)
			}
			for index, record := range tests[:3] {
				if record.Phase != queue.VerificationIsolatedTask || len(record.Argv) != 4 || record.Argv[3] != fmt.Sprintf("feature%03d.test.mjs", index+1) {
					t.Fatalf("task ran unrelated tests: %+v", record)
				}
				wantStatus := queue.VerificationCommandSucceeded
				if broken && index == 1 {
					wantStatus = queue.VerificationCommandExitFailed
				}
				if record.Status != wantStatus {
					t.Fatalf("unexpected task verification: %+v", record)
				}
			}
			if !broken && (tests[3].Phase != queue.VerificationIsolatedFinal || tests[3].Status != queue.VerificationCommandSucceeded || len(tests[3].Argv) != 6) {
				t.Fatalf("complete workload omitted its tests: %+v", tests[3])
			}
			for _, directory := range []string{root, scratch} {
				if entries, err := os.ReadDir(directory); err != nil || len(entries) != 0 {
					t.Fatalf("verification wrote authoritative files or leaked temporary data: %v %v", entries, err)
				}
			}
		})
	}
}

func TestJavaScriptDependentVerificationReceivesOnlyItsDirectInterface(t *testing.T) {
	pair := directCodingTaskArtifactPair{ImplementationPath: "feature002.mjs", VerificationPath: "feature002.test.mjs"}
	document := javaScriptCommandLineAcceptanceDocument(2, "task.002", "Observe the supplied result.\nDirect capability capability_001: Supplied text.", pair)
	program := directCodingProgram{Project: testCompiledLanguageVerificationProgram(t, genericJavaScriptCommandLineAdapter).Project, Source: assemblyline.SourceBlueprint{Documents: []assemblyline.SourceDocument{document}}}
	_, input := javaScriptAcceptanceFixture(t, &program)
	if !strings.Contains(input.Behavior, "capability_001: Supplied text.") || len(input.Capabilities) != 1 {
		t.Fatalf("lost the direct dependency's required meaning: %+v", input)
	}
	for _, value := range append([]string{input.Behavior, input.Signature}, input.Capabilities...) {
		for _, forbidden := range []string{"feature001", "feature002", ".mjs", "normalizeTaskResult"} {
			if strings.Contains(value, forbidden) {
				t.Fatalf("verification received unrelated implementation authority %q: %s", forbidden, value)
			}
		}
	}
}
