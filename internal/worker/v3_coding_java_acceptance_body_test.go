package worker

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/assemblyline"
)

const javaAcceptanceFixtureBinding = `Map<String, Object> result = run(Map.of("arguments", List.of(), "standardInput", ""), Map.of());`

func TestJavaAcceptanceRequiresAnExecutedIndependentObservation(t *testing.T) {
	program := testCompiledLanguageVerificationProgram(t, genericJavaCommandLineAdapter)
	ref, input := javaAcceptanceFixture(t, &program)
	for _, body := range []string{
		javaAcceptanceFixtureBinding + `assert result.get("output").equals("ready");`,
		javaAcceptanceFixtureBinding + `assert "ready".equals(result.get("output")) : "observed output";`,
		javaAcceptanceFixtureBinding + `assert ((Integer) result.get("exitCode")) == 0; assert Map.of("count", 2).equals(result.get("state"));`,
		javaAcceptanceFixtureBinding + `assert result.get("output").equals("ready") && ((Integer) result.get("exitCode")) == 0;`,
		strings.Replace(javaAcceptanceFixtureBinding, "Map<String, Object> result", "final var result", 1) + `assert result.get("output").equals("ready");`,
		strings.Replace(javaAcceptanceFixtureBinding, "run(", "run(/* input */", 1) + `assert (result.get("output").equals("ready"));`,
		`Map<String, Object> result = run(Map.of("arguments", List.of("8", "-3"), "standardInput", ""), Map.of()); assert result.get("output").equals("5");`,
		`Map<String, Object> result = run(Map.of("arguments", List.of(), "standardInput", ""), Map.of("capability_001", Map.of("output", "value", "error", "", "exitCode", 0, "state", Map.of()))); assert result.get("output").equals("VALUE");`,
	} {
		declaration, err := validateDirectCodingJavaFragment(input, body)
		if err != nil {
			t.Fatalf("ordinary Java body failed: %v", err)
		}
		if err := validateDirectCodingJavaAcceptance(ref, declaration); err != nil {
			t.Fatalf("rejected direct observation %s: %v", body, err)
		}
	}
	for name, body := range map[string]string{
		"empty":                "",
		"no run":               `Map<String, Object> result = Map.of(); assert result.get("output").equals("ready");`,
		"no assertion":         javaAcceptanceFixtureBinding,
		"self comparison":      javaAcceptanceFixtureBinding + `assert result.get("output").equals(result.get("output"));`,
		"detached assertion":   javaAcceptanceFixtureBinding + `assert "ready".equals("ready");`,
		"early return":         javaAcceptanceFixtureBinding + `return; assert result.get("output").equals("ready");`,
		"unreachable":          javaAcceptanceFixtureBinding + `if (false) { assert result.get("output").equals("ready"); }`,
		"swallowed":            javaAcceptanceFixtureBinding + `try { assert result.get("output").equals("ready"); } catch (AssertionError ignored) { }`,
		"rebound":              javaAcceptanceFixtureBinding + `result = Map.of("output", "ready"); assert result.get("output").equals("ready");`,
		"mutated observation":  javaAcceptanceFixtureBinding + `result.put("output", "ready"); assert result.get("output").equals("ready");`,
		"hidden mutation":      javaAcceptanceFixtureBinding + `assert result.put("output", "ready").equals("ready");`,
		"dependent expected":   javaAcceptanceFixtureBinding + `assert result.get("output").equals(String.valueOf(result.get("output")));`,
		"repeated observation": javaAcceptanceFixtureBinding + `assert result.equals(run(Map.of(), Map.of()));`,
		"unexecuted closure":   javaAcceptanceFixtureBinding + `Runnable check = () -> { assert result.get("output").equals("ready"); };`,
		"hidden call":          `Map<String, Object> result = run(Map.of("arguments", List.of(lookup()), "standardInput", ""), Map.of()); assert result.get("output").equals("ready");`,
		"missing input field":  `Map<String, Object> result = run(Map.of(), Map.of()); assert result.get("output").equals("ready");`,
		"null dependency map":  `Map<String, Object> result = run(Map.of("arguments", List.of(), "standardInput", ""), null); assert result.get("output").equals("ready");`,
		"bypass":               javaAcceptanceFixtureBinding + `assert result.get("output").equals("ready") || true;`,
		"reference equality":   javaAcceptanceFixtureBinding + `assert result.get("output") == "ready";`,
	} {
		t.Run(name, func(t *testing.T) {
			if err := validateDirectCodingJavaAcceptance(ref, ref.Block.Signature+" {\n"+body+"\n}"); err == nil {
				t.Fatalf("accepted non-proving verification: %s", body)
			}
		})
	}
	wrongName := strings.Replace(program.Generated[ref.Block.ID], "verifyFeature001", "verifyOther", 1)
	if err := validateDirectCodingJavaAcceptance(ref, wrongName); err == nil {
		t.Fatal("accepted a different verification declaration")
	}
}

func TestJavaVerificationUsesOneOrdinaryBodyCall(t *testing.T) {
	program := testCompiledLanguageVerificationProgram(t, genericJavaCommandLineAdapter)
	ref, input := javaAcceptanceFixture(t, &program)
	body := javaAcceptanceFixtureBinding + "\n" + `assert result.get("output").equals("ready");`
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
	generator, err := newDirectCodingLanguageSourceGeneratorForProgram(&directCodingSession{}, program)
	if err != nil {
		t.Fatal(err)
	}
	declaration, err := generator.generateBlockWithRuntime(runtime, "fixture-model", ref, input)
	if err != nil || calls != 1 || finalizations != 1 || declaration != ref.Block.Signature+" {\n"+body+"\n}" {
		t.Fatalf("ordinary verification body: err=%v calls=%d finalizations=%d source=%s", err, calls, finalizations, declaration)
	}
}

func TestJavaWriteGateRejectsNonProvingAcceptance(t *testing.T) {
	program := testCompiledLanguageVerificationProgram(t, genericJavaCommandLineAdapter)
	ref, _ := javaAcceptanceFixture(t, &program)
	program.Generated[ref.Block.ID] = ref.Block.Signature + " { }"
	assembly, err := directCodingAssemblyFromProgram(program)
	if err != nil {
		t.Fatal(err)
	}
	session := directCodingSession{program: &program}
	if _, _, _, err := session.hostVerificationAuthority(assembly); err == nil {
		t.Fatal("write gate accepted an assertion-free Java verification body")
	}
}

func TestJavaMissingAcceptanceValidatorFailsBeforeInference(t *testing.T) {
	program := testCompiledLanguageVerificationProgram(t, genericJavaCommandLineAdapter)
	ref, input := javaAcceptanceFixture(t, &program)
	generator, err := newDirectCodingLanguageSourceGeneratorForProgram(&directCodingSession{}, program)
	if err != nil {
		t.Fatal(err)
	}
	generator.config.ValidateAcceptance = nil
	calls := 0
	runtime := typedWorkerRuntime{Execute: func(assemblyline.PortableJob, string) (assemblyline.PortableResult, error) {
		calls++
		return assemblyline.PortableResult{}, fmt.Errorf("unavailable verifier reached inference")
	}}
	if _, err := generator.generateBlockWithRuntime(runtime, "fixture-model", ref, input); err == nil || calls != 0 {
		t.Fatalf("missing acceptance validator: err=%v inference calls=%d", err, calls)
	}
}
