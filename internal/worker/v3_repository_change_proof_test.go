package worker

import (
	"os"
	"os/exec"
	"reflect"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/operation"
	repositoryfacts "github.com/gryph/omnidex/internal/repository"
)

func TestRepositoryGoVerificationCommandRejectsTamperedProofBinding(t *testing.T) {
	t.Parallel()
	proof := &repositoryGoTestProof{
		Mode: repositoryGoProofFocused, Package: ".",
		Expected: []repositoryGoExpectedTest{{
			SymbolID: "symbol-one", Name: "TestOne", TargetSymbolIDs: []string{"target-one"},
		}},
	}
	valid := testCommand{
		Family: "go", Name: "go",
		Args:            []string{"test", "-json", "-count=1", "-run", "^TestOne$", "."},
		Purpose:         verificationTest,
		RepositoryProof: proof,
	}
	if err := validateRepositoryGoTestCommand(valid); err != nil {
		t.Fatal(err)
	}
	for _, command := range []testCommand{
		{Family: "go", Name: "go", Args: append([]string(nil), valid.Args...)},
		{Family: "go", Name: "go", Args: []string{"test", "-json", "-count=1", "-run", "TestOne", "."}, RepositoryProof: proof},
		{Family: "go", Name: "go", Args: append([]string(nil), valid.Args...), RepositoryProof: &repositoryGoTestProof{Mode: repositoryGoProofFocused, Package: "."}},
	} {
		if err := validateRepositoryGoTestCommand(command); err == nil {
			t.Fatalf("tampered proof binding was accepted: %+v", command)
		}
	}
}

func TestRepositoryGoVerificationPlanSeparatesFocusedAndBroadAuthority(t *testing.T) {
	t.Parallel()
	focused := testCommand{
		Family: "go", Name: "go",
		Args:    []string{"test", "-json", "-count=1", "-run", "^TestOne$", "."},
		Purpose: verificationTest,
		RepositoryProof: &repositoryGoTestProof{
			Mode: repositoryGoProofFocused, Package: ".",
			Expected: []repositoryGoExpectedTest{{
				SymbolID: "symbol-one", Name: "TestOne", TargetSymbolIDs: []string{"target-one"},
			}},
		},
	}
	broad := testCommand{
		Family: "go", Name: "go", Args: []string{"test", "-json", "-count=1", "./..."},
		Purpose:         verificationTest,
		RepositoryProof: &repositoryGoTestProof{Mode: repositoryGoProofBroad, Package: "./..."},
	}
	if err := validateRepositoryGoVerificationPlan(
		repositoryVerificationStaged, []testCommand{focused, broad},
	); err != nil {
		t.Fatal(err)
	}
	if err := validateRepositoryGoVerificationPlan(
		repositoryVerificationAuthoritative, []testCommand{focused, broad},
	); err != nil {
		t.Fatal(err)
	}
	if err := validateRepositoryGoVerificationPlan(
		repositoryVerificationBaseline, []testCommand{focused, broad},
	); err != nil {
		t.Fatal(err)
	}
	for _, invalid := range []struct {
		scope    repositoryVerificationScope
		commands []testCommand
	}{
		{scope: repositoryVerificationStaged, commands: []testCommand{broad}},
		{scope: repositoryVerificationStaged, commands: []testCommand{broad, focused}},
		{scope: repositoryVerificationAuthoritative, commands: []testCommand{focused}},
		{scope: repositoryVerificationAuthoritative, commands: []testCommand{broad}},
		{scope: repositoryVerificationBaseline, commands: []testCommand{focused}},
		{scope: repositoryVerificationBaseline, commands: []testCommand{broad}},
	} {
		if err := validateRepositoryGoVerificationPlan(invalid.scope, invalid.commands); err == nil {
			t.Fatalf("invalid %s proof plan was accepted: %+v", invalid.scope, invalid.commands)
		}
	}
}

func TestRepositoryCommandMetadataBindsExactExpectedTestAuthority(t *testing.T) {
	t.Parallel()
	command := testCommand{RepositoryProof: &repositoryGoTestProof{
		Mode: repositoryGoProofFocused, Package: "./sample",
		Expected: []repositoryGoExpectedTest{{
			SymbolID: "symbol-one", Name: "TestOne", TargetSymbolIDs: []string{"target-one"},
		}},
	}}
	metadata := repositoryCommandMetadata(
		map[string]any{
			"workspace": "/forbidden", "succeeded": true,
			"repository_verification_accepted":      true,
			"repository_verification_plan_accepted": true,
		},
		repositoryVerificationAuthority{
			contractID: "contract-one", sourceSnapshotID: "snapshot-one",
			stageID: "stage-one", patchSHA256: "patch-one",
			planID: "plan-one", expectedPostID: "post-one",
		},
		repositoryVerificationStaged, command, true,
	)
	if _, leaked := metadata["workspace"]; leaked {
		t.Fatalf("workspace authority leaked into evidence metadata: %#v", metadata)
	}
	if _, exists := metadata["repository_verification_accepted"]; exists {
		t.Fatalf("per-command metadata claimed plan acceptance: %#v", metadata)
	}
	if _, exists := metadata["repository_verification_plan_accepted"]; exists {
		t.Fatalf("per-command metadata claimed plan acceptance: %#v", metadata)
	}
	if metadata["repository_structured_proof_valid"] != true ||
		metadata["repository_change_contract_id"] != "contract-one" ||
		metadata["repository_source_snapshot_id"] != "snapshot-one" ||
		metadata["repository_change_stage_id"] != "stage-one" ||
		metadata["repository_change_patch_sha256"] != "patch-one" ||
		metadata["repository_verification_plan_id"] != "plan-one" ||
		metadata["repository_expected_post_id"] != "post-one" ||
		metadata["repository_verification_proof_mode"] != "focused" ||
		metadata["repository_verification_package"] != "./sample" ||
		!reflect.DeepEqual(metadata["repository_expected_test_ids"], []string{"symbol-one"}) ||
		!reflect.DeepEqual(metadata["repository_expected_test_names"], []string{"TestOne"}) ||
		!reflect.DeepEqual(metadata["repository_verified_target_ids"], []string{"target-one"}) {
		t.Fatalf("proof metadata=%#v", metadata)
	}
}

func TestRepositoryVerificationStagedAcceptanceIsPlanLevel(t *testing.T) {
	t.Parallel()
	authority := repositoryVerificationAuthority{
		contractID: "contract-one", sourceSnapshotID: "snapshot-one",
		stageID: "stage-one", patchSHA256: "patch-one",
		planID: "plan-one", expectedPostID: "post-one",
	}
	record, err := repositoryVerificationAcceptanceEvidence(
		authority, repositoryVerificationStaged, []testCommand{{}, {}, {}},
	)
	if err != nil {
		t.Fatal(err)
	}
	if record.Metadata["repository_verification_plan_accepted"] != true ||
		record.Metadata["repository_verification_command_count"] != 3 ||
		record.Metadata["repository_verification_scope"] != "staged" ||
		record.Metadata["repository_change_stage_id"] != "stage-one" ||
		record.Metadata["repository_expected_post_id"] != "post-one" {
		t.Fatalf("plan acceptance evidence=%#v", record.Metadata)
	}
	if _, err := repositoryVerificationAcceptanceEvidence(
		authority, repositoryVerificationAuthoritative, []testCommand{{}},
	); err == nil {
		t.Fatal("authoritative verification fabricated a non-journal acceptance record")
	}
}

func TestRepositoryVerificationExitOneIsTheOnlyCorrectableResult(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		output  map[string]any
		accepts bool
	}{
		{output: map[string]any{"succeeded": false, "exit_code": 1}, accepts: true},
		{output: map[string]any{"succeeded": false, "exit_code": -1}},
		{output: map[string]any{"succeeded": false, "exit_code": 7}},
		{output: map[string]any{"succeeded": false, "exit_code": float64(1)}},
		{output: map[string]any{"succeeded": true, "exit_code": 1}},
	} {
		err := requireRepositoryGoOrdinaryFailure(operation.Result{Output: test.output})
		if (err == nil) != test.accepts {
			t.Fatalf("output=%#v accepts=%t error=%v", test.output, test.accepts, err)
		}
	}
}

func TestRepositoryGoProofAcceptsRealFocusedAndBroadGoJSON(t *testing.T) {
	t.Parallel()
	snapshot, analysis := existingRepositoryVerificationFixture(t)
	first := existingRepositoryVerificationSymbol(t, analysis, "First")
	contract, err := repositoryfacts.BuildChangeContract(
		snapshot, analysis, []repositoryfacts.ChangeRequest{{
			SymbolID: first.ID, RequirementQuote: "change First",
		}},
	)
	if err != nil {
		t.Fatal(err)
	}
	commands, err := existingRepositoryGoVerificationCommands(snapshot, analysis, contract)
	if err != nil {
		t.Fatal(err)
	}
	focused := commands[0]
	command := exec.Command("go", focused.Args...)
	command.Dir = snapshot.Root
	command.Env = append(os.Environ(), "GOPROXY=off", "GOSUMDB=off", "GOTOOLCHAIN=local")
	output, err := command.Output()
	if err != nil {
		t.Fatal(err)
	}
	if err := validateRepositoryGoTestProof(*focused.RepositoryProof, string(output)); err != nil {
		t.Fatalf("real structured Go proof: %v\n%s", err, output)
	}
	broad := commands[len(commands)-1]
	command = exec.Command("go", broad.Args...)
	command.Dir = snapshot.Root
	command.Env = append(os.Environ(), "GOPROXY=off", "GOSUMDB=off", "GOTOOLCHAIN=local")
	output, err = command.Output()
	if err != nil {
		t.Fatal(err)
	}
	if err := validateRepositoryGoTestProof(*broad.RepositoryProof, string(output)); err != nil {
		t.Fatalf("real structured broad Go proof: %v\n%s", err, output)
	}
}

func TestRepositoryGoTestSelectorIsAnchoredAndDeterministic(t *testing.T) {
	t.Parallel()
	left := []repositoryGoExpectedTest{
		{SymbolID: "symbol-two", Name: "TestTwo", TargetSymbolIDs: []string{"target-two"}},
		{SymbolID: "symbol-one", Name: "TestOne", TargetSymbolIDs: []string{"target-one"}},
	}
	right := []repositoryGoExpectedTest{left[1], left[0]}
	first, err := repositoryGoTestSelector(left)
	if err != nil {
		t.Fatal(err)
	}
	second, err := repositoryGoTestSelector(right)
	if err != nil {
		t.Fatal(err)
	}
	if first != "^(TestOne|TestTwo)$" || first != second || !strings.HasPrefix(first, "^") || !strings.HasSuffix(first, "$") {
		t.Fatalf("selectors first=%q second=%q", first, second)
	}
}

func TestRunnableGoTestNameRejectsNonTestAndSpecialEntrypoints(t *testing.T) {
	t.Parallel()
	for _, name := range []string{"Test", "TestValue", "Test_Value"} {
		if !runnableGoTestName(name) {
			t.Fatalf("runnable test %q was rejected", name)
		}
	}
	for _, name := range []string{"", "TestMain", "Testable", "BenchmarkValue", "FuzzValue"} {
		if runnableGoTestName(name) {
			t.Fatalf("non-runnable exact test %q was accepted", name)
		}
	}
}

func TestRepositoryChangeVerificationHasNoPackageOKHeuristic(t *testing.T) {
	t.Parallel()
	for _, name := range []string{
		"v3_repository_change_commands.go", "v3_repository_change_verification.go",
	} {
		raw, err := os.ReadFile(name)
		if err != nil {
			t.Fatal(err)
		}
		for _, forbidden := range []string{
			"goVerificationOutputHasTests", "hasTests := false", "strings.Fields(line)",
		} {
			if strings.Contains(string(raw), forbidden) {
				t.Fatalf("repository verification %s retains forbidden package-ok heuristic %q", name, forbidden)
			}
		}
	}
}
