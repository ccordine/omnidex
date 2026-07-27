package worker

import (
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/artifacts"
)

func TestImplementationLedgerRequiresStandaloneToolchainBootstrap(t *testing.T) {
	ledger := validImplementationLedger()
	ledger.Items = append(ledger.Items[:0], ledger.Items[1:]...)
	for index := range ledger.Items {
		ledger.Items[index].DependsOn = removeImplementationDependency(ledger.Items[index].DependsOn, "write_module")
	}
	err := validateImplementationLedger(ledger)
	if err == nil || !strings.Contains(err.Error(), "go.mod") {
		t.Fatalf("validateImplementationLedger() err=%v, want standalone go.mod rejection", err)
	}
}

func TestImplementationLedgerRejectsSplitCommandAndFailureOwners(t *testing.T) {
	ledger := validImplementationLedger()
	ledger.AcceptanceCriteria = append(ledger.AcceptanceCriteria, "Invalid commands return nonzero failures")
	ledger.Items = append(ledger.Items[:2], append([]artifacts.ImplementationWorkItem{{
		ID: "write_errors", Kind: artifacts.ImplementationWorkKindFile,
		Discipline: artifacts.ImplementationDisciplineInterface,
		Path:       "errors.go", Responsibility: "Handle invalid commands",
		DependsOn:          []string{"write_commands"},
		AcceptanceCriteria: []string{"Invalid commands return nonzero failures"},
		Status:             artifacts.ImplementationWorkStatusPending,
	}}, ledger.Items[2:]...)...)
	ledger.Items[len(ledger.Items)-1].DependsOn = append(ledger.Items[len(ledger.Items)-1].DependsOn, "write_errors")

	err := validateImplementationLedger(ledger)
	if err == nil || !strings.Contains(err.Error(), "one interface owner") {
		t.Fatalf("validateImplementationLedger() err=%v, want split interface owner rejection", err)
	}
}

func TestImplementationCandidateRejectsPlaceholderImportAndDuplicateEntrypoint(t *testing.T) {
	storage := artifacts.ImplementationWorkItem{
		ID: "write_storage", Path: "storage/task.go",
		Discipline: artifacts.ImplementationDisciplineStorage,
	}
	_, err := parseImplementationFileDecision(`{
		"role_id":"file_worker","work_item_id":"write_storage","status":"write","path":"storage/task.go",
		"content":"package storage\n\nimport _ \"your-module/domain\"\n","error":""
	}`, storage)
	if err == nil || !strings.Contains(err.Error(), "module-path placeholder") {
		t.Fatalf("parseImplementationFileDecision() err=%v, want placeholder import rejection", err)
	}

	interfaceItem := artifacts.ImplementationWorkItem{
		ID: "write_commands", Path: "cli.go",
		Discipline: artifacts.ImplementationDisciplineInterface,
	}
	err = validateImplementationCandidateContent(interfaceItem, "package main\n\nfunc main() {}\n")
	if err == nil || !strings.Contains(err.Error(), "cannot declare a program entrypoint") {
		t.Fatalf("validateImplementationCandidateContent() err=%v, want duplicate entrypoint rejection", err)
	}
}

func TestImplementationCandidateRejectsExplicitPlaceholderAndFutureWork(t *testing.T) {
	item := artifacts.ImplementationWorkItem{
		ID: "write_domain", Path: "domain/task.go",
		Discipline: artifacts.ImplementationDisciplineDomain,
	}
	candidate := "package domain\n\nfunc IsOverdue() bool {\n\t// This is a placeholder for future logic.\n\treturn false\n}\n"
	_, err := parseImplementationFileDecision(`{
		"role_id":"file_worker","work_item_id":"write_domain","status":"write","path":"domain/task.go",
		"content":"package domain\n\nfunc IsOverdue() bool {\n\t// This is a placeholder for future logic.\n\treturn false\n}\n","error":""
	}`, item)
	if err == nil || !strings.Contains(err.Error(), "forbidden placeholder") {
		t.Fatalf("parseImplementationFileDecision() err=%v, want explicit placeholder rejection for %q", err, candidate)
	}
}

func TestImplementationFileDecisionReportsAllCandidateViolations(t *testing.T) {
	item := artifacts.ImplementationWorkItem{
		ID: "write_module", Path: "go.mod",
		Discipline: artifacts.ImplementationDisciplineBootstrap,
	}
	_, err := parseImplementationFileDecision(`{
		"role_id":"file_worker","work_item_id":"write_module","status":"write","path":"go.mod",
		"content":"module github.com/yourusername/yourproject\n\ngo 1.26","error":""
	}`, item)
	if err == nil || !strings.Contains(err.Error(), "ending with a newline") || !strings.Contains(err.Error(), "module-path placeholder") {
		t.Fatalf("parseImplementationFileDecision() err=%v, want every candidate violation", err)
	}
}

func TestImplementationFileDecisionRejectsWorkerDeclaredBlocker(t *testing.T) {
	item := artifacts.ImplementationWorkItem{
		ID: "write_module", Path: "go.mod",
		Discipline: artifacts.ImplementationDisciplineBootstrap,
	}
	_, err := parseImplementationFileDecision(`{
		"role_id":"file_worker","work_item_id":"write_module","status":"blocked","path":"go.mod",
		"content":"","error":"missing dependencies"
	}`, item)
	if err == nil || !strings.Contains(err.Error(), "cannot declare blockers") || !strings.Contains(err.Error(), "server validates dependencies") {
		t.Fatalf("parseImplementationFileDecision() err=%v, want unsupported blocker rejection", err)
	}
}

func TestImplementationGoModuleValidationRejectsMalformedPathAndIncompleteModule(t *testing.T) {
	item := artifacts.ImplementationWorkItem{
		ID: "write_module", Path: "go.mod",
		Discipline: artifacts.ImplementationDisciplineBootstrap,
	}
	for name, candidate := range map[string]string{
		"malformed path":     "module omnid:local/app\n\ngo 1.20\n",
		"missing go version": "module omnidex.local/app\n",
		"duplicate module":   "module omnidex.local/app\nmodule other.local/app\n\ngo 1.20\n",
	} {
		t.Run(name, func(t *testing.T) {
			if err := validateImplementationCandidateContent(item, candidate); err == nil {
				t.Fatalf("validateImplementationCandidateContent() accepted %q", candidate)
			}
		})
	}

	if err := validateImplementationCandidateContent(item, "module omnidex.local/app\n\ngo 1.20\n"); err != nil {
		t.Fatalf("validateImplementationCandidateContent() rejected valid go.mod: %v", err)
	}
}

func TestImplementationGoModuleGetsDeterministicPostWriteCheck(t *testing.T) {
	item := artifacts.ImplementationWorkItem{
		ID: "write_module", Path: "go.mod",
		Discipline: artifacts.ImplementationDisciplineBootstrap,
	}
	call, required := implementationFileCheckCall(item)
	if !required {
		t.Fatal("implementationFileCheckCall() did not require a go.mod check")
	}
	if call.Name != "command.run" || call.Input["program"] != "go" {
		t.Fatalf("implementationFileCheckCall()=%+v, want command.run go", call)
	}
	args, ok := call.Input["args"].([]string)
	if !ok || strings.Join(args, " ") != "list -m" {
		t.Fatalf("implementationFileCheckCall() args=%#v, want go list -m", call.Input["args"])
	}
}

func TestImplementationBootstrapUsesDeterministicReviewAuthority(t *testing.T) {
	bootstrap := artifacts.ImplementationWorkItem{
		ID: "write_module", Path: "go.mod",
		Discipline: artifacts.ImplementationDisciplineBootstrap,
	}
	if implementationSemanticReviewRequired(bootstrap) {
		t.Fatal("bootstrap work must not delegate deterministic go.mod authority to an LLM reviewer")
	}
	interfaceItem := artifacts.ImplementationWorkItem{
		ID: "write_commands", Path: "cli.go",
		Discipline:         artifacts.ImplementationDisciplineInterface,
		AcceptanceCriteria: []string{"Invalid commands return nonzero failures"},
	}
	if !implementationSemanticReviewRequired(interfaceItem) {
		t.Fatal("behavioral acceptance criteria still require independent semantic review")
	}
}

func TestCompilerPathRoutesFailureWithoutLLMTriage(t *testing.T) {
	ledger := validImplementationLedger()
	verification := ledger.Items[len(ledger.Items)-1]
	ledger.Items = append(ledger.Items, artifacts.ImplementationWorkItem{
		ID: "write_storage", Kind: artifacts.ImplementationWorkKindFile,
		Discipline: artifacts.ImplementationDisciplineStorage,
		Path:       "storage/task_storage.go", Responsibility: "Persist tasks",
	})
	failure := "storage/task_storage.go:10:2: package pocket/domain is not in std\ncmd/main.go:8:2: build failed"
	decision, found := deterministicImplementationFailureRoute(ledger, verification, failure)
	if !found || decision.OwnerID != "write_storage" {
		t.Fatalf("route found=%t decision=%+v, want write_storage", found, decision)
	}
}

func TestVerificationRepairGetsFreshBoundedWriterBudget(t *testing.T) {
	ledger := validImplementationLedger()
	item := &ledger.Items[1]
	item.Status = artifacts.ImplementationWorkStatusCompleted
	item.Attempts = maxImplementationItemAttempts

	if err := reopenImplementationOwner(&ledger, item.ID, "cli.go:9: undefined: Run"); err != nil {
		t.Fatal(err)
	}
	if item.Attempts != 0 || item.RepairCycles != 1 || item.Status != artifacts.ImplementationWorkStatusPending {
		t.Fatalf("reopened item=%+v, want fresh generation budget and one repair cycle", *item)
	}
}

func removeImplementationDependency(values []string, target string) []string {
	out := make([]string, 0, len(values))
	for _, value := range values {
		if value != target {
			out = append(out, value)
		}
	}
	return out
}
