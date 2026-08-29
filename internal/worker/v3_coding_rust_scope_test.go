package worker

import (
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/assemblyline"
)

func TestRustFeatureFragmentAcceptsOnlyLocalAndPermittedPureAuthority(t *testing.T) {
	input := rustFeatureFragmentTestInput()
	valid := `pub fn feature_001(input: &TaskInput, dependencies: &CapabilityResults) -> TaskResult {
    let normalize = |value: &str| value.to_string();
    let selected = dependencies.get(FEATURE_001_CAPABILITY_002);
    let mut result = TaskResult::default();
    result.output = if let Some(value) = selected {
        format!("{}:{}", normalize(&input.standard_input), value.output)
    } else {
        normalize(&input.standard_input)
    };
    result
}`
	if _, err := validateDirectCodingRustFragment(input, valid); err != nil {
		t.Fatal(err)
	}
	for name, body := range map[string]string{
		"filesystem":        `let _ = std::fs::read_to_string("secret"); TaskResult::default()`,
		"process":           `let _ = Command::new("sh"); TaskResult::default()`,
		"network":           `let _ = TcpStream::connect("127.0.0.1:1"); TaskResult::default()`,
		"environment":       `let _ = env::var("TOKEN"); TaskResult::default()`,
		"include":           `let _ = include_str!("secret"); TaskResult::default()`,
		"undeclared call":   `let _ = hidden_transform(&input.standard_input); TaskResult::default()`,
		"undeclared path":   `let _ = HiddenAuthority::new(); TaskResult::default()`,
		"crate path":        `let _ = crate::runtime::TaskResult::default(); TaskResult::default()`,
		"unsafe":            `unsafe { std::ptr::read(&0) }; TaskResult::default()`,
		"nested macro":      `macro_rules! hidden { () => { "value" } } let _ = hidden!(); TaskResult::default()`,
		"shadow capability": `let FEATURE_001_CAPABILITY_002 = "other"; TaskResult::default()`,
	} {
		t.Run(name, func(t *testing.T) {
			candidate := strings.Replace(
				input.Signature+" {\n    BODY\n}", "BODY", body, 1,
			)
			if _, err := validateDirectCodingRustFragment(input, candidate); err == nil {
				t.Fatalf("accepted Rust feature authority bypass:\n%s", candidate)
			}
		})
	}
}

func TestRustFeatureFragmentRejectsLocalImportsAndNestedDeclarations(t *testing.T) {
	input := rustFeatureFragmentTestInput()
	for name, body := range map[string]string{
		"use":      `use std::fs; TaskResult::default()`,
		"extern":   `extern crate core; TaskResult::default()`,
		"function": `fn hidden() {} TaskResult::default()`,
		"static":   `static VALUE: &str = "hidden"; TaskResult::default()`,
	} {
		t.Run(name, func(t *testing.T) {
			candidate := strings.Replace(input.Signature+" { BODY }", "BODY", body, 1)
			if _, err := validateDirectCodingRustFragment(input, candidate); err == nil {
				t.Fatalf("accepted nested Rust declaration authority:\n%s", candidate)
			}
		})
	}
}

func TestRustVerificationFragmentMayCallOnlyItsPermittedFeature(t *testing.T) {
	runtimeAPI := rustCommandLineRuntimeDocument().Blocks[0].API
	input := assemblyline.FragmentGenerationInput{
		Language: "rust", Dialect: "Rust 2024", Signature: "fn verify_feature_001()",
		Behavior: "Verify one derived result.",
		PermittedSymbols: []string{
			runtimeAPI,
			"fn representative_capability_results_for_feature_001() -> CapabilityResults",
			"pub fn feature_001(input: &TaskInput, dependencies: &CapabilityResults) -> TaskResult",
			"String", "Vec", "assert", "assert_eq", "assert_ne",
		},
	}
	valid := `fn verify_feature_001() {
    let input = TaskInput { arguments: vec!["ready".to_string()], standard_input: String::new() };
	let result = feature_001(&input, &representative_capability_results_for_feature_001());
    assert_eq!(result.output, "ready");
}`
	if _, err := validateDirectCodingRustFragment(input, valid); err != nil {
		t.Fatal(err)
	}
	invalid := strings.Replace(valid, "let result = feature_001(", "let result = unlisted_feature(", 1)
	if _, err := validateDirectCodingRustFragment(input, invalid); err == nil {
		t.Fatal("accepted an undeclared Rust verification callable")
	}
}

func rustFeatureFragmentTestInput() assemblyline.FragmentGenerationInput {
	return assemblyline.FragmentGenerationInput{
		Language:  "rust",
		Dialect:   "Rust 2024",
		Signature: "pub fn feature_001(input: &TaskInput, dependencies: &CapabilityResults) -> TaskResult",
		Behavior:  "Return one result derived from the supplied values.",
		PermittedSymbols: []string{
			rustCommandLineRuntimeDocument().Blocks[0].API,
			`pub const FEATURE_001_CAPABILITY_002: &str = "CAPABILITY_002";`,
			"String", "Vec",
		},
	}
}
