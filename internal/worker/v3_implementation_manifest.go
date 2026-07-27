package worker

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/gryph/omnidex/internal/artifacts"
)

type implementationManifestDecision struct {
	RoleID string                       `json:"role_id"`
	Items  []implementationManifestItem `json:"items"`
}

type implementationManifestItem struct {
	ID                 string                           `json:"id"`
	Kind               string                           `json:"kind"`
	Discipline         string                           `json:"discipline"`
	Path               string                           `json:"path"`
	Responsibility     string                           `json:"responsibility"`
	DependsOn          []string                         `json:"depends_on"`
	AcceptanceCriteria []string                         `json:"acceptance_criteria"`
	Command            *artifacts.ImplementationCommand `json:"command,omitempty"`
}

func buildImplementationManifestPrompt(objective artifacts.Objective, constraints, criteria []string, workspaceContext string) (string, error) {
	payloadJSON, err := json.MarshalIndent(struct {
		ObjectiveID        string   `json:"objective_id"`
		Objective          string   `json:"objective"`
		Constraints        []string `json:"constraints"`
		AcceptanceCriteria []string `json:"acceptance_criteria"`
	}{
		ObjectiveID:        strings.TrimSpace(objective.ID),
		Objective:          strings.TrimSpace(objective.Description),
		Constraints:        cleanOrderedStrings(constraints),
		AcceptanceCriteria: cleanOrderedStrings(criteria),
	}, "", "  ")
	if err != nil {
		return "", fmt.Errorf("marshal implementation objective: %w", err)
	}
	workspaceContext = strings.TrimSpace(workspaceContext)
	if workspaceContext == "" {
		return "", fmt.Errorf("implementation manifest requires current workspace context")
	}
	return strings.Join([]string{
		"You are an implementation manifest planner. Decompose one app objective into small file contracts and one authoritative verification command.",
		"Planning is your only job. Do not write source code, give advice, execute commands, or add requirements.",
		"Use one writer per file. Each file item owns exactly one complete path. Dependencies must form an acyclic graph. An edge means the consumer depends on the provider, so low-level domain/storage providers must never depend on the CLI entrypoint that consumes them.",
		"Use typed, non-overlapping disciplines: bootstrap owns toolchain manifests; domain owns business rules; storage owns persistence; interface owns all user input parsing and user-facing failure behavior; entrypoint only wires dependencies; test owns executable tests; documentation owns user docs.",
		"Assign every persistence or JSON-storage criterion to storage. For Go work, every directory component must be a valid package name and must not be a language keyword; use a path such as cli/cli.go, never interface/cli.go.",
		"Never create a source file merely to restate a global constraint such as standard-library-only. Never split behavior into a helper file when another file must call it unless the consumer explicitly depends on that helper. Never create more than one program entrypoint.",
		"Every acceptance criterion assigned to an item must be copied verbatim from the authoritative list. Assign each authoritative acceptance criterion to exactly one owning item. Support files such as a module manifest or thin entrypoint may have an empty acceptance_criteria array. Constraints are global and must not be converted into file work.",
		"When verification uses Go, include one bootstrap item for go.mod and make every Go source/test item depend directly or transitively on it. Equivalent workspace manifests are required for Cargo and npm verification. Downstream workers use that manifest to produce real internal import paths; placeholder module paths are forbidden.",
		"All observable command, option, validation, and nonzero-failure criteria belong to one interface owner so that worker can implement and test one coherent boundary. The entrypoint must depend on that interface and contain wiring only.",
		"Create exactly one verification item. It must depend directly or transitively on every file item and use a shell-free command supported by the workspace (for example go test ./..., cargo test, or npm test).",
		"Existing required files still need an item so a dedicated worker can explicitly confirm or revise them. Do not create two items for one path.",
		"The first response character must be { and the last must be }. Do not use a Markdown code fence or any prose outside the JSON object.",
		implementationManifestResponseContract,
		"AUTHORITATIVE_OBJECTIVE:\n" + string(payloadJSON),
		"CURRENT_WORKSPACE:\n" + workspaceContext,
		implementationControlCommand,
	}, "\n\n"), nil
}

func buildImplementationManifestRepairPrompt(
	objective artifacts.Objective,
	constraints, criteria []string,
	validationErr error,
	rejected string,
) (string, error) {
	if validationErr == nil {
		return "", fmt.Errorf("implementation manifest repair requires a validation failure")
	}
	rejected = strings.TrimSpace(rejected)
	if rejected == "" {
		return "", fmt.Errorf("implementation manifest repair requires the rejected candidate")
	}
	authorityJSON, err := json.MarshalIndent(struct {
		ObjectiveID        string   `json:"objective_id"`
		Objective          string   `json:"objective"`
		Constraints        []string `json:"constraints"`
		AcceptanceCriteria []string `json:"acceptance_criteria"`
	}{
		ObjectiveID:        strings.TrimSpace(objective.ID),
		Objective:          strings.TrimSpace(objective.Description),
		Constraints:        cleanOrderedStrings(constraints),
		AcceptanceCriteria: cleanOrderedStrings(criteria),
	}, "", "  ")
	if err != nil {
		return "", fmt.Errorf("marshal implementation manifest repair authority: %w", err)
	}
	sections := []string{
		"DIRECT MANIFEST CONTRACT CORRECTION. Correct the rejected JSON candidate; do not create an unrelated plan.",
		"VALIDATION_FAILURE:\n" + trimForBudget(validationErr.Error(), 7000),
		"Required action: fix every listed violation now. Preserve unrelated valid fields, file paths, responsibilities, and graph edges. Repeating the rejected candidate is another failure.",
		"Authoritative acceptance criteria are immutable strings. Every criterion assigned to an item must be copied byte-for-byte from AUTHORITATIVE_OBJECTIVE, and every authoritative criterion must have exactly one owner. Delete invented or paraphrased criteria. Global constraints never become file items or acceptance criteria.",
		"Dependencies point from consumer to provider. Add every missing bootstrap, domain, storage, interface, entrypoint, file-coverage, and verification edge named by the validation failure. Include exactly one verification item and put command only on that item.",
	}
	if actions := implementationManifestMechanicalRepairActions(validationErr); actions != "" {
		sections = append(sections, "MECHANICAL_EDITS_REQUIRED:\n"+actions)
	}
	sections = append(sections,
		"PREVIOUS_INVALID_MANIFEST:\n"+trimForBudget(rejected, 10000),
		"AUTHORITATIVE_OBJECTIVE:\n"+string(authorityJSON),
		implementationManifestResponseContract,
		"Return exactly one corrected complete JSON object. The first character must be { and the last must be }. Do not explain the correction.",
	)
	return strings.Join(sections, "\n\n"), nil
}

func implementationManifestMechanicalRepairActions(validationErr error) string {
	if validationErr == nil {
		return ""
	}
	failure := strings.ToLower(validationErr.Error())
	actions := make([]string, 0, 6)
	if strings.Contains(failure, "file work cannot execute a command") {
		actions = append(actions, "Remove the command property entirely from every file item named by VALIDATION_FAILURE. command:{} is still invalid. Only the verification item may contain command.")
	}
	if strings.Contains(failure, "storage work item") && strings.Contains(failure, "domain provider") {
		actions = append(actions, "For each named storage item, add the named domain provider ID to depends_on while preserving its existing bootstrap dependency.")
	}
	if strings.Contains(failure, "verification item must depend on file work item") {
		actions = append(actions, "On the verification item, add every named missing file item ID to depends_on. Keep every existing file dependency.")
	}
	if strings.Contains(failure, "interface criterion") || strings.Contains(failure, "all observable command") {
		actions = append(actions, "Move every command, option, and nonzero-failure criterion named by VALIDATION_FAILURE to one interface item's acceptance_criteria array; remove those criteria from all other items.")
	}
	if strings.Contains(failure, "storage criterion") {
		actions = append(actions, "Move every persistence and JSON-storage criterion named by VALIDATION_FAILURE to the storage item's acceptance_criteria array; remove it from every other item.")
	}
	if strings.Contains(failure, "language keyword") {
		actions = append(actions, "Rename each named Go file path so every directory component is a valid non-keyword package name. Use cli/cli.go instead of interface/cli.go; preserve item IDs and dependency edges.")
	}
	if strings.Contains(failure, "is not assigned to a work item") {
		actions = append(actions, "Copy each exact unassigned authoritative criterion into the acceptance_criteria array of its single typed owner.")
	}
	if strings.Contains(failure, "must have exactly one owner") {
		actions = append(actions, "For each duplicated criterion, keep the exact string on only its single typed owner and delete every other copy.")
	}
	return strings.Join(actions, "\n")
}

const implementationManifestResponseContract = `Return exactly one JSON object with this shape:
{
  "role_id": "implementation_planner",
  "items": [
    {
      "id": "stable_snake_case_id",
      "kind": "file|verification",
      "discipline": "bootstrap|domain|storage|interface|entrypoint|test|documentation|verification",
      "path": "relative/path for file; empty for verification",
      "responsibility": "one bounded responsibility",
      "depends_on": ["earlier_item_id"],
      "acceptance_criteria": ["verbatim authoritative criterion"],
      "command": {"program":"go","args":["test","./..."],"timeout_seconds":180}
    }
  ]
}
Omit command from file items. Include command only on the verification item.`

func parseImplementationManifest(raw string, objective artifacts.Objective, constraints, criteria []string) (artifacts.ImplementationLedgerArtifact, error) {
	var decision implementationManifestDecision
	if err := decodeStrictImplementationJSON(raw, &decision); err != nil {
		return artifacts.ImplementationLedgerArtifact{}, fmt.Errorf("decode implementation manifest: %w", err)
	}
	if decision.RoleID != "implementation_planner" {
		return artifacts.ImplementationLedgerArtifact{}, fmt.Errorf("implementation planner role drift: received %q", decision.RoleID)
	}
	ledger := artifacts.ImplementationLedgerArtifact{
		ObjectiveID:        strings.TrimSpace(objective.ID),
		Objective:          strings.TrimSpace(objective.Description),
		Constraints:        cleanOrderedStrings(constraints),
		AcceptanceCriteria: cleanOrderedStrings(criteria),
		Revision:           1,
		Items:              make([]artifacts.ImplementationWorkItem, 0, len(decision.Items)),
	}
	for _, candidate := range decision.Items {
		ledger.Items = append(ledger.Items, artifacts.ImplementationWorkItem{
			ID:                 strings.TrimSpace(candidate.ID),
			Kind:               strings.TrimSpace(candidate.Kind),
			Discipline:         strings.TrimSpace(candidate.Discipline),
			Path:               strings.TrimSpace(candidate.Path),
			Responsibility:     strings.TrimSpace(candidate.Responsibility),
			DependsOn:          cleanOrderedStrings(candidate.DependsOn),
			AcceptanceCriteria: cleanOrderedStrings(candidate.AcceptanceCriteria),
			Command:            cloneImplementationCommand(candidate.Command),
			Status:             artifacts.ImplementationWorkStatusPending,
		})
	}
	if err := validateImplementationLedger(ledger); err != nil {
		return artifacts.ImplementationLedgerArtifact{}, err
	}
	return ledger, nil
}

func cloneImplementationCommand(command *artifacts.ImplementationCommand) *artifacts.ImplementationCommand {
	if command == nil {
		return nil
	}
	return &artifacts.ImplementationCommand{
		Program:        strings.TrimSpace(command.Program),
		Args:           append([]string(nil), command.Args...),
		TimeoutSeconds: command.TimeoutSeconds,
	}
}
