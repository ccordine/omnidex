package omni

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

var musicStudioDomainSignals = []string{
	"studio-shell",
	"channel-rack",
	"piano-roll",
	"beat studio",
	"omnidex beat studio",
	"pattern step sequencer",
	"sample/instrument pads",
	"music production studio",
	"fruity loops",
}

var noteAppDomainSignals = []string{
	"note manager",
	"note list",
	"notes app",
	"add note",
	"delete note",
	"note title",
	"note body",
}

func promptRequestsMusicStudio(prompt, toolTask string) bool {
	text := strings.ToLower(strings.TrimSpace(prompt + "\n" + toolTask))
	if strings.Contains(text, "music production") || strings.Contains(text, "beat studio") || strings.Contains(text, "step sequencer") {
		return true
	}
	if strings.Contains(text, "music") && (strings.Contains(text, "studio") || strings.Contains(text, "mixer") || strings.Contains(text, "sequencer")) {
		return true
	}
	for _, criterion := range explicitReactAppAcceptanceCriteria(prompt, toolTask) {
		lower := strings.ToLower(criterion)
		if strings.Contains(lower, "music") || strings.Contains(lower, "sequencer") || strings.Contains(lower, "mixer") || strings.Contains(lower, "channel rack") || strings.Contains(lower, "piano roll") {
			return true
		}
	}
	return false
}

func promptRequestsNoteApp(prompt, toolTask string) bool {
	text := strings.ToLower(strings.TrimSpace(prompt + "\n" + toolTask))
	return strings.Contains(text, "note app") ||
		strings.Contains(text, "notes app") ||
		strings.Contains(text, "note-taking") ||
		strings.Contains(text, "note taking") ||
		(strings.Contains(text, "note") && (strings.Contains(text, "crud") || strings.Contains(text, "todo") || strings.Contains(text, "journal")))
}

func contentContainsAnySignal(content string, signals []string) bool {
	lower := strings.ToLower(content)
	for _, signal := range signals {
		if strings.Contains(lower, strings.ToLower(signal)) {
			return true
		}
	}
	return false
}

func validateArchitectContentAlignsWithPrompt(content string, item ArchitectWorkItem, prompt string, contract ImplementationArchitectContract) error {
	trimmed := strings.TrimSpace(content)
	if trimmed == "" {
		return fmt.Errorf("alignment_validator rejected: generated content is empty")
	}
	toolTask := strings.Join(contract.Guardrails, " ")
	wantsMusic := promptRequestsMusicStudio(prompt, toolTask) || len(musicStudioAcceptanceCriteria(contract.AcceptanceCriteria)) > 0
	wantsNotes := promptRequestsNoteApp(prompt, toolTask) || len(noteAppAcceptanceCriteria(contract.AcceptanceCriteria)) > 0
	hasMusic := contentContainsAnySignal(trimmed, musicStudioDomainSignals)
	hasNotes := contentContainsAnySignal(trimmed, noteAppDomainSignals)

	switch {
	case wantsNotes && hasMusic && !wantsMusic:
		return fmt.Errorf("alignment_validator rejected: content implements a music studio app but the active prompt requests a notes app")
	case wantsMusic && hasNotes && !wantsNotes:
		return fmt.Errorf("alignment_validator rejected: content implements a notes app but the active prompt requests a music production app")
	}
	path := filepath.ToSlash(strings.ToLower(strings.TrimSpace(item.Path)))
	if architectAlignmentChecksAcceptanceCriteria(path) && len(contract.AcceptanceCriteria) > 0 {
		if missing := missingAcceptanceSignals(trimmed, contract.AcceptanceCriteria); len(missing) > 0 {
			return fmt.Errorf("alignment_validator rejected: content missing requested acceptance signal(s): %s", strings.Join(missing, ", "))
		}
	}
	if architectAlignmentChecksForeignDomain(path) {
		if foreign := foreignDomainSignalsForPrompt(prompt, toolTask, contract.AcceptanceCriteria); len(foreign) > 0 {
			if contentContainsAnySignal(trimmed, foreign) {
				return fmt.Errorf("alignment_validator rejected: content contains foreign-domain signals from a prior app pattern: %s", strings.Join(foreign, ", "))
			}
		}
	}
	return nil
}

func architectAlignmentChecksAcceptanceCriteria(path string) bool {
	switch path {
	case "src/app.js", "src/app.jsx", "src/app.css", "scripts/smoke-test.mjs":
		return true
	default:
		return false
	}
}

func architectAlignmentChecksForeignDomain(path string) bool {
	switch path {
	case "src/app.js", "src/app.jsx", "src/app.css", "scripts/smoke-test.mjs":
		return true
	default:
		return false
	}
}

func musicStudioAcceptanceCriteria(criteria []string) []string {
	out := []string{}
	for _, criterion := range criteria {
		lower := strings.ToLower(strings.TrimSpace(criterion))
		if strings.Contains(lower, "music") || strings.Contains(lower, "sequencer") || strings.Contains(lower, "mixer") || strings.Contains(lower, "channel") || strings.Contains(lower, "piano") || strings.Contains(lower, "studio") {
			out = append(out, criterion)
		}
	}
	return out
}

func noteAppAcceptanceCriteria(criteria []string) []string {
	out := []string{}
	for _, criterion := range criteria {
		lower := strings.ToLower(strings.TrimSpace(criterion))
		if strings.Contains(lower, "note") || strings.Contains(lower, "crud") || strings.Contains(lower, "todo") {
			out = append(out, criterion)
		}
	}
	return out
}

func foreignDomainSignalsForPrompt(prompt, toolTask string, criteria []string) []string {
	if promptRequestsMusicStudio(prompt, toolTask) {
		return noteAppDomainSignals
	}
	if promptRequestsNoteApp(prompt, toolTask) || len(noteAppAcceptanceCriteria(criteria)) > 0 {
		return musicStudioDomainSignals
	}
	return nil
}

func cssMustIncludeForContract(contract ImplementationArchitectContract) []string {
	signals := acceptanceSignalsForFileContract(contract)
	if len(signals) == 0 {
		return []string{"body", ".app"}
	}
	out := make([]string, 0, len(signals))
	for _, signal := range signals {
		signal = strings.TrimSpace(signal)
		if signal == "" {
			continue
		}
		if strings.HasPrefix(signal, ".") {
			out = append(out, signal)
			continue
		}
		out = append(out, "."+strings.ReplaceAll(signal, " ", "-"))
	}
	return uniqueNonEmptyStrings(out)
}

func architectWorkItemFileEvidenceValid(item ArchitectWorkItem, workingDir string, contract ImplementationArchitectContract, prompt string) (string, error) {
	if item.Path == "" || strings.HasSuffix(item.Path, "/") {
		return "", fmt.Errorf("architect work item %q has no concrete file path for evidence validation", item.ID)
	}
	targetPath := filepath.Join(workingDir, item.CWD, item.Path)
	content, err := os.ReadFile(targetPath)
	if err != nil {
		return "", fmt.Errorf("architect work item %q file evidence missing at %s: %w", item.ID, filepath.ToSlash(targetPath), err)
	}
	trimmed := strings.TrimSpace(string(content))
	if trimmed == "" {
		return "", fmt.Errorf("architect work item %q file evidence is empty at %s", item.ID, filepath.ToSlash(targetPath))
	}
	if err := validateCodeContentProposalForArchitectItem(trimmed, contract, item); err != nil {
		return "", fmt.Errorf("architect work item %q file evidence failed content validation: %w", item.ID, err)
	}
	if err := validateArchitectContentAlignsWithPrompt(trimmed, item, prompt, contract); err != nil {
		return "", err
	}
	return trimmed, nil
}

func architectImplementationBlockedByMissingTestProbe(queue []ArchitectWorkItem, item ArchitectWorkItem, workingDir string, contract ImplementationArchitectContract, prompt string, observations []StructuredCommandObservation) error {
	if architectWorkItemIsTestFirst(item) || item.Operation != "create" && item.Operation != "update" {
		return nil
	}
	path := filepath.ToSlash(strings.ToLower(strings.TrimSpace(item.Path)))
	if path != "src/app.js" && path != "src/app.jsx" && path != "src/app.css" {
		return nil
	}
	var probe ArchitectWorkItem
	for _, candidate := range queue {
		if strings.EqualFold(candidate.Path, "scripts/smoke-test.mjs") {
			probe = candidate
			break
		}
	}
	if probe.ID == "" {
		return nil
	}
	if !architectWorkItemApplyObserved(probe, observations) {
		return fmt.Errorf("test_first gate: implementation work item %q is blocked until acceptance probe %q is written", item.ID, probe.ID)
	}
	if _, err := architectWorkItemFileEvidenceValid(probe, workingDir, contract, prompt); err != nil {
		return fmt.Errorf("test_first gate: implementation work item %q is blocked until acceptance probe %q passes validation: %w", item.ID, probe.ID, err)
	}
	return nil
}

func architectWorkItemApplyObserved(item ArchitectWorkItem, observations []StructuredCommandObservation) bool {
	for _, obs := range observations {
		if obs.ExitCode == 0 && architectApplyObservationMatches(item, obs) {
			return true
		}
	}
	return false
}

func memoryBriefLooksForeignToPrompt(memory SessionMemory, prompt, toolTask string) bool {
	content := strings.ToLower(strings.TrimSpace(memory.Content + " " + strings.Join(memory.Tags, " ")))
	if content == "" {
		return false
	}
	if promptRequestsNoteApp(prompt, toolTask) {
		return contentContainsAnySignal(content, musicStudioDomainSignals)
	}
	if promptRequestsMusicStudio(prompt, toolTask) {
		return contentContainsAnySignal(content, noteAppDomainSignals)
	}
	return false
}
