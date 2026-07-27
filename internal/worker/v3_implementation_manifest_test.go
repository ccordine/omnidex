package worker

import (
	"errors"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/artifacts"
	"github.com/gryph/omnidex/internal/model"
	"github.com/gryph/omnidex/internal/specialist"
)

func TestImplementationScopesHavePurposeSizedOutputBudgets(t *testing.T) {
	for scope, want := range map[string]int{
		"v3_implementation_manifest":       2048,
		"v3_work_item_writer_write_main_1": 8192,
		"v3_work_item_review_write_main_1": 1024,
		"v3_failure_triage_verify_all_1":   1024,
	} {
		if got := v3OutputTokenBudget(scope); got != want {
			t.Fatalf("v3OutputTokenBudget(%q)=%d, want %d", scope, got, want)
		}
	}
}

func TestImplementationWriterReservesLargeCoderForCodeContracts(t *testing.T) {
	for discipline, wantFast := range map[string]bool{
		artifacts.ImplementationDisciplineBootstrap:     true,
		artifacts.ImplementationDisciplineDocumentation: true,
		artifacts.ImplementationDisciplineTest:          true,
		artifacts.ImplementationDisciplineEntrypoint:    true,
		artifacts.ImplementationDisciplineDomain:        false,
		artifacts.ImplementationDisciplineInterface:     false,
	} {
		item := artifacts.ImplementationWorkItem{Discipline: discipline}
		if got := implementationWriterUsesFastModel(item); got != wantFast {
			t.Fatalf("discipline=%s fast=%t, want %t", discipline, got, wantFast)
		}
	}
}

func TestImplementationBootstrapRepairEscalatesToReasoningAndExecutorModels(t *testing.T) {
	runtime := &nativeRuntimeV3{claim: &model.ClaimedStep{}, svc: &Service{models: ModelRouting{
		Fast: "qwen2.5-coder:7b", Reasoning: "qwen2.5-coder:14b",
		Plan: "qwen3:4b-thinking", Analyze: "qwen2.5-coder:7b",
		Specialist: map[string]string{specialist.RoleSubtaskExecutorSpecialist: "qwen3-coder:30b"},
	}}}
	item := artifacts.ImplementationWorkItem{Discipline: artifacts.ImplementationDisciplineBootstrap}
	models := runtime.implementationWriterAttemptModels(item, 4)
	want := []string{"qwen2.5-coder:7b", "qwen2.5-coder:14b", "qwen3-coder:30b", "qwen3-coder:30b"}
	if strings.Join(models, "|") != strings.Join(want, "|") {
		t.Fatalf("bootstrap writer models=%#v, want %#v", models, want)
	}
}

func TestImplementationCodeRepairRetainsDedicatedExecutorModel(t *testing.T) {
	runtime := &nativeRuntimeV3{claim: &model.ClaimedStep{}, svc: &Service{models: ModelRouting{
		Fast: "qwen2.5-coder:7b", Reasoning: "qwen2.5-coder:14b",
		Plan: "qwen3:4b-thinking", Analyze: "qwen2.5-coder:7b",
		Specialist: map[string]string{specialist.RoleSubtaskExecutorSpecialist: "qwen3-coder:30b"},
	}}}
	item := artifacts.ImplementationWorkItem{Discipline: artifacts.ImplementationDisciplineInterface}
	models := runtime.implementationWriterAttemptModels(item, 4)
	want := []string{"qwen3-coder:30b", "qwen3-coder:30b", "qwen3-coder:30b", "qwen3-coder:30b"}
	if strings.Join(models, "|") != strings.Join(want, "|") {
		t.Fatalf("code writer models=%#v, want %#v", models, want)
	}
}

func TestImplementationManifestCreatesPendingTypedLedger(t *testing.T) {
	objective := artifacts.Objective{ID: "build_pocket_tasks", Description: "Build Pocket Tasks"}
	criteria := []string{"Commands work", "Focused tests exist", "go test ./... passes"}
	raw := `{
		"role_id":"implementation_planner",
		"items":[
			{
				"id":"write_module",
				"kind":"file",
				"discipline":"bootstrap",
				"path":"go.mod",
				"responsibility":"Declare the standalone Go module",
				"depends_on":[],
				"acceptance_criteria":[]
			},
			{
				"id":"write_commands",
				"kind":"file",
				"discipline":"interface",
				"path":"cli.go",
				"responsibility":"Implement the commands",
				"depends_on":["write_module"],
				"acceptance_criteria":["Commands work"]
			},
			{
				"id":"write_main",
				"kind":"file",
				"discipline":"entrypoint",
				"path":"main.go",
				"responsibility":"Wire the command entrypoint",
				"depends_on":["write_commands"],
				"acceptance_criteria":[]
			},
			{
				"id":"write_tests",
				"kind":"file",
				"discipline":"test",
				"path":"main_test.go",
				"responsibility":"Test commands",
				"depends_on":["write_commands","write_main"],
				"acceptance_criteria":["Focused tests exist"]
			},
			{
				"id":"verify_all",
				"kind":"verification",
				"discipline":"verification",
				"path":"",
				"responsibility":"Run all tests",
				"depends_on":["write_module","write_commands","write_main","write_tests"],
				"acceptance_criteria":["go test ./... passes"],
				"command":{"program":"go","args":["test","./..."],"timeout_seconds":180}
			}
		]
	}`

	ledger, err := parseImplementationManifest(raw, objective, []string{"Use only the Go standard library"}, criteria)
	if err != nil {
		t.Fatal(err)
	}
	if ledger.Revision != 1 || len(ledger.Items) != 5 || len(ledger.Constraints) != 1 {
		t.Fatalf("ledger=%+v", ledger)
	}
	for _, item := range ledger.Items {
		if item.Status != artifacts.ImplementationWorkStatusPending || item.Attempts != 0 || item.LastError != "" {
			t.Fatalf("planner controlled server state for item %+v", item)
		}
	}
}

func TestImplementationManifestCannotDropAuthoritativeCriterion(t *testing.T) {
	objective := artifacts.Objective{ID: "build_pocket_tasks", Description: "Build Pocket Tasks"}
	raw := `{
		"role_id":"implementation_planner",
		"items":[
			{"id":"write_module","kind":"file","discipline":"bootstrap","path":"go.mod","responsibility":"Declare module","depends_on":[],"acceptance_criteria":[]},
			{"id":"write_commands","kind":"file","discipline":"interface","path":"cli.go","responsibility":"Implement CLI","depends_on":["write_module"],"acceptance_criteria":["Commands work"]},
			{"id":"write_main","kind":"file","discipline":"entrypoint","path":"main.go","responsibility":"Wire CLI","depends_on":["write_commands"],"acceptance_criteria":[]},
			{"id":"verify_all","kind":"verification","discipline":"verification","path":"","responsibility":"Run tests","depends_on":["write_module","write_commands","write_main"],"acceptance_criteria":["go test passes"],"command":{"program":"go","args":["test","./..."]}}
		]
	}`
	_, err := parseImplementationManifest(raw, objective, nil, []string{"Commands work", "README includes examples", "go test passes"})
	if err == nil || !strings.Contains(err.Error(), "README includes examples") {
		t.Fatalf("parseImplementationManifest() err=%v, want dropped-criterion rejection", err)
	}
}

func TestImplementationManifestPromptHasNoMemoryOrConversationChannel(t *testing.T) {
	objective := artifacts.Objective{ID: "build_app", Description: "Build the current app", AcceptanceCriteria: []string{"Tests pass"}}
	prompt, err := buildImplementationManifestPrompt(objective, []string{"No external dependencies"}, []string{"Tests pass"}, "File tree:\n- main.go")
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{"memory_references", "conversation_history", "user_feedback", "web_evidence"} {
		if strings.Contains(strings.ToLower(prompt), forbidden) {
			t.Fatalf("manifest prompt exposed forbidden context channel %q:\n%s", forbidden, prompt)
		}
	}
	for _, required := range []string{"Build the current app", "Tests pass", "No external dependencies", "main.go", "one writer per file"} {
		if !strings.Contains(prompt, required) {
			t.Fatalf("manifest prompt omitted %q:\n%s", required, prompt)
		}
	}
}

func TestImplementationManifestRepairLeadsWithExactFailureAndPriorCandidate(t *testing.T) {
	objective := artifacts.Objective{ID: "build_pocket_tasks", Description: "Build Pocket Tasks"}
	criteria := []string{
		"Pocket Tasks supports add, list, and done commands.",
		"go test ./... passes",
	}
	rejected := `{"role_id":"implementation_planner","items":[{"id":"commands","acceptance_criteria":["Commands work"]}]}`
	prompt, err := buildImplementationManifestRepairPrompt(
		objective,
		[]string{"The Go standard library is used exclusively."},
		criteria,
		errors.New(`work item "commands" acceptance criterion "Commands work" is not authoritative; items[0] file work cannot execute a command; storage work item "storage" must depend on a domain provider; verification item must depend on file work item "readme"`),
		rejected,
	)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(prompt, "DIRECT MANIFEST CONTRACT CORRECTION") {
		t.Fatalf("manifest repair does not lead with the rejection: %s", prompt)
	}
	if strings.Index(prompt, "VALIDATION_FAILURE") > strings.Index(prompt, "AUTHORITATIVE_OBJECTIVE") {
		t.Fatal("manifest repair buried validation feedback behind objective context")
	}
	for _, required := range []string{
		`acceptance criterion "Commands work" is not authoritative`,
		rejected,
		criteria[0],
		criteria[1],
		"Preserve unrelated valid fields",
		"copied byte-for-byte",
		"MECHANICAL_EDITS_REQUIRED",
		"Remove the command property entirely",
		"add the named domain provider ID",
		"add every named missing file item ID",
	} {
		if !strings.Contains(prompt, required) {
			t.Fatalf("manifest repair omitted %q", required)
		}
	}
	for _, forbidden := range []string{"conversation_history", "memory_references", "web_evidence"} {
		if strings.Contains(strings.ToLower(prompt), forbidden) {
			t.Fatalf("manifest repair exposed forbidden context channel %q", forbidden)
		}
	}
}

func TestImplementationManifestRepairEscalatesAcrossConfiguredPlannerModels(t *testing.T) {
	runtime := &nativeRuntimeV3{svc: &Service{models: ModelRouting{
		Plan: "qwen3:4b-thinking", Analyze: "qwen2.5-coder:7b", Fast: "qwen2.5-coder:7b",
		Specialist: map[string]string{specialist.RolePlannerSpecialist: "qwen2.5-coder:14b"},
	}}}
	models := runtime.implementationManifestAttemptModels("qwen2.5-coder:7b", 4)
	want := []string{"qwen2.5-coder:7b", "qwen3:4b-thinking", "qwen2.5-coder:14b", "qwen2.5-coder:14b"}
	if strings.Join(models, "|") != strings.Join(want, "|") {
		t.Fatalf("manifest models=%#v, want %#v", models, want)
	}
}

func TestImplementationManifestRepairSkipsDuplicatePlannerAndUsesReasoningModel(t *testing.T) {
	runtime := &nativeRuntimeV3{svc: &Service{models: ModelRouting{
		Plan: "qwen3:4b-thinking", Reasoning: "qwen2.5-coder:14b", Analyze: "qwen2.5-coder:7b", Fast: "qwen2.5-coder:7b",
		Specialist: map[string]string{specialist.RolePlannerSpecialist: "qwen3:4b-thinking"},
	}}}
	models := runtime.implementationManifestAttemptModels("qwen2.5-coder:7b", 4)
	want := []string{"qwen2.5-coder:7b", "qwen3:4b-thinking", "qwen2.5-coder:14b", "qwen2.5-coder:14b"}
	if strings.Join(models, "|") != strings.Join(want, "|") {
		t.Fatalf("manifest models=%#v, want %#v", models, want)
	}
}

func TestImplementationLedgerRequiresDedicatedTestAndDocumentationFiles(t *testing.T) {
	ledger := validImplementationLedger()
	ledger.AcceptanceCriteria = append(ledger.AcceptanceCriteria, "README includes usage examples")
	ledger.Items[4].AcceptanceCriteria = append(ledger.Items[4].AcceptanceCriteria, "README includes usage examples")

	err := validateImplementationLedger(ledger)
	if err == nil || !strings.Contains(err.Error(), "documentation file") {
		t.Fatalf("validateImplementationLedger() err=%v, want documentation ownership rejection", err)
	}

	ledger = validImplementationLedger()
	ledger.Items[3].Discipline = artifacts.ImplementationDisciplineDomain
	err = validateImplementationLedger(ledger)
	if err == nil || !strings.Contains(err.Error(), "test file") {
		t.Fatalf("validateImplementationLedger() err=%v, want test ownership rejection", err)
	}
}

func TestImplementationLedgerRejectsInventedCriteriaAndReversedSeparation(t *testing.T) {
	ledger := validImplementationLedger()
	ledger.Items[1].AcceptanceCriteria = append(ledger.Items[1].AcceptanceCriteria, "Invent a database dependency")
	err := validateImplementationLedger(ledger)
	if err == nil || !strings.Contains(err.Error(), "Invent a database dependency") || !strings.Contains(err.Error(), "not authoritative") {
		t.Fatalf("validateImplementationLedger() err=%v, want invented-criterion rejection", err)
	}

	ledger = separatedImplementationLedger()
	ledger.Items[1].DependsOn = []string{"write_main"}
	ledger.Items[4].Responsibility = "Implement command parsing, task management, and storage in main"
	err = validateImplementationLedger(ledger)
	if err == nil || !strings.Contains(err.Error(), "entrypoint") || !strings.Contains(err.Error(), "downstream") {
		t.Fatalf("validateImplementationLedger() err=%v, want responsibility and dependency direction rejection", err)
	}
}

func TestImplementationLedgerRequiresPersistenceCriterionOnStorageOwner(t *testing.T) {
	ledger := separatedImplementationLedger()
	criterion := "Tasks are persisted as JSON."
	ledger.AcceptanceCriteria = append(ledger.AcceptanceCriteria, criterion)
	ledger.Items[3].AcceptanceCriteria = append(ledger.Items[3].AcceptanceCriteria, criterion)

	err := validateImplementationLedger(ledger)
	if err == nil || !strings.Contains(err.Error(), "storage criterion") {
		t.Fatalf("validateImplementationLedger() err=%v, want persistence ownership rejection", err)
	}
}

func TestImplementationLedgerRejectsGoKeywordPackageDirectory(t *testing.T) {
	ledger := separatedImplementationLedger()
	ledger.Items[3].Path = "interface/cli.go"

	err := validateImplementationLedger(ledger)
	if err == nil || !strings.Contains(err.Error(), "language keyword") {
		t.Fatalf("validateImplementationLedger() err=%v, want Go keyword directory rejection", err)
	}
}

func TestImplementationLedgerRejectsSeparationAsAcceptanceCriterion(t *testing.T) {
	ledger := separatedImplementationLedger()
	criterion := "Domain/storage logic is separated from command parsing."
	ledger.AcceptanceCriteria = append(ledger.AcceptanceCriteria, criterion)
	ledger.Items[1].AcceptanceCriteria = append(ledger.Items[1].AcceptanceCriteria, criterion)

	err := validateImplementationLedger(ledger)
	if err == nil || !strings.Contains(err.Error(), "global implementation constraint") {
		t.Fatalf("validateImplementationLedger() err=%v, want global constraint ownership rejection", err)
	}
}

func separatedImplementationLedger() artifacts.ImplementationLedgerArtifact {
	return artifacts.ImplementationLedgerArtifact{
		ObjectiveID: "build_app", Objective: "Build app",
		Constraints:        []string{"Separate domain/storage logic from command parsing"},
		AcceptanceCriteria: []string{"Tests pass."}, Revision: 1,
		Items: []artifacts.ImplementationWorkItem{
			{ID: "write_module", Kind: artifacts.ImplementationWorkKindFile, Discipline: artifacts.ImplementationDisciplineBootstrap, Path: "go.mod", Responsibility: "Declare module", Status: artifacts.ImplementationWorkStatusPending},
			{ID: "write_domain", Kind: artifacts.ImplementationWorkKindFile, Discipline: artifacts.ImplementationDisciplineDomain, Path: "domain.go", Responsibility: "Implement domain task transitions", DependsOn: []string{"write_module"}, Status: artifacts.ImplementationWorkStatusPending},
			{ID: "write_storage", Kind: artifacts.ImplementationWorkKindFile, Discipline: artifacts.ImplementationDisciplineStorage, Path: "storage.go", Responsibility: "Implement JSON storage", DependsOn: []string{"write_domain"}, Status: artifacts.ImplementationWorkStatusPending},
			{ID: "write_commands", Kind: artifacts.ImplementationWorkKindFile, Discipline: artifacts.ImplementationDisciplineInterface, Path: "commands.go", Responsibility: "Implement command parsing", DependsOn: []string{"write_domain", "write_storage"}, Status: artifacts.ImplementationWorkStatusPending},
			{ID: "write_main", Kind: artifacts.ImplementationWorkKindFile, Discipline: artifacts.ImplementationDisciplineEntrypoint, Path: "main.go", Responsibility: "Wire the CLI entrypoint", DependsOn: []string{"write_commands"}, Status: artifacts.ImplementationWorkStatusPending},
			{ID: "write_tests", Kind: artifacts.ImplementationWorkKindFile, Discipline: artifacts.ImplementationDisciplineTest, Path: "main_test.go", Responsibility: "Test the contracts", DependsOn: []string{"write_main", "write_commands"}, Status: artifacts.ImplementationWorkStatusPending},
			{ID: "verify_all", Kind: artifacts.ImplementationWorkKindVerification, Discipline: artifacts.ImplementationDisciplineVerification, Responsibility: "Run tests", DependsOn: []string{"write_module", "write_main", "write_domain", "write_storage", "write_commands", "write_tests"}, AcceptanceCriteria: []string{"Tests pass."}, Command: &artifacts.ImplementationCommand{Program: "go", Args: []string{"test", "./..."}}, Status: artifacts.ImplementationWorkStatusPending},
		},
	}
}
