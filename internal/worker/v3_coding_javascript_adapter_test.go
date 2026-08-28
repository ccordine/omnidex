package worker

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/assemblyline"
)

func TestJavaScriptCommandLineStackCompilesComposesAndExecutesFocusedTests(t *testing.T) {
	specification, workload := javaScriptCommandLineStackFixture(t)
	target := assemblyline.TargetTree{
		StackID: genericJavaScriptCommandLineAdapter, VersionProfileID: javaScriptCommandLineVersionProfileV1,
		Paths: []string{"feature.mjs", "feature.test.mjs"},
	}
	stack, err := directCodingProjectStackByID(genericJavaScriptCommandLineAdapter)
	if err != nil {
		t.Fatal(err)
	}
	coverage, err := buildDirectCodingApplicationFileCoveragePlan(
		stack, workload, target,
		map[string][]string{workload.Tasks[0].ID: append([]string(nil), target.Paths...)},
	)
	if err != nil {
		t.Fatal(err)
	}
	program, err := compileDirectCodingProgram(
		"javascript-fixture", specification, nil, map[string]directCodingSkillBinding{}, workload,
		directCodingCapabilityGraph{"requirement_001": nil}, target, coverage,
	)
	if err != nil {
		t.Fatal(err)
	}
	program.Generated["feature.001"] = `function feature001(input, dependencies) {
	if (input.arguments.length === 0) return { output: '', error: 'one argument is required', exitCode: 2, state: {} };
	return { output: input.arguments[0], error: '', exitCode: 0, state: { value: input.arguments[0] } };
}`
	program.Generated["acceptance.001"] = `function verifyFeature001() {
  const result = feature001({ arguments: ['ready'], standardInput: '' }, {});
  assert.equal(result.output, 'ready');
}`
	assembly, err := directCodingAssemblyFromProgram(program)
	if err != nil {
		t.Fatal(err)
	}
	if err := validateDirectCodingAssemblySources(program, assembly); err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	for _, file := range assembly.Files {
		targetPath := filepath.Join(root, filepath.FromSlash(file.Path))
		if err := os.MkdirAll(filepath.Dir(targetPath), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(targetPath, []byte(file.Content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	output, err := runDirectCodingStageCommand(
		context.Background(), root, directCodingJavaScriptStageTimeout,
		"node", javaScriptNodeTestArgs()...,
	)
	if err != nil {
		t.Fatalf("node test failed: %v\n%s", err, output)
	}
}

func TestJavaScriptRuntimeRejectsMissingCoercedAndAsynchronousResults(t *testing.T) {
	root := t.TempDir()
	runtimeDocument, err := javaScriptCommandLineRuntimeDocument(
		requireDirectCodingVersionProfile(t, javaScriptCommandLineVersionProfileV1),
	)
	if err != nil {
		t.Fatal(err)
	}
	runtime := runtimeDocument.Blocks[0].Static
	if err := os.WriteFile(filepath.Join(root, "runtime.mjs"), []byte(runtime+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	testSource := `import test from 'node:test';
import assert from 'node:assert/strict';
import { normalizeTaskResult } from './runtime.mjs';

test('strict result boundary', () => {
  const valid = { output: 'ready', error: '', exitCode: 0, state: { value: 'ready' } };
  assert.deepEqual(normalizeTaskResult(valid), valid);
  for (const invalid of [
    {},
    { output: 'ready', error: '', exitCode: 0 },
    { output: null, error: '', exitCode: 0, state: {} },
    { output: 'ready', error: '', exitCode: '0', state: {} },
    { output: 'ready', error: 'also failed', exitCode: 1, state: {} },
    Promise.resolve(valid),
  ]) assert.throws(() => normalizeTaskResult(invalid), TypeError);
});
`
	if err := os.WriteFile(filepath.Join(root, "runtime.test.mjs"), []byte(testSource), 0o600); err != nil {
		t.Fatal(err)
	}
	output, err := runDirectCodingStageCommand(
		context.Background(), root, directCodingJavaScriptStageTimeout,
		"node", javaScriptNodeTestArgs()...,
	)
	if err != nil {
		t.Fatalf("strict JavaScript runtime test failed: %v\n%s", err, output)
	}
}

func TestJavaScriptAcceptanceRequiresImplementationCallAndAssertion(t *testing.T) {
	_, workload := javaScriptCommandLineStackFixture(t)
	stage := directCodingProgram{
		StackID: genericJavaScriptCommandLineAdapter, Workload: workload,
		Source: assemblyline.SourceBlueprint{Documents: []assemblyline.SourceDocument{
			{ID: "feature", Path: "feature.mjs", AdapterID: "javascript", Blocks: []assemblyline.SourceBlock{{
				ID: "feature.001", Signature: "function feature001(input, dependencies)",
				Contract: "Return a result.", API: "function feature001(input, dependencies)",
				TaskID: "task_001", Role: assemblyline.SourceBlockTaskImplementation,
			}}},
			{ID: "acceptance", Path: "feature.test.mjs", AdapterID: "javascript", Blocks: []assemblyline.SourceBlock{{
				ID: "acceptance.001", Signature: "function verifyFeature001()", Contract: "Verify.",
				API: "function verifyFeature001()", DependsOn: []string{"feature.001"},
				TaskID: "task_001", Role: assemblyline.SourceBlockTaskVerification,
			}}},
		}},
	}
	ref := assemblyline.SourceBlockRef{
		Document: stage.Source.Documents[1], Block: stage.Source.Documents[1].Blocks[0],
	}
	valid := `function verifyFeature001() {
		const result = feature001({ arguments: ['ready'], standardInput: '' }, {});
		assert.equal(result.output, 'ready');
	}`
	if err := validateDirectCodingJavaScriptAcceptance(&stage, ref, valid); err != nil {
		t.Fatal(err)
	}
	for name, source := range map[string]string{
		"no call":      `function verifyFeature001() { assert.fail('missing'); }`,
		"no assertion": `function verifyFeature001() { feature001({ arguments: [] }, {}); }`,
		"boolean shortcut": `function verifyFeature001() {
			const result = feature001({ arguments: [] }, {});
			assert.ok(result.output || true);
		}`,
		"bitwise truth forcing": `function verifyFeature001() {
			const result = feature001({ arguments: [] }, {});
			assert.ok((result.output === 'impossible') | 1);
		}`,
		"comparison wrapper": `function verifyFeature001() {
			const result = feature001({ arguments: [] }, {});
			assert.ok(Boolean(result.output === 'impossible'));
		}`,
		"self comparison": `function verifyFeature001() {
			const result = feature001({ arguments: [] }, {});
			assert.equal(result.output, result.output);
		}`,
		"detached value": `function verifyFeature001() {
			const result = feature001({ arguments: [] }, {});
			const observed = result.output;
			assert.equal(observed, 'ready');
		}`,
		"nested unreachable assertion": `function verifyFeature001() {
			const result = feature001({ arguments: [] }, {});
			if (false) assert.equal(result.output, 'ready');
		}`,
		"unbound call": `function verifyFeature001() {
			feature001({ arguments: [] }, {});
			assert.ok(true);
		}`,
	} {
		t.Run(name, func(t *testing.T) {
			if err := validateDirectCodingJavaScriptAcceptance(&stage, ref, source); err == nil {
				t.Fatalf("accepted invalid JavaScript verification source %s", source)
			}
		})
	}

	stage.Workload.Tasks[0].AcceptanceCriteria = append(
		stage.Workload.Tasks[0].AcceptanceCriteria,
		"The result exposes the expected exit status.",
	)
	duplicate := `function verifyFeature001() {
		const result = feature001({ arguments: ['ready'], standardInput: '' }, {});
		assert.equal(result.output, 'ready');
		assert.equal(result.output, 'ready');
	}`
	if err := validateDirectCodingJavaScriptAcceptance(&stage, ref, duplicate); err == nil {
		t.Fatalf("accepted duplicate JavaScript verification conditions:\n%s", duplicate)
	}
}

func TestProjectStackSelectionCanSelectJavaScriptWithoutRegistryIdentity(t *testing.T) {
	specification, _ := javaScriptCommandLineStackFixture(t)
	runtime := typedWorkerRuntime{
		Context: context.Background(), MaxAttempts: 1,
		Execute: testPortableExecutor(func(_ string, _ string, prompt string) (string, error) {
			if strings.Contains(prompt, genericJavaScriptCommandLineAdapter) || !strings.Contains(prompt, "STACK_CANDIDATE_2") {
				t.Fatalf("stack constraint leaked or omitted JavaScript authority: %s", prompt)
			}
			return "STACK_CANDIDATE_2", nil
		}),
	}
	selection, err := selectDirectCodingProject(
		runtime, func() (string, error) { return "constraint", nil }, specification, nil, nil,
	)
	if err != nil || selection.Stack.ID != genericJavaScriptCommandLineAdapter ||
		selection.VersionProfileID != javaScriptCommandLineVersionProfileV1 {
		t.Fatalf("selection=%+v error=%v", selection, err)
	}
}

func javaScriptCommandLineStackFixture(
	t *testing.T,
) (assemblyline.ApplicationSpecification, assemblyline.FrozenApplicationWorkload) {
	t.Helper()
	specification := assemblyline.ApplicationSpecification{
		Surface: assemblyline.ApplicationSurfaceCommandLine, ProductQuote: "argument echo command",
		Requirements: []assemblyline.Requirement{{
			ID: "requirement_001", SourceQuote: "Print the first supplied argument",
		}},
	}
	input := applicationWorkloadInput(specification)
	workload, err := assemblyline.FreezeApplicationWorkload(input, assemblyline.ApplicationWorkloadDraft{
		Schema: assemblyline.ApplicationWorkloadDraftSchemaV1,
		Tasks: []assemblyline.ApplicationWorkloadTaskDraft{{
			RequirementID: "requirement_001", Objective: "Return the first command argument.",
			RequiredBehaviors:  []string{"Accept one command argument and expose it as output."},
			AcceptanceCriteria: []string{"The first command argument is returned unchanged."},
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := specification.Validate(); err != nil {
		t.Fatal(err)
	}
	return specification, workload
}
