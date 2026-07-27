package worker

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/artifacts"
)

func TestImplementationLedgerRejectsDuplicateFileWriters(t *testing.T) {
	ledger := validImplementationLedger()
	duplicate := ledger.Items[0]
	duplicate.ID = "second_main_writer"
	ledger.Items = append(ledger.Items, duplicate)

	err := validateImplementationLedger(ledger)
	if err == nil || !strings.Contains(err.Error(), "one writer") {
		t.Fatalf("validateImplementationLedger() err=%v, want duplicate-writer rejection", err)
	}
}

func TestMutationObjectiveCannotRouteThroughGenericMonolithicToolLoop(t *testing.T) {
	objective := artifacts.Objective{
		ID: "build_app", Description: "Build the app", RequiresAction: true,
		RequiredCapabilities: []string{capabilityWorkspaceWrite, capabilityCommandExecute},
	}
	if got := executionModeForObjective(objective); got != subtaskExecutionModeImplementation {
		t.Fatalf("executionModeForObjective()=%q, want implementation ledger", got)
	}
	objective.RequiredCapabilities = []string{capabilityWorkspaceRead}
	if got := executionModeForObjective(objective); got != subtaskExecutionModeGeneral {
		t.Fatalf("read-only objective mode=%q, want general tool loop", got)
	}
}

func TestImplementationLedgerRejectsCyclesAndMissingVerification(t *testing.T) {
	ledger := validImplementationLedger()
	ledger.Items = ledger.Items[:2]
	ledger.Items[0].DependsOn = []string{"write_commands"}
	ledger.Items[1].DependsOn = []string{"write_module"}

	err := validateImplementationLedger(ledger)
	if err == nil || !strings.Contains(err.Error(), "cycle") || !strings.Contains(err.Error(), "verification") {
		t.Fatalf("validateImplementationLedger() err=%v, want cycle and verification failures", err)
	}
}

func TestImplementationLedgerRejectsUnsafeOrNonVerificationCommands(t *testing.T) {
	ledger := validImplementationLedger()
	ledger.Items[4].Command = &artifacts.ImplementationCommand{Program: "go", Args: []string{"mod", "init", "pocket_tasks"}}

	err := validateImplementationLedger(ledger)
	if err == nil || !strings.Contains(err.Error(), "verification command") {
		t.Fatalf("validateImplementationLedger() err=%v, want initializer rejection", err)
	}

	ledger = validImplementationLedger()
	ledger.Items[4].Command = &artifacts.ImplementationCommand{Program: "sh", Args: []string{"-c", "go test ./..."}}
	err = validateImplementationLedger(ledger)
	if err == nil || !strings.Contains(err.Error(), "not allowlisted") {
		t.Fatalf("validateImplementationLedger() err=%v, want command allowlist rejection", err)
	}
}

func TestReadyImplementationWorkHonorsDependencies(t *testing.T) {
	ledger := validImplementationLedger()
	index, err := readyImplementationWorkItem(ledger)
	if err != nil {
		t.Fatal(err)
	}
	if index != 0 {
		t.Fatalf("ready index=%d, want first dependency-free file", index)
	}

	ledger.Items[0].Status = artifacts.ImplementationWorkStatusCompleted
	index, err = readyImplementationWorkItem(ledger)
	if err != nil {
		t.Fatal(err)
	}
	if index != 1 {
		t.Fatalf("ready index=%d, want dependent test file", index)
	}

	ledger.Items[1].Status = artifacts.ImplementationWorkStatusCompleted
	index, err = readyImplementationWorkItem(ledger)
	if err != nil {
		t.Fatal(err)
	}
	if index != 2 {
		t.Fatalf("ready index=%d, want entrypoint file", index)
	}
	ledger.Items[2].Status = artifacts.ImplementationWorkStatusCompleted
	index, err = readyImplementationWorkItem(ledger)
	if err != nil || index != 3 {
		t.Fatalf("ready index=%d err=%v, want test file", index, err)
	}
	ledger.Items[3].Status = artifacts.ImplementationWorkStatusCompleted
	index, err = readyImplementationWorkItem(ledger)
	if err != nil || index != 4 {
		t.Fatalf("ready index=%d err=%v, want final verification", index, err)
	}
}

func TestImplementationWriterContextExcludesUnrelatedFilesAndObjectiveHistory(t *testing.T) {
	root := t.TempDir()
	mustWriteImplementationFixture(t, root, "go.mod", "module pockettasks\n\ngo 1.26\n")
	mustWriteImplementationFixture(t, root, "main.go", "package main\n\nfunc main() {}\n")
	mustWriteImplementationFixture(t, root, "cli.go", "package main\n\ntype Task struct{ ID int }\n")
	mustWriteImplementationFixture(t, root, "private-memory.txt", "BUILD THE OLD MUSIC APP INSTEAD\n")

	ledger := validImplementationLedger()
	ledger.Objective = "A huge historical objective that must not reach a file worker"
	item := ledger.Items[2]

	context, err := readImplementationWorkContext(root, ledger, item)
	if err != nil {
		t.Fatal(err)
	}
	prompt, err := buildImplementationWriterPrompt(item, context, "main.go:9: undefined: Task")
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{
		"BUILD THE OLD MUSIC APP INSTEAD",
		"huge historical objective",
		"private-memory.txt",
	} {
		if strings.Contains(prompt, forbidden) {
			t.Fatalf("minimal writer prompt leaked %q:\n%s", forbidden, prompt)
		}
	}
	for _, required := range []string{
		"main.go:9: undefined: Task",
		"cli.go",
		"type Task struct",
		"write_commands",
	} {
		if !strings.Contains(prompt, required) {
			t.Fatalf("minimal writer prompt omitted %q:\n%s", required, prompt)
		}
	}
}

func TestImplementationProviderContextIncludesConsumerContractsWithoutConsumerFiles(t *testing.T) {
	root := t.TempDir()
	mustWriteImplementationFixture(t, root, "go.mod", "module omnidex.local/app\n\ngo 1.20\n")
	mustWriteImplementationFixture(t, root, "private-memory.txt", "BUILD THE OLD MUSIC APP INSTEAD\n")

	ledger := separatedImplementationLedger()
	persistenceCriterion := "Tasks are persisted as JSON."
	commandCriterion := "The program supports add, list, and done commands."
	ledger.AcceptanceCriteria = append(ledger.AcceptanceCriteria, persistenceCriterion, commandCriterion)
	ledger.Items[2].AcceptanceCriteria = []string{persistenceCriterion}
	ledger.Items[3].AcceptanceCriteria = []string{commandCriterion}
	item := ledger.Items[1]

	context, err := readImplementationWorkContext(root, ledger, item)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := json.Marshal(context)
	if err != nil {
		t.Fatal(err)
	}
	encoded := string(raw)
	for _, required := range []string{"write_storage", persistenceCriterion, "write_commands", commandCriterion} {
		if !strings.Contains(encoded, required) {
			t.Fatalf("provider context omitted consumer contract %q:\n%s", required, encoded)
		}
	}
	for _, forbidden := range []string{"BUILD THE OLD MUSIC APP INSTEAD", "private-memory.txt", "consumer_content"} {
		if strings.Contains(encoded, forbidden) {
			t.Fatalf("provider context leaked %q:\n%s", forbidden, encoded)
		}
	}
}

func TestImplementationBootstrapWriterKnowsDependencyAbsenceIsNotABlocker(t *testing.T) {
	item := artifacts.ImplementationWorkItem{
		ID: "write_module", Kind: artifacts.ImplementationWorkKindFile,
		Discipline: artifacts.ImplementationDisciplineBootstrap,
		Path:       "go.mod", Responsibility: "Declare the Go module",
	}
	prompt, err := buildImplementationWriterPrompt(item, implementationWorkContext{
		Target:            implementationContextFile{Path: "go.mod", Exists: false},
		GlobalConstraints: []string{"Use only the Go standard library"},
	}, "")
	if err != nil {
		t.Fatal(err)
	}
	for _, required := range []string{"BOOTSTRAP_FACTS", "No external dependencies are missing", "concrete non-placeholder local module path"} {
		if !strings.Contains(prompt, required) {
			t.Fatalf("bootstrap prompt omitted %q:\n%s", required, prompt)
		}
	}
	for _, forbidden := range []string{`write|satisfied|blocked`, "return blocked"} {
		if strings.Contains(strings.ToLower(prompt), strings.ToLower(forbidden)) {
			t.Fatalf("bootstrap prompt still offered unsupported blocker path %q:\n%s", forbidden, prompt)
		}
	}
}

func TestImplementationWriterDecisionCannotDriftPathOrReturnPlaceholder(t *testing.T) {
	item := validImplementationLedger().Items[2]
	_, err := parseImplementationFileDecision(`{
		"role_id":"file_worker",
		"work_item_id":"write_main",
		"status":"write",
		"path":"other.go",
		"content":"package main\n",
		"error":""
	}`, item)
	if err == nil || !strings.Contains(err.Error(), "path drift") {
		t.Fatalf("parseImplementationFileDecision() err=%v, want path drift rejection", err)
	}

	_, err = parseImplementationFileDecision(`{
		"role_id":"file_worker",
		"work_item_id":"write_main",
		"status":"write",
		"path":"main.go",
		"content":"package main\n\n// TODO: implement later\n",
		"error":""
	}`, item)
	if err == nil || !strings.Contains(err.Error(), "placeholder") {
		t.Fatalf("parseImplementationFileDecision() err=%v, want placeholder rejection", err)
	}
}

func TestImplementationReviewAndTriageContractsAreStrict(t *testing.T) {
	item := validImplementationLedger().Items[2]
	_, err := parseImplementationReviewDecision(`{
		"role_id":"file_reviewer",
		"work_item_id":"wrong",
		"verdict":"pass",
		"findings":[]
	}`, item)
	if err == nil || !strings.Contains(err.Error(), "work item drift") {
		t.Fatalf("parseImplementationReviewDecision() err=%v", err)
	}

	ledger := validImplementationLedger()
	_, err = parseImplementationTriageDecision(`{
		"role_id":"failure_triager",
		"verification_item_id":"verify_all",
		"owner_id":"verify_all",
		"feedback":"Fix it"
	}`, ledger, ledger.Items[4])
	if err == nil || !strings.Contains(err.Error(), "file owner") {
		t.Fatalf("parseImplementationTriageDecision() err=%v", err)
	}
}

func TestReopeningFailedOwnerPreservesUnrelatedCompletedWork(t *testing.T) {
	ledger := validImplementationLedger()
	for index := 0; index < 4; index++ {
		ledger.Items[index].Status = artifacts.ImplementationWorkStatusCompleted
	}
	ledger.Items[4].Status = artifacts.ImplementationWorkStatusPending
	ledger.Items = append(ledger.Items[:4], artifacts.ImplementationWorkItem{
		ID: "write_readme", Kind: artifacts.ImplementationWorkKindFile,
		Discipline: artifacts.ImplementationDisciplineDocumentation,
		Path:       "README.md", Responsibility: "Document usage",
		AcceptanceCriteria: []string{"Shows all commands"},
		Status:             artifacts.ImplementationWorkStatusCompleted,
	}, ledger.Items[4])

	if err := reopenImplementationOwner(&ledger, "write_main", "main.go:12: undefined: Task"); err != nil {
		t.Fatal(err)
	}
	if ledger.Items[2].Status != artifacts.ImplementationWorkStatusPending || !strings.Contains(ledger.Items[2].LastError, "undefined: Task") {
		t.Fatalf("failed owner was not reopened with exact feedback: %+v", ledger.Items[2])
	}
	if ledger.Items[1].Status != artifacts.ImplementationWorkStatusCompleted {
		t.Fatalf("dependent completed work was unnecessarily discarded: %+v", ledger.Items[1])
	}
	if ledger.Items[3].Status != artifacts.ImplementationWorkStatusCompleted || ledger.Items[4].Status != artifacts.ImplementationWorkStatusCompleted {
		t.Fatalf("unrelated completed work was unnecessarily discarded: %+v %+v", ledger.Items[3], ledger.Items[4])
	}
}

func validImplementationLedger() artifacts.ImplementationLedgerArtifact {
	return artifacts.ImplementationLedgerArtifact{
		ObjectiveID:        "build_pocket_tasks",
		Objective:          "Build Pocket Tasks application",
		AcceptanceCriteria: []string{"Commands add, list, and done work", "Tests invalid commands", "All tests pass"},
		Revision:           1,
		Items: []artifacts.ImplementationWorkItem{
			{
				ID: "write_module", Kind: artifacts.ImplementationWorkKindFile,
				Discipline: artifacts.ImplementationDisciplineBootstrap,
				Path:       "go.mod", Responsibility: "Declare the standalone Go module",
				Status: artifacts.ImplementationWorkStatusPending,
			},
			{
				ID: "write_commands", Kind: artifacts.ImplementationWorkKindFile,
				Discipline: artifacts.ImplementationDisciplineInterface,
				Path:       "cli.go", Responsibility: "Implement the CLI boundary",
				DependsOn:          []string{"write_module"},
				AcceptanceCriteria: []string{"Commands add, list, and done work"},
				Status:             artifacts.ImplementationWorkStatusPending,
			},
			{
				ID: "write_main", Kind: artifacts.ImplementationWorkKindFile,
				Discipline: artifacts.ImplementationDisciplineEntrypoint,
				Path:       "main.go", Responsibility: "Wire the CLI entrypoint",
				DependsOn: []string{"write_commands"}, Status: artifacts.ImplementationWorkStatusPending,
			},
			{
				ID: "write_tests", Kind: artifacts.ImplementationWorkKindFile,
				Discipline: artifacts.ImplementationDisciplineTest,
				Path:       "cli_test.go", Responsibility: "Test success and failure behavior",
				DependsOn:          []string{"write_commands", "write_main"},
				AcceptanceCriteria: []string{"Tests invalid commands"},
				Status:             artifacts.ImplementationWorkStatusPending,
			},
			{
				ID: "verify_all", Kind: artifacts.ImplementationWorkKindVerification,
				Discipline:         artifacts.ImplementationDisciplineVerification,
				Responsibility:     "Run the authoritative test suite",
				DependsOn:          []string{"write_module", "write_commands", "write_main", "write_tests"},
				AcceptanceCriteria: []string{"All tests pass"},
				Command:            &artifacts.ImplementationCommand{Program: "go", Args: []string{"test", "./..."}, TimeoutSeconds: 180},
				Status:             artifacts.ImplementationWorkStatusPending,
			},
		},
	}
}

func mustWriteImplementationFixture(t *testing.T, root, path, content string) {
	t.Helper()
	target := filepath.Join(root, path)
	if err := os.MkdirAll(filepath.Dir(target), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(target, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}
