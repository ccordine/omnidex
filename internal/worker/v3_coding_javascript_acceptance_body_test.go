package worker

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/assemblyline"
)

func TestJavaScriptAcceptanceRequiresAnExecutedIndependentObservation(t *testing.T) {
	program := testCompiledLanguageVerificationProgram(t, genericJavaScriptCommandLineAdapter)
	ref, input := javaScriptAcceptanceFixture(t, &program)
	const binding = `const result = run({ arguments: [], standardInput: "" }, {});`
	for _, assertion := range []string{
		`assert.strictEqual(result.output, "ready");`,
		`assert.deepStrictEqual(result.state, { count: -2, values: [true, null, 1.5] });`,
		`assert.strictEqual(result.exitCode, 0, "process status"); assert.strictEqual(result.error, "");`,
	} {
		for _, keyword := range []string{"const", "let", "var"} {
			body := strings.Replace(binding, "const", keyword, 1) + "\n" + assertion
			declaration, err := validateDirectCodingJavaScriptFragment(input, body)
			if err != nil {
				t.Fatal(err)
			}
			if err := validateDirectCodingJavaScriptAcceptance(ref, declaration); err != nil {
				t.Fatalf("rejected unchanged %s observation %s: %v", keyword, assertion, err)
			}
		}
	}
	for name, body := range map[string]string{
		"empty":                 "",
		"no call":               `const result = { output: "ready" }; assert.strictEqual(result.output, "ready");`,
		"no assertion":          binding,
		"self comparison":       binding + `assert.strictEqual(result.output, result.output);`,
		"detached assertion":    binding + `assert.strictEqual("ready", "ready");`,
		"early return":          binding + `return; assert.strictEqual(result.output, "ready");`,
		"unreachable assertion": binding + `if (false) { assert.strictEqual(result.output, "ready"); }`,
		"swallowed failure":     binding + `try { assert.strictEqual(result.output, "ready"); } catch {}`,
		"rebound result":        strings.Replace(binding, "const", "let", 1) + `result = { output: "ready" }; assert.strictEqual(result.output, "ready");`,
		"changed observation":   binding + `result.output = "ready"; assert.strictEqual(result.output, "ready");`,
		"aliased assertion":     binding + `const check = assert.strictEqual; check(result.output, "ready");`,
		"repeated call":         binding + `assert.strictEqual(result.output, run({arguments: [], standardInput: ""}, {}).output);`,
		"invalid input":         `const result = run({ arguments: [42], standardInput: "" }, {}); assert.strictEqual(result.output, "ready");`,
		"optional call":         `const result = run?.({ arguments: [], standardInput: "" }, {}); assert.strictEqual(result.output, "ready");`,
	} {
		t.Run(name, func(t *testing.T) {
			declaration := ref.Block.Signature + " {\n" + body + "\n}"
			if err := validateDirectCodingJavaScriptAcceptance(ref, declaration); err == nil {
				t.Fatalf("accepted non-proving verification: %s", body)
			}
		})
	}
	wrongName := strings.Replace(program.Generated[ref.Block.ID], "verifyFeature001", "verifyOther", 1)
	if err := validateDirectCodingJavaScriptAcceptance(ref, wrongName); err == nil {
		t.Fatal("accepted a different verification declaration")
	}
}

func TestJavaScriptVerificationUsesOneOrdinaryBodyCall(t *testing.T) {
	program := testCompiledLanguageVerificationProgram(t, genericJavaScriptCommandLineAdapter)
	ref, input := javaScriptAcceptanceFixture(t, &program)
	const body = `const result = run({ arguments: [], standardInput: "" }, {});
assert.strictEqual(result.output, "ready");`
	calls, finalizations := 0, 0
	runtime := typedWorkerRuntime{
		Context: context.Background(), MaxAttempts: assemblyline.MaxSourceBodyAttempts,
		Execute: func(job assemblyline.PortableJob, model string) (assemblyline.PortableResult, error) {
			calls++
			if model != "fixture-model" {
				t.Fatalf("changed model: %s", model)
			}
			return exactSourceBodyTestResult(t, job, body), nil
		},
		Correct: func(assemblyline.PortableJob, string, assemblyline.SourceBodyCorrection) (assemblyline.PortableResult, error) {
			return assemblyline.PortableResult{}, fmt.Errorf("valid verification must not request correction")
		},
		Release: func(assemblyline.PortableJob) error { return nil },
		Finalize: func(_ assemblyline.PortableJob, _ assemblyline.PortableResult, err error) error {
			finalizations++
			return err
		},
	}
	generator := directCodingLanguageSourceGenerator{config: directCodingLanguageSourceConfig{
		Language: "javascript", AdapterID: "javascript", ValidateFragment: validateDirectCodingJavaScriptFragment,
		ValidateAcceptance: validateDirectCodingJavaScriptAcceptance,
	}}
	declaration, err := generator.generateBlockWithRuntime(runtime, "fixture-model", ref, input)
	if err != nil || calls != 1 || finalizations != 1 || declaration != ref.Block.Signature+" {\n"+body+"\n}" {
		t.Fatalf("ordinary verification body: err=%v calls=%d finalizations=%d source=%s", err, calls, finalizations, declaration)
	}
}

func TestJavaScriptWriteGateRejectsNonProvingAcceptance(t *testing.T) {
	program := testCompiledLanguageVerificationProgram(t, genericJavaScriptCommandLineAdapter)
	ref, _ := javaScriptAcceptanceFixture(t, &program)
	program.Generated[ref.Block.ID] = ref.Block.Signature + " { return; }"
	assembly, err := directCodingAssemblyFromProgram(program)
	if err != nil {
		t.Fatal(err)
	}
	session := directCodingSession{program: &program}
	if _, _, _, err := session.hostVerificationAuthority(assembly); err == nil {
		t.Fatal("write gate accepted an assertion-free verification body")
	}
}
