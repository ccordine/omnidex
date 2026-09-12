package worker

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/assemblyline"
)

const rustAcceptanceFixtureBinding = `let result = run(&TaskInput { arguments: vec![], standard_input: String::new() }, &CapabilityResults::new());`

func TestRustAcceptanceRequiresAnExecutedIndependentObservation(t *testing.T) {
	program := testCompiledLanguageVerificationProgram(t, genericRustCommandLineAdapter)
	ref, input := rustAcceptanceFixture(t, &program)
	for _, body := range []string{
		rustAcceptanceFixtureBinding + `assert_eq!(result.output, "ready");`,
		rustAcceptanceFixtureBinding + `assert_eq!{result.output.as_str(), "ready", "observed output"}`,
		rustAcceptanceFixtureBinding + `assert_eq!(result.exit_code, -2); assert_eq!(result.error, r#"a message"#);`,
		rustAcceptanceFixtureBinding + `assert_eq!(result.state.get("count"), Some(&String::from("2")));`,
		strings.Replace(rustAcceptanceFixtureBinding, "let result", "let mut result", 1) + `assert_eq!(result.output, "ready");`,
		`let result = run(&TaskInput { arguments: vec!["8".to_string(), "-3".to_owned()], standard_input: "".into() }, &CapabilityResults::new()); assert_eq!(result.output, "5");`,
		`let result = run(&TaskInput::default(), &CapabilityResults::from([("capability_001".to_string(), TaskResult { output: String::from("value"), ..TaskResult::default() })])); assert_eq!(result.output, "VALUE");`,
	} {
		declaration, err := validateDirectCodingRustFragment(input, body)
		if err != nil {
			t.Fatalf("ordinary Rust body failed: %v", err)
		}
		if err := validateDirectCodingRustAcceptance(ref, declaration); err != nil {
			t.Fatalf("rejected direct observation %s: %v", body, err)
		}
	}
	for name, body := range map[string]string{
		"empty":                "",
		"no run":               `let result = TaskResult::default(); assert_eq!(result.output, "ready");`,
		"no assertion":         rustAcceptanceFixtureBinding,
		"self comparison":      rustAcceptanceFixtureBinding + `assert_eq!(result.output, result.output);`,
		"detached assertion":   rustAcceptanceFixtureBinding + `assert_eq!("ready", "ready");`,
		"early return":         rustAcceptanceFixtureBinding + `return; assert_eq!(result.output, "ready");`,
		"unreachable":          rustAcceptanceFixtureBinding + `if false { assert_eq!(result.output, "ready"); }`,
		"swallowed":            rustAcceptanceFixtureBinding + `let _ = std::panic::catch_unwind(|| assert_eq!(result.output, "ready"));`,
		"rebound":              strings.Replace(rustAcceptanceFixtureBinding, "let result", "let mut result", 1) + `result = TaskResult::default(); assert_eq!(result.output, "");`,
		"mutated observation":  rustAcceptanceFixtureBinding + `result.output.clear(); assert_eq!(result.output, "");`,
		"hidden mutation":      rustAcceptanceFixtureBinding + `assert_eq!(result.output.clear(), ());`,
		"dependent expected":   rustAcceptanceFixtureBinding + `assert_eq!(result.output, result.output.clone());`,
		"repeated observation": rustAcceptanceFixtureBinding + `assert_eq!(result.output, run(&TaskInput::default(), &CapabilityResults::new()).output);`,
		"unexecuted closure":   rustAcceptanceFixtureBinding + `|| assert_eq!(result.output, "ready");`,
		"hidden call":          `let result = run(&TaskInput { arguments: vec![lookup()], standard_input: String::new() }, &CapabilityResults::new()); assert_eq!(result.output, "ready");`,
	} {
		t.Run(name, func(t *testing.T) {
			if err := validateDirectCodingRustAcceptance(ref, ref.Block.Signature+" {\n"+body+"\n}"); err == nil {
				t.Fatalf("accepted non-proving verification: %s", body)
			}
		})
	}
	wrongName := strings.Replace(program.Generated[ref.Block.ID], "verify_feature_001", "verify_other", 1)
	if err := validateDirectCodingRustAcceptance(ref, wrongName); err == nil {
		t.Fatal("accepted a different verification declaration")
	}
}

func TestRustVerificationUsesOneOrdinaryBodyCall(t *testing.T) {
	program := testCompiledLanguageVerificationProgram(t, genericRustCommandLineAdapter)
	ref, input := rustAcceptanceFixture(t, &program)
	body := rustAcceptanceFixtureBinding + "\n" + `assert_eq!(result.output, "ready");`
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

func TestRustWriteGateRejectsNonProvingAcceptance(t *testing.T) {
	program := testCompiledLanguageVerificationProgram(t, genericRustCommandLineAdapter)
	ref, _ := rustAcceptanceFixture(t, &program)
	program.Generated[ref.Block.ID] = ref.Block.Signature + " { }"
	assembly, err := directCodingAssemblyFromProgram(program)
	if err != nil {
		t.Fatal(err)
	}
	session := directCodingSession{program: &program}
	if _, _, _, err := session.hostVerificationAuthority(assembly); err == nil {
		t.Fatal("write gate accepted an assertion-free Rust verification body")
	}
}

func TestRustMissingAcceptanceValidatorFailsBeforeInference(t *testing.T) {
	program := testCompiledLanguageVerificationProgram(t, genericRustCommandLineAdapter)
	ref, input := rustAcceptanceFixture(t, &program)
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
