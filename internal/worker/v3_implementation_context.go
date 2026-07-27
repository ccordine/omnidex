package worker

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/gryph/omnidex/internal/artifacts"
)

const maxImplementationContextBytes = 24 * 1024

type implementationContextFile struct {
	Path    string `json:"path"`
	Exists  bool   `json:"exists"`
	Content string `json:"content,omitempty"`
}

type implementationWorkContext struct {
	Target              implementationContextFile          `json:"target"`
	GlobalConstraints   []string                           `json:"global_constraints"`
	Dependencies        []implementationContextFile        `json:"dependencies"`
	DependencyContracts []implementationDependencyContract `json:"dependency_contracts"`
	ConsumerContracts   []implementationDependencyContract `json:"consumer_contracts"`
}

type implementationDependencyContract struct {
	ID                 string   `json:"id"`
	Path               string   `json:"path"`
	Responsibility     string   `json:"responsibility"`
	AcceptanceCriteria []string `json:"acceptance_criteria"`
}

func readImplementationWorkContext(root string, ledger artifacts.ImplementationLedgerArtifact, item artifacts.ImplementationWorkItem) (implementationWorkContext, error) {
	if item.Kind != artifacts.ImplementationWorkKindFile {
		return implementationWorkContext{}, fmt.Errorf("work item %q does not own a file", item.ID)
	}
	root = strings.TrimSpace(root)
	if root == "" {
		return implementationWorkContext{}, fmt.Errorf("implementation workspace root is required")
	}
	target, size, err := readImplementationContextFile(root, item.Path)
	if err != nil {
		return implementationWorkContext{}, err
	}
	context := implementationWorkContext{Target: target, GlobalConstraints: cleanOrderedStrings(ledger.Constraints)}
	total := size
	byID := implementationItemIndexes(ledger)
	closure := implementationDependencyClosure(item, ledger, byID)
	for _, dependency := range ledger.Items {
		if _, declared := closure[dependency.ID]; !declared {
			continue
		}
		if dependency.Kind != artifacts.ImplementationWorkKindFile {
			continue
		}
		file, fileSize, err := readImplementationContextFile(root, dependency.Path)
		if err != nil {
			return implementationWorkContext{}, err
		}
		if !file.Exists {
			return implementationWorkContext{}, fmt.Errorf("dependency %q for work item %q has no file %q", dependency.ID, item.ID, dependency.Path)
		}
		total += fileSize
		if total > maxImplementationContextBytes {
			return implementationWorkContext{}, fmt.Errorf("minimal context for work item %q requires %d bytes, exceeding the %d-byte hard limit; split the work contract", item.ID, total, maxImplementationContextBytes)
		}
		context.Dependencies = append(context.Dependencies, file)
		context.DependencyContracts = append(context.DependencyContracts, implementationDependencyContract{
			ID: dependency.ID, Path: dependency.Path, Responsibility: dependency.Responsibility,
			AcceptanceCriteria: cleanOrderedStrings(dependency.AcceptanceCriteria),
		})
	}
	for _, consumer := range ledger.Items {
		if consumer.Kind != artifacts.ImplementationWorkKindFile || consumer.ID == item.ID {
			continue
		}
		consumerClosure := implementationDependencyClosure(consumer, ledger, byID)
		if _, consumesTarget := consumerClosure[item.ID]; !consumesTarget {
			continue
		}
		context.ConsumerContracts = append(context.ConsumerContracts, implementationDependencyContract{
			ID: consumer.ID, Path: consumer.Path, Responsibility: consumer.Responsibility,
			AcceptanceCriteria: cleanOrderedStrings(consumer.AcceptanceCriteria),
		})
	}
	encoded, err := json.Marshal(context)
	if err != nil {
		return implementationWorkContext{}, fmt.Errorf("measure minimal implementation context for work item %q: %w", item.ID, err)
	}
	if len(encoded) > maxImplementationContextBytes {
		return implementationWorkContext{}, fmt.Errorf("minimal context for work item %q requires %d bytes, exceeding the %d-byte hard limit; split the work contract", item.ID, len(encoded), maxImplementationContextBytes)
	}
	return context, nil
}

func readImplementationContextFile(root, path string) (implementationContextFile, int, error) {
	target, err := resolveV3WorkspaceFile(root, path)
	if err != nil {
		return implementationContextFile{}, 0, err
	}
	content, err := os.ReadFile(target)
	if os.IsNotExist(err) {
		return implementationContextFile{Path: filepath.ToSlash(filepath.Clean(path)), Exists: false}, 0, nil
	}
	if err != nil {
		return implementationContextFile{}, 0, fmt.Errorf("read implementation context file %s: %w", path, err)
	}
	if len(content) > maxImplementationContextBytes {
		return implementationContextFile{}, 0, fmt.Errorf("implementation context file %q is %d bytes, exceeding the %d-byte hard limit; split the work contract", path, len(content), maxImplementationContextBytes)
	}
	return implementationContextFile{Path: filepath.ToSlash(filepath.Clean(path)), Exists: true, Content: string(content)}, len(content), nil
}

func buildImplementationWriterPrompt(item artifacts.ImplementationWorkItem, context implementationWorkContext, feedback string) (string, error) {
	itemJSON, err := json.MarshalIndent(struct {
		ID                 string   `json:"id"`
		Discipline         string   `json:"discipline"`
		Path               string   `json:"path"`
		Responsibility     string   `json:"responsibility"`
		AcceptanceCriteria []string `json:"acceptance_criteria"`
	}{item.ID, item.Discipline, item.Path, item.Responsibility, item.AcceptanceCriteria}, "", "  ")
	if err != nil {
		return "", fmt.Errorf("marshal implementation work item: %w", err)
	}
	contextJSON, err := json.MarshalIndent(context, "", "  ")
	if err != nil {
		return "", fmt.Errorf("marshal implementation work context: %w", err)
	}
	feedback = strings.TrimSpace(feedback)
	if feedback == "" {
		feedback = "No prior failure. Implement the declared file contract."
	}
	sections := []string{
		"You are a file worker. Complete exactly one file contract.",
		"The work item is your entire authority. Do not redesign the application, create another file, give advice, or infer work from historical memory.",
		"Use only the target file, declared dependency files, and direct feedback below. Return one complete UTF-8 file, never a diff or Markdown fence.",
		"Do not leave TODO, FIXME, placeholder, stub, unimplemented, or panic-not-implemented bodies.",
		"The server verified every declared dependency before dispatch. Produce a complete write, or return satisfied only when the existing target already fulfills the contract. Refusal and unsupported missing-dependency claims are invalid.",
		"Consumer contracts state the downstream behavior this file must support. They are read-only integration requirements, not authority to edit consumer paths or add speculative APIs.",
	}
	if facts := implementationWriterDisciplineFacts(item, context); facts != "" {
		sections = append(sections, facts)
	}
	sections = append(sections,
		implementationWriterResponseContract,
		"WORK_ITEM:\n"+string(itemJSON),
		"MINIMAL_FILE_CONTEXT:\n"+string(contextJSON),
		"DIRECT_FEEDBACK:\n"+trimForBudget(feedback, 6000),
		implementationControlCommand,
	)
	return strings.Join(sections, "\n\n"), nil
}

func implementationWriterDisciplineFacts(item artifacts.ImplementationWorkItem, context implementationWorkContext) string {
	if filepath.ToSlash(filepath.Clean(item.Path)) != "go.mod" {
		return ""
	}
	for _, constraint := range context.GlobalConstraints {
		if !isImplementationGlobalConstraint(constraint) {
			continue
		}
		return strings.Join([]string{
			"BOOTSTRAP_FACTS:",
			"- No external dependencies are missing. A standard-library-only Go module needs no require directives.",
			"- Choose a concrete non-placeholder local module path, such as omnidex.local/app; never use your-module, yourusername, or example placeholders.",
			"- Declare the module and Go language version as a complete go.mod file ending with a newline.",
		}, "\n")
	}
	return ""
}

const implementationWriterResponseContract = `Return exactly one JSON object with this shape:
{
  "role_id": "file_worker",
  "work_item_id": "<exact work item id>",
  "status": "write|satisfied",
  "path": "<exact assigned path>",
  "content": "<complete file content for write; empty for satisfied>"
}`
