package worker

import (
	"reflect"
	"testing"

	"github.com/gryph/omnidex/internal/assemblyline"
)

func TestRustCommandLineTargetTreeRequiresMatchingSnakeCasePair(t *testing.T) {
	for name, paths := range map[string][]string{
		"nested implementation": {"src/domain/echo.rs", "tests/echo_test.rs"},
		"nested verification":   {"src/echo.rs", "tests/domain/echo_test.rs"},
		"mismatched":            {"src/echo.rs", "tests/print_test.rs"},
		"reserved":              {"src/type.rs", "tests/type_test.rs"},
		"not snake case":        {"src/Echo.rs", "tests/Echo_test.rs"},
		"source test suffix":    {"src/echo_test.rs", "tests/echo_test_test.rs"},
		"missing verification":  {"src/echo.rs", "src/print.rs"},
	} {
		t.Run(name, func(t *testing.T) {
			if err := validateRustCommandLineTargetTree(assemblyline.TargetTree{Paths: paths}); err == nil {
				t.Fatalf("accepted invalid Rust target paths %v", paths)
			}
		})
	}
	if err := validateRustCommandLineTargetTree(assemblyline.TargetTree{
		Paths: []string{"src/argument_echo.rs", "tests/argument_echo_test.rs"},
	}); err != nil {
		t.Fatal(err)
	}
}

func TestRustAcceptanceRequiresBoundImplementationResultAssertion(t *testing.T) {
	_, workload := rustCommandLineStackFixture(t)
	stage := directCodingProgram{
		StackID: genericRustCommandLineAdapter, Workload: workload,
		Source: assemblyline.SourceBlueprint{Documents: []assemblyline.SourceDocument{
			{
				ID: "feature", Path: "src/echo.rs", AdapterID: "rust",
				Blocks: []assemblyline.SourceBlock{{
					ID:        "feature.001",
					Signature: "pub fn feature_001(input: &TaskInput, dependencies: &CapabilityResults) -> TaskResult",
					Contract:  "Return a result.",
					API:       "pub fn feature_001(input: &TaskInput, dependencies: &CapabilityResults) -> TaskResult",
					TaskID:    "task_001", Role: assemblyline.SourceBlockTaskImplementation,
				}},
			},
			{
				ID: "acceptance", Path: "tests/echo_test.rs", AdapterID: "rust",
				Blocks: []assemblyline.SourceBlock{
					{
						ID:     "acceptance.fixture.001",
						Static: "fn representative_capability_results_for_feature_001() -> CapabilityResults { CapabilityResults::new() }",
						API:    "fn representative_capability_results_for_feature_001() -> CapabilityResults",
						TaskID: "task_001", Role: assemblyline.SourceBlockTaskSupport,
					},
					{
						ID: "acceptance.001", Signature: "fn verify_feature_001()", Contract: "Verify.",
						API: "fn verify_feature_001()", DependsOn: []string{"feature.001", "acceptance.fixture.001"},
						TaskID: "task_001", Role: assemblyline.SourceBlockTaskVerification,
					},
				},
			},
		}},
	}
	ref := assemblyline.SourceBlockRef{
		Document: stage.Source.Documents[1], Block: stage.Source.Documents[1].Blocks[1],
	}
	valid := `fn verify_feature_001() {
    let input = TaskInput { arguments: vec![], standard_input: String::new() };
	let result = feature_001(&input, &representative_capability_results_for_feature_001());
    assert_eq!(result.output, "ready");
}`
	if err := validateDirectCodingRustAcceptance(&stage, ref, valid); err != nil {
		t.Fatal(err)
	}
	for name, assertion := range map[string]string{
		"direct comparison":         `assert!(result.output == "ready");`,
		"direct empty test":         `assert!(result.error.is_empty());`,
		"direct expected test":      `assert!(result.output.contains("read"));`,
		"direct length observation": `assert_eq!(result.output.len(), 5);`,
		"direct map observation":    `assert_eq!(result.state.get("missing"), None);`,
	} {
		t.Run("valid "+name, func(t *testing.T) {
			source := `fn verify_feature_001() {
    let result = feature_001(&TaskInput::default(), &representative_capability_results_for_feature_001());
    ` + assertion + `
}`
			if err := validateDirectCodingRustAcceptance(&stage, ref, source); err != nil {
				t.Fatalf("rejected exact Rust predicate: %v\n%s", err, source)
			}
		})
	}
	for name, source := range map[string]string{
		"no call": `fn verify_feature_001() {
    let result = TaskResult::default();
    assert_eq!(result.output, "ready");
}`,
		"unbound call": `fn verify_feature_001() {
	feature_001(&TaskInput::default(), &representative_capability_results_for_feature_001());
    assert!(true);
}`,
		"unrelated assertion": `fn verify_feature_001() {
	let result = feature_001(&TaskInput::default(), &representative_capability_results_for_feature_001());
    assert_eq!("ready", "ready");
    let _ = result;
}`,
		"external authority": `fn verify_feature_001() {
	let result = feature_001(&TaskInput::default(), &representative_capability_results_for_feature_001());
    let _cwd = std::env::current_dir();
    assert_eq!(result.output, "ready");
}`,
		"shadow implementation": `fn verify_feature_001() {
    let feature_001 = || TaskResult::default();
    let result = feature_001();
			assert_eq!(result.output, "ready");
		}`,
		"empty dependency authority": `fn verify_feature_001() {
			let result = feature_001(&TaskInput::default(), &CapabilityResults::new());
			assert_eq!(result.output, "ready");
		}`,
		"boolean shortcut": `fn verify_feature_001() {
			let result = feature_001(&TaskInput::default(), &representative_capability_results_for_feature_001());
			assert!(result.output.is_empty() || true);
		}`,
		"bitwise truth forcing": `fn verify_feature_001() {
			let result = feature_001(&TaskInput::default(), &representative_capability_results_for_feature_001());
			assert!((result.output == "impossible") | true);
		}`,
		"integer bitwise wrapper": `fn verify_feature_001() {
			let result = feature_001(&TaskInput::default(), &representative_capability_results_for_feature_001());
			assert_eq!(result.exit_code | 1, 1);
		}`,
		"cast wrapper": `fn verify_feature_001() {
			let result = feature_001(&TaskInput::default(), &representative_capability_results_for_feature_001());
			assert!((result.exit_code as i64) == 0);
		}`,
		"self comparison": `fn verify_feature_001() {
			let result = feature_001(&TaskInput::default(), &representative_capability_results_for_feature_001());
			assert_eq!(result.output, result.output);
		}`,
		"detached value": `fn verify_feature_001() {
			let result = feature_001(&TaskInput::default(), &representative_capability_results_for_feature_001());
			let observed = result.output;
			assert_eq!(observed, "ready");
		}`,
		"unreachable nested assertion": `fn verify_feature_001() {
			let result = feature_001(&TaskInput::default(), &representative_capability_results_for_feature_001());
			if false { assert_eq!(result.output, "ready"); }
		}`,
	} {
		t.Run(name, func(t *testing.T) {
			if err := validateDirectCodingRustAcceptance(&stage, ref, source); err == nil {
				t.Fatalf("accepted invalid Rust verification source:\n%s", source)
			}
		})
	}

}

func TestRustTaskVerificationUsesExactCoverageTestTarget(t *testing.T) {
	_, workload := rustCommandLineStackFixture(t)
	target := assemblyline.TargetTree{
		Paths: []string{"src/argument_echo.rs", "tests/argument_echo_test.rs"},
	}
	stack, err := directCodingProjectStackByID(genericRustCommandLineAdapter)
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
	contexts, err := directCodingApplicationTaskContexts(workload)
	if err != nil {
		t.Fatal(err)
	}
	context := contexts["requirement_001"]
	commands, err := rustCommandLineTaskVerificationCommands(context, directCodingProgram{
		StackID: genericRustCommandLineAdapter, Workload: workload, Coverage: coverage,
	})
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"test", "--locked", "--offline", "--test", "argument_echo_test"}
	if len(commands) != 1 || commands[0].Name != "cargo" || !reflect.DeepEqual(commands[0].Args, want) {
		t.Fatalf("Rust task command = %+v, want cargo %v", commands, want)
	}
}

func TestRustCoverageRejectsSharedWorkloadModules(t *testing.T) {
	specification := assemblyline.ApplicationSpecification{
		Surface:      assemblyline.ApplicationSurfaceCommandLine,
		ProductQuote: "two independent command behaviors",
		Requirements: []assemblyline.Requirement{
			{ID: "requirement_001", SourceQuote: "Return the first argument"},
			{ID: "requirement_002", SourceQuote: "Return the standard input"},
		},
	}
	workload, err := assemblyline.FreezeApplicationWorkload(specification)
	if err != nil {
		t.Fatal(err)
	}
	context, err := directCodingApplicationTaskContexts(workload)
	if err != nil {
		t.Fatal(err)
	}
	coverage := assemblyline.ApplicationFileCoveragePlan{
		WorkloadSHA256: workload.SHA256,
		Files: []assemblyline.ApplicationFileCoverage{
			{Path: "src/echo.rs", Kind: assemblyline.TargetArtifactImplementation, TaskIDs: []string{"task_001", "task_002"}},
			{Path: "tests/echo_test.rs", Kind: assemblyline.TargetArtifactVerification, TaskIDs: []string{"task_001", "task_002"}},
		},
	}
	_, err = genericRustCommandLineDocuments(
		"omnidex_fixture", specification, nil, context,
		directCodingCapabilityGraph{"requirement_001": nil, "requirement_002": nil}, coverage,
	)
	if err == nil {
		t.Fatal("accepted shared Rust workload module authority")
	}
}
