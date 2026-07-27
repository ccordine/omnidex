package worker

import (
	"fmt"
	"sort"
	"strings"

	"github.com/gryph/omnidex/internal/artifacts"
	"github.com/gryph/omnidex/internal/evidence"
)

type v3VerificationInput struct {
	ObjectiveLedger    []artifacts.Objective `json:"objective_ledger"`
	CompletionCriteria []string              `json:"completion_criteria"`
	DraftResponse      string                `json:"draft_response"`
	Evidence           []v3EvidenceReference `json:"evidence"`
}

type v3EvidenceReference struct {
	ID           int64    `json:"id"`
	Kind         string   `json:"kind"`
	SourceType   string   `json:"source_type,omitempty"`
	SourceRef    string   `json:"source_ref,omitempty"`
	ToolName     string   `json:"tool_name,omitempty"`
	SpecialistID string   `json:"specialist_id,omitempty"`
	SubtaskID    string   `json:"subtask_id,omitempty"`
	ObjectiveID  string   `json:"objective_id,omitempty"`
	Summary      string   `json:"summary,omitempty"`
	Excerpt      string   `json:"excerpt,omitempty"`
	Command      string   `json:"command,omitempty"`
	FilePaths    []string `json:"file_paths,omitempty"`
	Warnings     []string `json:"warnings,omitempty"`
}

func buildV3VerificationInput(intent artifacts.IntentArtifact, draft string, records []evidence.Record) v3VerificationInput {
	independentRecords := independentV3EvidenceRecords(records)
	references := make([]v3EvidenceReference, 0, len(independentRecords))
	for _, record := range independentRecords {
		references = append(references, v3EvidenceReference{
			ID:           record.ID,
			Kind:         strings.TrimSpace(record.Kind),
			SourceType:   strings.TrimSpace(record.SourceType),
			SourceRef:    strings.TrimSpace(record.SourceRef),
			ToolName:     strings.TrimSpace(record.ToolName),
			SpecialistID: metadataText(record.Metadata, "specialist_id"),
			SubtaskID:    metadataText(record.Metadata, "subtask_id"),
			ObjectiveID:  metadataText(record.Metadata, "subtask_objective_id"),
			Summary:      trimForBudget(record.Summary, 600),
			Excerpt:      trimForBudget(record.Excerpt, 1000),
			Command:      trimForBudget(record.Command, 500),
			FilePaths:    cleanOrderedStrings(record.FilePaths),
			Warnings:     cleanOrderedStrings(record.Warnings),
		})
	}
	return v3VerificationInput{
		ObjectiveLedger:    append([]artifacts.Objective(nil), intent.Objectives...),
		CompletionCriteria: cleanOrderedStrings(intent.CompletionCriteria),
		DraftResponse:      strings.TrimSpace(draft),
		Evidence:           references,
	}
}

func independentV3EvidenceRecords(records []evidence.Record) []evidence.Record {
	out := make([]evidence.Record, 0, len(records))
	for _, record := range records {
		if record.Kind == evidence.KindMemoryExcerpt || record.Kind == evidence.KindModelJudgment {
			continue
		}
		out = append(out, record)
	}
	return out
}

func metadataText(metadata map[string]any, key string) string {
	value, ok := metadata[strings.TrimSpace(key)]
	if !ok {
		return ""
	}
	text, ok := value.(string)
	if !ok {
		return ""
	}
	return strings.TrimSpace(text)
}

func validateV3Finalization(intent artifacts.IntentArtifact, verification artifacts.VerificationArtifact, records []evidence.Record, draft string) error {
	violations := make([]string, 0, 8)
	if strings.TrimSpace(draft) == "" {
		violations = append(violations, "response draft is empty")
	}
	if verification.Verdict != artifacts.VerificationVerdictPass {
		violations = append(violations, fmt.Sprintf("verification verdict is %q", verification.Verdict))
	}
	if !verification.IndependentChallenge {
		violations = append(violations, "independent challenge is required")
	}
	if verification.AdviceOnly {
		violations = append(violations, "verifier classified the response as advice-only")
	}
	if len(verification.UnsupportedClaims) > 0 {
		violations = append(violations, "unsupported claims remain")
	}
	if len(verification.MissingEvidence) > 0 {
		violations = append(violations, "required evidence is missing")
	}
	if len(verification.Contradictions) > 0 {
		violations = append(violations, "contradictions remain")
	}
	if len(verification.RoleViolations) > 0 {
		violations = append(violations, "specialist role violations remain")
	}
	coverage := map[string]bool{}
	for _, item := range verification.ObjectiveCoverage {
		coverage[strings.TrimSpace(item.ObjectiveID)] = item.Satisfied
	}
	for _, objective := range intent.Objectives {
		if !coverage[strings.TrimSpace(objective.ID)] {
			violations = append(violations, fmt.Sprintf("objective %q is not verified", objective.ID))
		}
	}
	if intent.RequiresAction && !hasV3ExecutionEvidence(records) {
		violations = append(violations, "action completion has no execution evidence")
	}
	if intentRequiresV3Capability(intent, capabilityWorkspaceWrite) && !hasSuccessfulV3GeneratedDiff(records) {
		violations = append(violations, "workspace.write has no successful generated-diff evidence")
	}
	if intentRequiresV3Capability(intent, capabilityCommandExecute) && !hasSuccessfulV3CommandEvidence(records) {
		violations = append(violations, "command.execute has no successful command evidence")
	}
	if failed := unresolvedV3CommandFailures(records); len(failed) > 0 {
		violations = append(violations, "latest verification commands failed: "+strings.Join(failed, ", "))
	}
	if len(violations) > 0 {
		sort.Strings(violations)
		return fmt.Errorf("v3 finalization rejected: %s", strings.Join(violations, "; "))
	}
	return nil
}

func validateV3VerificationArtifact(intent artifacts.IntentArtifact, verification artifacts.VerificationArtifact, records []evidence.Record) error {
	violations := make([]string, 0, 8)
	if !verification.IndependentChallenge {
		violations = append(violations, "independent_challenge must be true")
	}
	switch verification.Verdict {
	case artifacts.VerificationVerdictPass, artifacts.VerificationVerdictRevise, artifacts.VerificationVerdictBlocked:
	default:
		violations = append(violations, fmt.Sprintf("verdict %q is invalid", verification.Verdict))
	}
	objectiveIDs := map[string]struct{}{}
	for _, objective := range intent.Objectives {
		objectiveIDs[strings.TrimSpace(objective.ID)] = struct{}{}
	}
	seenCoverage := map[string]struct{}{}
	evidenceIDs := map[int64]struct{}{}
	for _, record := range records {
		if record.Kind == evidence.KindMemoryExcerpt || record.Kind == evidence.KindModelJudgment {
			continue
		}
		evidenceIDs[record.ID] = struct{}{}
	}
	for index, coverage := range verification.ObjectiveCoverage {
		objectiveID := strings.TrimSpace(coverage.ObjectiveID)
		if _, ok := objectiveIDs[objectiveID]; !ok {
			violations = append(violations, fmt.Sprintf("objective_coverage[%d] references unknown objective %q", index, objectiveID))
		}
		if _, duplicate := seenCoverage[objectiveID]; duplicate {
			violations = append(violations, fmt.Sprintf("objective_coverage duplicates objective %q", objectiveID))
		}
		seenCoverage[objectiveID] = struct{}{}
		for _, evidenceID := range coverage.EvidenceIDs {
			if _, ok := evidenceIDs[evidenceID]; !ok {
				violations = append(violations, fmt.Sprintf("objective %q cites unavailable independent evidence id %d", objectiveID, evidenceID))
			}
		}
		if coverage.Satisfied && len(coverage.EvidenceIDs) == 0 {
			violations = append(violations, fmt.Sprintf("objective %q is marked satisfied without independent evidence", objectiveID))
		}
	}
	for objectiveID := range objectiveIDs {
		if _, ok := seenCoverage[objectiveID]; !ok {
			violations = append(violations, fmt.Sprintf("objective %q is absent from objective_coverage", objectiveID))
		}
	}
	if verification.Verdict == artifacts.VerificationVerdictPass && verificationHasBlockingFindings(verification) {
		violations = append(violations, "pass verdict conflicts with blocking verification findings")
	}
	if intent.RequiresAction && verification.Verdict == artifacts.VerificationVerdictPass && !hasV3ExecutionEvidence(records) {
		violations = append(violations, "pass verdict lacks action execution evidence")
	}
	if len(violations) > 0 {
		sort.Strings(violations)
		return fmt.Errorf("v3 verification artifact rejected: %s", strings.Join(violations, "; "))
	}
	return nil
}

func verificationHasBlockingFindings(verification artifacts.VerificationArtifact) bool {
	if verification.AdviceOnly || len(verification.UnsupportedClaims) > 0 || len(verification.MissingEvidence) > 0 || len(verification.Contradictions) > 0 || len(verification.RoleViolations) > 0 {
		return true
	}
	for _, coverage := range verification.ObjectiveCoverage {
		if !coverage.Satisfied || len(coverage.Gaps) > 0 {
			return true
		}
	}
	return false
}

func hasV3ExecutionEvidence(records []evidence.Record) bool {
	for _, record := range records {
		switch record.Kind {
		case evidence.KindGeneratedDiff:
			if metadataFlag(record.Metadata, "succeeded") {
				return true
			}
		case evidence.KindCommandOutput:
			if metadataFlag(record.Metadata, "succeeded") && (metadataFlag(record.Metadata, "mutation") || metadataFlag(record.Metadata, "side_effect")) {
				return true
			}
		case evidence.KindTestResult:
			if metadataFlag(record.Metadata, "succeeded") && metadataFlag(record.Metadata, "side_effect") {
				return true
			}
		}
	}
	return false
}

func intentRequiresV3Capability(intent artifacts.IntentArtifact, capability string) bool {
	if containsString(intent.RequiredCapabilities, capability) {
		return true
	}
	for _, objective := range intent.Objectives {
		if containsString(objective.RequiredCapabilities, capability) {
			return true
		}
	}
	return false
}

func hasSuccessfulV3GeneratedDiff(records []evidence.Record) bool {
	for _, record := range records {
		if record.Kind == evidence.KindGeneratedDiff && metadataFlag(record.Metadata, "succeeded") {
			return true
		}
	}
	return false
}

func hasSuccessfulV3CommandEvidence(records []evidence.Record) bool {
	for _, record := range records {
		if (record.Kind == evidence.KindCommandOutput || record.Kind == evidence.KindTestResult) && metadataFlag(record.Metadata, "succeeded") {
			return true
		}
	}
	return false
}

func unresolvedV3CommandFailures(records []evidence.Record) []string {
	latest := map[string]evidence.Record{}
	for _, record := range records {
		if record.Kind != evidence.KindCommandOutput && record.Kind != evidence.KindTestResult {
			continue
		}
		command := strings.TrimSpace(record.Command)
		if command == "" {
			command = strings.TrimSpace(record.SourceRef)
		}
		if command == "" {
			continue
		}
		latest[command] = record
	}
	failed := make([]string, 0, len(latest))
	for command, record := range latest {
		if !metadataFlag(record.Metadata, "succeeded") {
			failed = append(failed, command)
		}
	}
	sort.Strings(failed)
	return failed
}

func metadataFlag(metadata map[string]any, key string) bool {
	value, ok := metadata[key]
	return ok && value == true
}
