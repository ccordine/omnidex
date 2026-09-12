package worker

import (
	"reflect"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/assemblyline"
)

func TestRegisteredSourceExecutorsRequireStagedVerification(t *testing.T) {
	contract := reflect.TypeFor[directCodingProjectSourceGenerator]()
	for _, method := range []string{"VerifyTask", "VerifyFinal", "Close"} {
		if _, exists := contract.MethodByName(method); !exists {
			t.Errorf("registered source executor can omit %s", method)
		}
	}
	for _, stack := range registeredDirectCodingProjectStacks() {
		if stack.NewSourceGenerator == nil || stack.VerifyHost == nil {
			t.Errorf("registered stack %s omits source execution or host verification", stack.ID)
		}
	}
}

func TestCompiledLanguageStacksCannotSkipAuthoritativeVerification(t *testing.T) {
	for _, stackID := range []string{genericJavaScriptCommandLineAdapter, genericRustCommandLineAdapter, genericJavaCommandLineAdapter} {
		t.Run(stackID, func(t *testing.T) {
			program := testCompiledLanguageVerificationProgram(t, stackID)
			assembly, err := directCodingAssemblyFromProgram(program)
			if err != nil {
				t.Fatal(err)
			}
			session := &directCodingSession{program: &program}
			selected, observed, verify, err := session.hostVerificationAuthority(assembly)
			if err != nil || !verify || selected != &program || !directCodingAssembliesEqual(assembly, observed) {
				t.Fatalf("registered stack skipped host verification: verify=%t err=%v", verify, err)
			}
			changed := assembly
			changed.Files = append([]directCodingFileTask(nil), assembly.Files...)
			changed.Files[0].Content = append(append([]byte(nil), changed.Files[0].Content...), '\n')
			if _, _, _, err := session.hostVerificationAuthority(changed); err == nil {
				t.Fatal("changed source passed the exact host assembly boundary")
			}
			program.Project.Stack.VerifyHost = nil
			if _, _, _, err := session.hostVerificationAuthority(assembly); err == nil || !strings.Contains(err.Error(), "no authoritative host verifier") {
				t.Fatalf("missing host verifier did not fail explicitly: %v", err)
			}
		})
	}
}

// This explicitly constructed fixture proves execution mechanics, not intent interpretation.
func testCompiledLanguageVerificationProgram(t *testing.T, stackID string) directCodingProgram {
	t.Helper()
	base := testExactGoSieveProgram(t)
	specification := assemblyline.ApplicationSpecification{
		Surface: base.Workload.Surface, ProductQuote: base.Workload.ProductQuote,
		Requirements: []assemblyline.Requirement{{ID: base.Workload.Tasks[0].RequirementID, SourceQuote: base.Workload.Tasks[0].RequirementQuote}},
	}
	stack, err := directCodingProjectStackByID(stackID)
	if err != nil {
		t.Fatal(err)
	}
	var profile directCodingProjectVersionProfile
	for _, candidate := range registeredDirectCodingProjectVersionProfiles() {
		if candidate.StackID == stackID {
			profile = candidate
			break
		}
	}
	dialect, err := directCodingProjectSourceDialect(profile)
	if err != nil {
		t.Fatal(err)
	}
	target, coverage, err := resolveDirectCodingTargetTree(specification, base.Workload, stack, nil, directCodingTargetTreeOccupation{})
	if err != nil {
		t.Fatal(err)
	}
	program, err := compileDirectCodingProgram(specification, base.Workload,
		directCodingCapabilityGraph{base.Workload.Tasks[0].RequirementID: nil},
		directCodingProjectSelection{Stack: stack, Profile: profile, Dialect: dialect}, target, coverage, nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	program.RequirementRelations = base.RequirementRelations
	for _, document := range program.Source.Documents {
		for _, block := range document.Blocks {
			if !block.Generated() {
				continue
			}
			body := ""
			switch stackID {
			case genericJavaScriptCommandLineAdapter:
				body = `return normalizeTaskResult({ output: "ready", error: "", exitCode: 0, state: {} });`
				if block.Role == assemblyline.SourceBlockTaskVerification {
					body = `const result = run({ arguments: [], standardInput: "" }, {});
assert.strictEqual(result.output, "ready");`
				}
			case genericRustCommandLineAdapter:
				body = `TaskResult { output: String::from("ready"), ..TaskResult::default() }`
				if block.Role == assemblyline.SourceBlockTaskVerification {
					body = `let result = run(&TaskInput { arguments: vec![], standard_input: String::new() }, &CapabilityResults::new());
assert_eq!(result.output, "ready");`
				}
			case genericJavaCommandLineAdapter:
				body = `return Runtime.result("ready", "", 0, Map.of());`
				if block.Role == assemblyline.SourceBlockTaskVerification {
					body = `Map<String, Object> result = run(Map.of("arguments", List.of(), "standardInput", ""), Map.of());
assert result.get("output").equals("ready");`
				}
			}
			program.Generated[block.ID] = strings.TrimSpace(block.Signature) + " {\n" + body + "\n}"
		}
	}
	return program
}
