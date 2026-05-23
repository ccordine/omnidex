package omni

import (
	"strings"
	"testing"
)

func TestObjectiveWorkItemBroadObjectiveCannotPassWithoutEvidence(t *testing.T) {
	item := ObjectiveWorkItem{
		ID:               "build_notes_app",
		Kind:             WorkItemKindCreate,
		Scope:            WorkItemScope{Root: ".", Paths: []string{"src/App.js"}},
		Instruction:      "Build the notes app shell",
		Validator:        ValidatorSpec{RequiredEvidence: []EvidenceKind{EvidenceKindFileDiff}},
		RequiredEvidence: []EvidenceKind{EvidenceKindFileDiff},
		Status:           WorkItemStatusPassed,
	}

	result := ValidateObjectiveWorkTree(item)
	if result.Passed {
		t.Fatal("broad objective passed without required evidence")
	}
	if !strings.Contains(result.Reason, "missing required evidence") {
		t.Fatalf("reason = %q", result.Reason)
	}
}

func TestObjectiveWorkItemCannotPassWhileStatusPendingEvenWithEvidence(t *testing.T) {
	item := passingWorkItem("verify_build", WorkItemKindVerify, EvidenceKindCommand)
	item.Status = WorkItemStatusPending

	result := ValidateObjectiveWorkTree(item)
	if result.Passed {
		t.Fatal("pending item passed final validation because evidence existed")
	}
	if !strings.Contains(result.Reason, "not passed") {
		t.Fatalf("reason = %q", result.Reason)
	}
}

func TestObjectiveWorkItemReconciliationPromotesEvidenceBackedItemToPassed(t *testing.T) {
	items := []ObjectiveWorkItem{passedWorkItem("verify_build", WorkItemKindVerify)}
	reconciled := ReconcileObjectiveWorkItemsFromObservations(items, []StructuredCommandObservation{{
		Command:  "npm run build",
		ExitCode: 0,
	}})

	if len(reconciled) != 1 || reconciled[0].Status != WorkItemStatusPassed {
		t.Fatalf("reconciled item was not promoted to passed: %#v", reconciled)
	}
	if result := ValidateObjectiveWorkTree(reconciled[0]); !result.Passed {
		t.Fatalf("promoted item failed final validation: %#v", result)
	}
}

func TestBuildObjectiveWorkItemsConvertsInputLedgerToTopLevelItems(t *testing.T) {
	items := BuildObjectiveWorkItemsFromLedger("build a notes app", []StructuredObjective{{
		ID:          "complete_notes_app",
		Description: "Implement the notes app UI",
		Status:      "pending",
		Kind:        string(WorkItemKindArchitect),
		Required:    true,
		Source:      structuredObjectiveSourceUserExplicit,
	}}, t.TempDir(), WorksiteSurvey{})

	if len(items) != 1 {
		t.Fatalf("items = %#v", items)
	}
	if items[0].ID != "complete_notes_app" || items[0].Kind != WorkItemKindArchitect {
		t.Fatalf("top-level item = %#v", items[0])
	}
	if len(items[0].Children) == 0 {
		t.Fatalf("architect item should create nested child queue: %#v", items[0])
	}
}

func TestBuildObjectiveWorkItemsUsesExplicitObjectiveKind(t *testing.T) {
	items := BuildObjectiveWorkItemsFromLedger("build a notes app", []StructuredObjective{{
		ID:          "inspect_notes_source",
		Description: "Build wording should not override typed read intent",
		Status:      "pending",
		Kind:        string(WorkItemKindRead),
		Required:    true,
		Source:      structuredObjectiveSourceUserExplicit,
	}}, t.TempDir(), WorksiteSurvey{})

	if len(items) != 1 {
		t.Fatalf("items = %#v", items)
	}
	if items[0].Kind != WorkItemKindRead {
		t.Fatalf("explicit objective kind was not preserved: %#v", items[0])
	}
	if len(items[0].Children) != 0 {
		t.Fatalf("read objective should not be expanded into architect children: %#v", items[0])
	}
}

func TestObjectiveWorkItemCreateRequiresFileDiffEvidence(t *testing.T) {
	item := passedWorkItem("create_note_list", WorkItemKindCreate)
	item.EvidenceRefs = []WorkItemEvidence{{Kind: EvidenceKindCommand, Command: "touch src/components/NoteList.js", ExitCode: 0}}

	result := ValidateObjectiveWorkTree(item)
	if result.Passed {
		t.Fatal("create item passed without file-diff evidence")
	}
}

func TestObjectiveWorkItemUpdateRequiresFileDiffEvidence(t *testing.T) {
	item := passedWorkItem("update_app", WorkItemKindUpdate)
	item.EvidenceRefs = []WorkItemEvidence{{Kind: EvidenceKindCommand, Command: "sed -i s/a/b/ src/App.js", ExitCode: 0}}

	result := ValidateObjectiveWorkTree(item)
	if result.Passed {
		t.Fatal("update item passed without file-diff evidence")
	}
}

func TestObjectiveWorkItemVerifyRequiresCommandEvidence(t *testing.T) {
	item := passedWorkItem("verify_build", WorkItemKindVerify)
	item.EvidenceRefs = []WorkItemEvidence{{Kind: EvidenceKindFileDiff, Path: "src/App.js", Diff: "+app"}}

	result := ValidateObjectiveWorkTree(item)
	if result.Passed {
		t.Fatal("verify item passed without command/test evidence")
	}
}

func TestObjectiveWorkItemReadRequiresReadEvidence(t *testing.T) {
	item := passedWorkItem("read_package", WorkItemKindRead)
	item.EvidenceRefs = []WorkItemEvidence{{Kind: EvidenceKindCommand, Command: "cat package.json", ExitCode: 0}}

	result := ValidateObjectiveWorkTree(item)
	if result.Passed {
		t.Fatal("read item passed without read evidence")
	}
}

func TestObjectiveWorkItemDeleteRequiresSafetyEvidence(t *testing.T) {
	item := passedWorkItem("delete_unused_file", WorkItemKindDelete)
	item.EvidenceRefs = []WorkItemEvidence{{Kind: EvidenceKindFileDiff, Path: "src/unused.js", Diff: "-unused"}}

	result := ValidateObjectiveWorkTree(item)
	if result.Passed {
		t.Fatal("delete item passed without delete safety evidence")
	}
}

func TestObjectiveWorkItemArchitectRequiresAllChildrenPassed(t *testing.T) {
	item := ObjectiveWorkItem{
		ID:          "architect_notes_app",
		Kind:        WorkItemKindArchitect,
		Scope:       WorkItemScope{Root: "."},
		Instruction: "Implement notes app",
		Status:      WorkItemStatusPassed,
		Children: []ObjectiveWorkItem{
			passingWorkItem("write_test", WorkItemKindCreate, EvidenceKindFileDiff),
			passedWorkItem("write_component", WorkItemKindUpdate),
		},
	}

	result := ValidateObjectiveWorkTree(item)
	if result.Passed {
		t.Fatal("architect item passed with incomplete child")
	}
	if !strings.Contains(result.Reason, "child") {
		t.Fatalf("reason = %q", result.Reason)
	}
}

func TestObjectiveWorkItemNestedFailedChildPreventsFinalSuccess(t *testing.T) {
	items := []ObjectiveWorkItem{{
		ID:     "architect_notes_app",
		Kind:   WorkItemKindArchitect,
		Status: WorkItemStatusPassed,
		Children: []ObjectiveWorkItem{
			passingWorkItem("write_test", WorkItemKindCreate, EvidenceKindFileDiff),
			failedWorkItem("write_component", WorkItemKindUpdate),
		},
	}}

	result := ValidateObjectiveWorkForest(items)
	if result.Passed {
		t.Fatal("final typed completion passed with nested failed child")
	}
}

func TestObjectiveWorkItemBlockedItemPreventsFinalSuccess(t *testing.T) {
	items := []ObjectiveWorkItem{blockedWorkItem("verify_build", WorkItemKindVerify)}

	result := ValidateObjectiveWorkForest(items)
	if result.Passed {
		t.Fatal("final typed completion passed with blocked item")
	}
}

func TestTypedFinalGateEmptyFileFailurePreventsSuccess(t *testing.T) {
	gate := EvaluateTypedFinalGate(TypedFinalGateInput{
		Items:              []ObjectiveWorkItem{passingWorkItem("verify_build", WorkItemKindVerify, EvidenceKindCommand)},
		BroadEvaluatorDone: true,
		CompletionDone:     true,
		EmptyFiles:         []string{"src/components/NoteList.js"},
	})

	if gate.Passed {
		t.Fatal("final gate passed with empty files")
	}
	if !strings.Contains(gate.Reason, "empty file") {
		t.Fatalf("reason = %q", gate.Reason)
	}
}

func TestTypedFinalGateBroadEvaluatorCannotConvertIncompleteWorkIntoSuccess(t *testing.T) {
	item := passedWorkItem("create_notes_app", WorkItemKindCreate)
	item.Status = WorkItemStatusPassed
	gate := EvaluateTypedFinalGate(TypedFinalGateInput{
		Items:              []ObjectiveWorkItem{item},
		BroadEvaluatorDone: true,
		CompletionDone:     true,
	})

	if gate.Passed {
		t.Fatal("broad evaluator converted incomplete typed work into success")
	}
}

func TestTypedFinalGateCompletionCheckerCannotUseNaturalLanguageAsProof(t *testing.T) {
	item := passedWorkItem("verify_notes_app", WorkItemKindVerify)
	item.Status = WorkItemStatusPassed
	item.EvidenceRefs = []WorkItemEvidence{{Kind: EvidenceKindRationale, Summary: "The notes app appears complete because the model says so."}}

	gate := EvaluateTypedFinalGate(TypedFinalGateInput{
		Items:          []ObjectiveWorkItem{item},
		CompletionDone: true,
	})

	if gate.Passed {
		t.Fatal("completion checker rationale was accepted as proof")
	}
}

func TestArchitectChildrenPreservePerFileActionKinds(t *testing.T) {
	contract := ImplementationArchitectContract{WorkQueue: []ArchitectWorkItem{
		{ID: "read_app", Operation: "read", CWD: ".", Path: "src/App.js", Description: "Read App source"},
		{ID: "create_list", Operation: "create", CWD: ".", Path: "src/NotesList.js", Description: "Create notes list"},
		{ID: "update_app", Operation: "update", CWD: ".", Path: "src/App.js", Description: "Update App"},
		{ID: "delete_empty", Operation: "delete", CWD: ".", Path: "src/empty.js", Description: "Delete empty placeholder"},
		{ID: "verify_build", Operation: "verify", CWD: ".", Description: "Verify build", Verify: "npm run build"},
	}}
	children := architectChildrenFromContract("complete_app", contract, "/repo/app")
	if len(children) != 5 {
		t.Fatalf("children = %d, want 5", len(children))
	}
	want := []struct {
		kind     WorkItemKind
		evidence EvidenceKind
	}{
		{WorkItemKindRead, EvidenceKindRead},
		{WorkItemKindCreate, EvidenceKindFileDiff},
		{WorkItemKindUpdate, EvidenceKindFileDiff},
		{WorkItemKindDelete, EvidenceKindDeleteSafety},
		{WorkItemKindVerify, EvidenceKindCommand},
	}
	for i, expectation := range want {
		if children[i].Kind != expectation.kind {
			t.Fatalf("child %d kind = %q, want %q", i, children[i].Kind, expectation.kind)
		}
		if len(children[i].RequiredEvidence) != 1 || children[i].RequiredEvidence[0] != expectation.evidence {
			t.Fatalf("child %d required evidence = %#v, want %q", i, children[i].RequiredEvidence, expectation.evidence)
		}
	}
}

func TestArchitectReadAndDeleteObservationsSatisfyScopedItems(t *testing.T) {
	items := []ObjectiveWorkItem{{
		ID:          "architect_parent",
		Kind:        WorkItemKindArchitect,
		Instruction: "own file-scoped work",
		Status:      WorkItemStatusPending,
		Children: architectChildrenFromContract("architect_parent", ImplementationArchitectContract{WorkQueue: []ArchitectWorkItem{
			{ID: "read_app", Operation: "read", CWD: ".", Path: "src/App.js", Description: "Read App source"},
			{ID: "delete_empty", Operation: "delete", CWD: ".", Path: "src/empty.js", Description: "Delete empty placeholder"},
		}}, "/repo/app"),
		RequiredEvidence: RequiredEvidenceForWorkItemKind(WorkItemKindArchitect),
	}}
	reconciled := ReconcileObjectiveWorkItemsFromObservations(items, []StructuredCommandObservation{
		{Command: "architect.read src/App.js", ExitCode: 0, Stdout: "export default function App() {}"},
		{Command: "architect.delete src/empty.js", ExitCode: 0, Stdout: "safety validated"},
	})
	result := ValidateObjectiveWorkForest(reconciled)
	if !result.Passed {
		t.Fatalf("architect read/delete evidence did not satisfy scoped queue: %v", result)
	}
}

func passedWorkItem(id string, kind WorkItemKind) ObjectiveWorkItem {
	return ObjectiveWorkItem{
		ID:               id,
		Kind:             kind,
		Scope:            WorkItemScope{Root: ".", Paths: []string{"src/App.js"}},
		Instruction:      "test work item",
		Validator:        ValidatorSpec{RequiredEvidence: RequiredEvidenceForWorkItemKind(kind)},
		RequiredEvidence: RequiredEvidenceForWorkItemKind(kind),
		Status:           WorkItemStatusPending,
	}
}

func passingWorkItem(id string, kind WorkItemKind, evidence EvidenceKind) ObjectiveWorkItem {
	item := passedWorkItem(id, kind)
	item.Status = WorkItemStatusPassed
	item.EvidenceRefs = []WorkItemEvidence{{Kind: evidence, Path: "src/App.js", Diff: "+content", Command: "npm test", ExitCode: 0, SafetyValidated: true}}
	return item
}

func failedWorkItem(id string, kind WorkItemKind) ObjectiveWorkItem {
	item := passedWorkItem(id, kind)
	item.Status = WorkItemStatusFailed
	return item
}

func blockedWorkItem(id string, kind WorkItemKind) ObjectiveWorkItem {
	item := passedWorkItem(id, kind)
	item.Status = WorkItemStatusBlocked
	return item
}
