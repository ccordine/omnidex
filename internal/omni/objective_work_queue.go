package omni

import (
	"fmt"
	"path/filepath"
	"strings"
)

type WorkItemKind string

const (
	WorkItemKindRead      WorkItemKind = "read"
	WorkItemKindCreate    WorkItemKind = "create"
	WorkItemKindUpdate    WorkItemKind = "update"
	WorkItemKindDelete    WorkItemKind = "delete"
	WorkItemKindVerify    WorkItemKind = "verify"
	WorkItemKindArchitect WorkItemKind = "architect"
)

type WorkItemStatus string

const (
	WorkItemStatusPending WorkItemStatus = "pending"
	WorkItemStatusRunning WorkItemStatus = "running"
	WorkItemStatusPassed  WorkItemStatus = "passed"
	WorkItemStatusFailed  WorkItemStatus = "failed"
	WorkItemStatusBlocked WorkItemStatus = "blocked"
)

type EvidenceKind string

const (
	EvidenceKindFileDiff     EvidenceKind = "file_diff"
	EvidenceKindCommand      EvidenceKind = "command"
	EvidenceKindRead         EvidenceKind = "read"
	EvidenceKindDeleteSafety EvidenceKind = "delete_safety"
	EvidenceKindRationale    EvidenceKind = "rationale"
)

type WorkItemScope struct {
	Root  string   `json:"root,omitempty"`
	Paths []string `json:"paths,omitempty"`
}

type ValidatorSpec struct {
	Name             string         `json:"name,omitempty"`
	RequiredEvidence []EvidenceKind `json:"required_evidence,omitempty"`
}

type WorkItemEvidence struct {
	Kind            EvidenceKind `json:"kind"`
	Path            string       `json:"path,omitempty"`
	Diff            string       `json:"diff,omitempty"`
	Command         string       `json:"command,omitempty"`
	ExitCode        int          `json:"exit_code,omitempty"`
	Stdout          string       `json:"stdout,omitempty"`
	Stderr          string       `json:"stderr,omitempty"`
	SafetyValidated bool         `json:"safety_validated,omitempty"`
	Summary         string       `json:"summary,omitempty"`
}

type ObjectiveWorkItem struct {
	ID               string              `json:"id"`
	ParentID         string              `json:"parent_id,omitempty"`
	Kind             WorkItemKind        `json:"kind"`
	Scope            WorkItemScope       `json:"scope"`
	Instruction      string              `json:"instruction"`
	Validator        ValidatorSpec       `json:"validator_spec"`
	RequiredEvidence []EvidenceKind      `json:"required_evidence"`
	EvidenceRefs     []WorkItemEvidence  `json:"evidence_refs"`
	Status           WorkItemStatus      `json:"status"`
	Children         []ObjectiveWorkItem `json:"children,omitempty"`
}

type WorkValidationResult struct {
	Passed bool
	Reason string
	ItemID string
}

type TypedFinalGateInput struct {
	Items              []ObjectiveWorkItem
	BroadEvaluatorDone bool
	CompletionDone     bool
	EmptyFiles         []string
}

func RequiredEvidenceForWorkItemKind(kind WorkItemKind) []EvidenceKind {
	switch kind {
	case WorkItemKindRead:
		return []EvidenceKind{EvidenceKindRead}
	case WorkItemKindCreate, WorkItemKindUpdate:
		return []EvidenceKind{EvidenceKindFileDiff}
	case WorkItemKindDelete:
		return []EvidenceKind{EvidenceKindDeleteSafety}
	case WorkItemKindVerify:
		return []EvidenceKind{EvidenceKindCommand}
	case WorkItemKindArchitect:
		return nil
	default:
		return nil
	}
}

func ValidateObjectiveWorkForest(items []ObjectiveWorkItem) WorkValidationResult {
	if len(items) == 0 {
		return WorkValidationResult{Passed: false, Reason: "typed objective work queue is empty"}
	}
	for _, item := range items {
		result := ValidateObjectiveWorkTree(item)
		if !result.Passed {
			return result
		}
	}
	return WorkValidationResult{Passed: true, Reason: "all typed objective work items passed"}
}

func ValidateObjectiveWorkTree(item ObjectiveWorkItem) WorkValidationResult {
	if strings.TrimSpace(item.ID) == "" {
		return WorkValidationResult{Passed: false, Reason: "work item missing id"}
	}
	if item.Status == WorkItemStatusFailed || item.Status == WorkItemStatusBlocked {
		return WorkValidationResult{Passed: false, ItemID: item.ID, Reason: fmt.Sprintf("work item %q is %s", item.ID, item.Status)}
	}
	if item.Status != WorkItemStatusPassed {
		return WorkValidationResult{Passed: false, ItemID: item.ID, Reason: fmt.Sprintf("work item %q is %s, not passed", item.ID, firstNonEmpty(string(item.Status), string(WorkItemStatusPending)))}
	}
	if item.Kind == WorkItemKindArchitect {
		if len(item.Children) == 0 {
			return WorkValidationResult{Passed: false, ItemID: item.ID, Reason: fmt.Sprintf("architect item %q has no child work items", item.ID)}
		}
		for _, child := range item.Children {
			result := ValidateObjectiveWorkTree(child)
			if !result.Passed {
				if result.ItemID == "" {
					result.ItemID = child.ID
				}
				result.Reason = fmt.Sprintf("architect item %q cannot pass because child failed: %s", item.ID, result.Reason)
				return result
			}
		}
		return WorkValidationResult{Passed: true, ItemID: item.ID, Reason: fmt.Sprintf("architect item %q passed with all children complete", item.ID)}
	}
	required := item.RequiredEvidence
	if len(required) == 0 {
		required = item.Validator.RequiredEvidence
	}
	if len(required) == 0 {
		required = RequiredEvidenceForWorkItemKind(item.Kind)
	}
	for _, evidenceKind := range required {
		if !workItemHasRequiredEvidence(item, evidenceKind) {
			return WorkValidationResult{Passed: false, ItemID: item.ID, Reason: fmt.Sprintf("work item %q missing required evidence %q", item.ID, evidenceKind)}
		}
	}
	return WorkValidationResult{Passed: true, ItemID: item.ID, Reason: fmt.Sprintf("work item %q passed required evidence", item.ID)}
}

func validateObjectiveWorkEvidenceTree(item ObjectiveWorkItem) WorkValidationResult {
	if strings.TrimSpace(item.ID) == "" {
		return WorkValidationResult{Passed: false, Reason: "work item missing id"}
	}
	if item.Status == WorkItemStatusFailed || item.Status == WorkItemStatusBlocked {
		return WorkValidationResult{Passed: false, ItemID: item.ID, Reason: fmt.Sprintf("work item %q is %s", item.ID, item.Status)}
	}
	if item.Kind == WorkItemKindArchitect {
		if len(item.Children) == 0 {
			return WorkValidationResult{Passed: false, ItemID: item.ID, Reason: fmt.Sprintf("architect item %q has no child work items", item.ID)}
		}
		for _, child := range item.Children {
			if child.Status != WorkItemStatusPassed {
				return WorkValidationResult{Passed: false, ItemID: child.ID, Reason: fmt.Sprintf("architect item %q cannot pass because child %q is %s", item.ID, child.ID, child.Status)}
			}
			if result := ValidateObjectiveWorkTree(child); !result.Passed {
				result.Reason = fmt.Sprintf("architect item %q cannot pass because child failed: %s", item.ID, result.Reason)
				return result
			}
		}
		return WorkValidationResult{Passed: true, ItemID: item.ID, Reason: fmt.Sprintf("architect item %q evidence supports passed status", item.ID)}
	}
	required := item.RequiredEvidence
	if len(required) == 0 {
		required = item.Validator.RequiredEvidence
	}
	if len(required) == 0 {
		required = RequiredEvidenceForWorkItemKind(item.Kind)
	}
	for _, evidenceKind := range required {
		if !workItemHasRequiredEvidence(item, evidenceKind) {
			return WorkValidationResult{Passed: false, ItemID: item.ID, Reason: fmt.Sprintf("work item %q missing required evidence %q", item.ID, evidenceKind)}
		}
	}
	return WorkValidationResult{Passed: true, ItemID: item.ID, Reason: fmt.Sprintf("work item %q evidence supports passed status", item.ID)}
}

func workItemHasRequiredEvidence(item ObjectiveWorkItem, required EvidenceKind) bool {
	for _, evidence := range item.EvidenceRefs {
		if evidence.Kind != required {
			continue
		}
		switch required {
		case EvidenceKindFileDiff:
			if strings.TrimSpace(evidence.Path) != "" && strings.TrimSpace(evidence.Diff) != "" {
				return true
			}
		case EvidenceKindCommand:
			if strings.TrimSpace(evidence.Command) != "" && evidence.ExitCode == 0 {
				return true
			}
		case EvidenceKindRead:
			if strings.TrimSpace(evidence.Path) != "" || strings.TrimSpace(evidence.Stdout) != "" || strings.TrimSpace(evidence.Summary) != "" {
				return true
			}
		case EvidenceKindDeleteSafety:
			if evidence.SafetyValidated && strings.TrimSpace(evidence.Path) != "" {
				return true
			}
		}
	}
	return false
}

func EvaluateTypedFinalGate(input TypedFinalGateInput) WorkValidationResult {
	if len(input.EmptyFiles) > 0 {
		return WorkValidationResult{Passed: false, Reason: "empty file(s) block final success: " + strings.Join(input.EmptyFiles, ",")}
	}
	result := ValidateObjectiveWorkForest(input.Items)
	if !result.Passed {
		return result
	}
	if input.CompletionDone || input.BroadEvaluatorDone {
		return WorkValidationResult{Passed: true, Reason: "typed recursive work passed before final acceptance"}
	}
	return WorkValidationResult{Passed: true, Reason: "typed recursive work passed"}
}

func BuildObjectiveWorkItemsFromLedger(prompt string, ledger []StructuredObjective, workingDir string, survey WorksiteSurvey) []ObjectiveWorkItem {
	items := []ObjectiveWorkItem{}
	for _, objective := range pendingAndSatisfiedObjectives(ledger) {
		if strings.TrimSpace(objective.ID) == "" || !structuredObjectiveBlocksCompletion(objective) {
			continue
		}
		item := ObjectiveWorkItem{
			ID:          objective.ID,
			Kind:        objectiveWorkItemKind(prompt, objective),
			Scope:       WorkItemScope{Root: workingDir},
			Instruction: strings.TrimSpace(objective.Description),
			Status:      WorkItemStatusPending,
		}
		item.RequiredEvidence = RequiredEvidenceForWorkItemKind(item.Kind)
		item.Validator = ValidatorSpec{Name: string(item.Kind) + "_validator", RequiredEvidence: item.RequiredEvidence}
		if item.Kind == WorkItemKindArchitect {
			item.Children = architectChildrenForObjective(prompt, objective, workingDir, survey)
		}
		items = append(items, item)
	}
	return items
}

func pendingAndSatisfiedObjectives(ledger []StructuredObjective) []StructuredObjective {
	out := []StructuredObjective{}
	for _, objective := range ledger {
		if structuredObjectiveBlocksCompletion(objective) {
			out = append(out, objective)
		}
	}
	return out
}

func objectiveWorkItemKind(prompt string, objective StructuredObjective) WorkItemKind {
	objectiveText := strings.ToLower(objective.ID + " " + objective.Description)
	switch {
	case strings.Contains(objectiveText, "weather") || strings.Contains(objectiveText, "time") || strings.Contains(objectiveText, "current") || (strings.Contains(objectiveText, "complete") && !strings.Contains(objectiveText, "app") && !strings.Contains(objectiveText, "ui") && !strings.Contains(objectiveText, "project")):
		return WorkItemKindVerify
	case strings.Contains(objectiveText, "operation") || strings.Contains(objectiveText, "function") || strings.Contains(objectiveText, "logic") || strings.Contains(objectiveText, "state") || strings.Contains(objectiveText, "memory"):
		return WorkItemKindUpdate
	case strings.Contains(objectiveText, "delete") || strings.Contains(objectiveText, "remove"):
		return WorkItemKindDelete
	case strings.Contains(objectiveText, "install") || strings.Contains(objectiveText, "initialize") || strings.Contains(objectiveText, "webpack"):
		return WorkItemKindVerify
	case strings.Contains(objectiveText, "verify") || strings.Contains(objectiveText, "test") || strings.Contains(objectiveText, "build") && strings.Contains(objectiveText, "run"):
		return WorkItemKindVerify
	case strings.Contains(objectiveText, "read") || strings.Contains(objectiveText, "inspect"):
		return WorkItemKindRead
	}
	text := strings.ToLower(prompt + " " + objectiveText)
	switch {
	case strings.Contains(text, "read") || strings.Contains(text, "inspect"):
		return WorkItemKindRead
	case strings.Contains(text, "delete") || strings.Contains(text, "remove"):
		return WorkItemKindDelete
	case strings.Contains(text, "verify") || strings.Contains(text, "test") || strings.Contains(text, "build") && strings.Contains(text, "run"):
		return WorkItemKindVerify
	case strings.Contains(text, "component"):
		if strings.Contains(text, "create") || strings.Contains(text, "setup") {
			return WorkItemKindCreate
		}
		return WorkItemKindUpdate
	case strings.Contains(text, "context") || strings.Contains(text, "function") || strings.Contains(text, "logic") || strings.Contains(text, "state"):
		return WorkItemKindUpdate
	case strings.Contains(text, "app") || strings.Contains(text, "project") || strings.Contains(text, "ui") || strings.Contains(text, "crud") || strings.Contains(text, "implement") || strings.Contains(text, "source"):
		return WorkItemKindArchitect
	case strings.Contains(text, "update") || strings.Contains(text, "modify"):
		return WorkItemKindUpdate
	default:
		return WorkItemKindCreate
	}
}

func architectChildrenForObjective(prompt string, objective StructuredObjective, workingDir string, survey WorksiteSurvey) []ObjectiveWorkItem {
	contract := buildImplementationArchitectContract(prompt, "Implementation architect target root: "+architectTargetRootForWorkQueue(workingDir)+". Create or modify the actual project files.", workingDir, survey, nil)
	children := []ObjectiveWorkItem{}
	for _, archItem := range contract.WorkQueue {
		kind := WorkItemKindUpdate
		switch archItem.Operation {
		case "create":
			kind = WorkItemKindCreate
		case "verify":
			kind = WorkItemKindVerify
		}
		child := ObjectiveWorkItem{
			ID:          objective.ID + "." + archItem.ID,
			ParentID:    objective.ID,
			Kind:        kind,
			Scope:       WorkItemScope{Root: filepath.Join(workingDir, archItem.CWD), Paths: []string{archItem.Path}},
			Instruction: archItem.Description,
			Status:      WorkItemStatusPending,
		}
		child.RequiredEvidence = RequiredEvidenceForWorkItemKind(kind)
		child.Validator = ValidatorSpec{Name: string(kind) + "_validator", RequiredEvidence: child.RequiredEvidence}
		children = append(children, child)
	}
	return children
}

func architectTargetRootForWorkQueue(workingDir string) string {
	if root := firstNestedAppRootWithFiles(workingDir); root != "" {
		return root
	}
	return "."
}

func ReconcileObjectiveWorkItemsFromObservations(items []ObjectiveWorkItem, observations []StructuredCommandObservation) []ObjectiveWorkItem {
	out := make([]ObjectiveWorkItem, len(items))
	copy(out, items)
	for i := range out {
		out[i] = reconcileWorkItem(out[i], observations)
	}
	return out
}

func reconcileWorkItem(item ObjectiveWorkItem, observations []StructuredCommandObservation) ObjectiveWorkItem {
	for i := range item.Children {
		item.Children[i] = reconcileWorkItem(item.Children[i], observations)
	}
	for _, obs := range observations {
		if obs.ExitCode != 0 {
			continue
		}
		for _, evidence := range workEvidenceFromObservation(obs) {
			if workEvidenceMatchesItem(evidence, item) && !workItemContainsEvidence(item, evidence) {
				item.EvidenceRefs = append(item.EvidenceRefs, evidence)
			}
		}
	}
	if validateObjectiveWorkEvidenceTree(item).Passed {
		item.Status = WorkItemStatusPassed
	}
	return item
}

func workEvidenceFromObservation(obs StructuredCommandObservation) []WorkItemEvidence {
	command := strings.TrimSpace(obs.Command)
	if command == "" {
		return nil
	}
	lower := strings.ToLower(command)
	evidence := []WorkItemEvidence{}
	if strings.HasPrefix(lower, "architect.apply ") {
		fields := strings.Fields(command)
		if len(fields) >= 3 {
			evidence = append(evidence, WorkItemEvidence{Kind: EvidenceKindFileDiff, Path: fields[len(fields)-1], Diff: "runtime applied generated content"})
		}
	}
	if structuredCommandLooksReadOnlyEvidence(command) {
		evidence = append(evidence, WorkItemEvidence{Kind: EvidenceKindRead, Command: command, ExitCode: obs.ExitCode, Stdout: obs.Stdout, Summary: "read command completed"})
	}
	if commandLooksFileMutation(command) {
		evidence = append(evidence, WorkItemEvidence{Kind: EvidenceKindFileDiff, Path: inferredMutationPath(command), Diff: "command mutated file content", Command: command, ExitCode: obs.ExitCode})
	}
	if commandLooksVerification(command) {
		evidence = append(evidence, WorkItemEvidence{Kind: EvidenceKindCommand, Command: command, ExitCode: obs.ExitCode, Stdout: obs.Stdout, Stderr: obs.Stderr})
	}
	if commandLooksDelete(command) {
		evidence = append(evidence, WorkItemEvidence{Kind: EvidenceKindDeleteSafety, Path: inferredMutationPath(command), Command: command, ExitCode: obs.ExitCode, SafetyValidated: true})
	}
	return evidence
}

func workEvidenceMatchesItem(evidence WorkItemEvidence, item ObjectiveWorkItem) bool {
	if len(item.RequiredEvidence) > 0 && !evidenceKindInList(evidence.Kind, item.RequiredEvidence) {
		return false
	}
	if len(item.Scope.Paths) == 0 || strings.TrimSpace(evidence.Path) == "" {
		return true
	}
	evidencePath := filepath.ToSlash(strings.ToLower(evidence.Path))
	for _, path := range item.Scope.Paths {
		path = filepath.ToSlash(strings.ToLower(strings.TrimSpace(path)))
		if path == "" {
			continue
		}
		if strings.Contains(evidencePath, strings.TrimPrefix(path, "./")) || strings.Contains(path, evidencePath) {
			return true
		}
	}
	return false
}

func evidenceKindInList(kind EvidenceKind, list []EvidenceKind) bool {
	for _, candidate := range list {
		if candidate == kind {
			return true
		}
	}
	return false
}

func workItemContainsEvidence(item ObjectiveWorkItem, evidence WorkItemEvidence) bool {
	for _, existing := range item.EvidenceRefs {
		if existing.Kind == evidence.Kind && existing.Path == evidence.Path && existing.Command == evidence.Command {
			return true
		}
	}
	return false
}

func commandLooksFileMutation(command string) bool {
	lower := strings.ToLower(command)
	return strings.Contains(lower, " > ") || strings.Contains(lower, " >") || strings.Contains(lower, "tee ") || strings.Contains(lower, "apply_patch") || strings.Contains(lower, "sed -i") || strings.Contains(lower, "architect.apply") || strings.Contains(lower, "npm pkg set")
}

func commandLooksVerification(command string) bool {
	lower := strings.ToLower(command)
	for _, needle := range []string{"npm test", "npm run build", "npm init", "npm install", "go test", "cargo test", "zig build test", "grep -q", "test -s", "test -d", "pytest"} {
		if strings.Contains(lower, needle) {
			return true
		}
	}
	return false
}

func commandLooksDelete(command string) bool {
	lower := strings.ToLower(strings.TrimSpace(command))
	return strings.HasPrefix(lower, "rm ") || strings.Contains(lower, " rm ") || strings.HasPrefix(lower, "git rm ")
}

func inferredMutationPath(command string) string {
	fields := strings.Fields(command)
	for i := len(fields) - 2; i >= 0; i-- {
		if fields[i] == ">" || fields[i] == ">>" {
			return strings.Trim(fields[i+1], `"'`)
		}
	}
	for i, field := range fields {
		if strings.ToLower(field) == "npm" && i+2 < len(fields) && strings.ToLower(fields[i+1]) == "pkg" && strings.ToLower(fields[i+2]) == "set" {
			return "package.json"
		}
		if (field == "tee" || strings.HasSuffix(field, "/tee")) && i+1 < len(fields) {
			return strings.Trim(fields[i+1], `"'`)
		}
		if strings.HasPrefix(field, "src/") && filepath.Ext(strings.Trim(field, `"'`)) != "" {
			return strings.Trim(field, `"'`)
		}
	}
	return ""
}
