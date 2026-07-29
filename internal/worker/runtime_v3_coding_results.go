package worker

import (
	"fmt"
	"sort"
	"strings"

	"github.com/gryph/omnidex/internal/artifacts"
	"github.com/gryph/omnidex/internal/evidence"
)

func buildDeterministicV3CodingAnalysis(
	intent artifacts.IntentArtifact,
	results []artifacts.SubtaskResultArtifact,
) (artifacts.AnalysisArtifact, bool, error) {
	result, handled, err := deterministicV3CodingResult(intent, results)
	if err != nil || !handled {
		return artifacts.AnalysisArtifact{}, handled, err
	}
	return artifacts.AnalysisArtifact{
		Summary: result.Summary, Blockers: []string{}, Assumptions: []string{},
	}, true, nil
}

func buildDeterministicV3CodingResponse(
	intent artifacts.IntentArtifact,
	analysis artifacts.AnalysisArtifact,
	results []artifacts.SubtaskResultArtifact,
) (artifacts.ResponseDraftArtifact, bool, error) {
	result, handled, err := deterministicV3CodingResult(intent, results)
	if err != nil || !handled {
		return artifacts.ResponseDraftArtifact{}, handled, err
	}
	if strings.TrimSpace(analysis.Summary) != strings.TrimSpace(result.Summary) {
		return artifacts.ResponseDraftArtifact{}, true, fmt.Errorf("deterministic coding analysis differs from the accepted coding result")
	}
	return artifacts.ResponseDraftArtifact{Response: strings.TrimSpace(result.Summary)}, true, nil
}

func buildDeterministicV3CodingVerification(
	intent artifacts.IntentArtifact,
	records []evidence.Record,
) (artifacts.VerificationArtifact, bool, error) {
	if _, handled := buildV3CodingCoordinatorPlan(intent); !handled {
		return artifacts.VerificationArtifact{}, false, nil
	}
	violations := make([]string, 0, 4)
	if !hasV3ExecutionEvidence(records) {
		violations = append(violations, "execution evidence is missing")
	}
	if !hasSuccessfulV3GeneratedDiff(records) {
		violations = append(violations, "successful generated-diff evidence is missing")
	}
	if !hasSuccessfulV3CommandEvidence(records) {
		violations = append(violations, "successful command/test evidence is missing")
	}
	if failed := unresolvedV3CommandFailures(records); len(failed) > 0 {
		violations = append(violations, "latest verification commands failed: "+strings.Join(failed, ", "))
	}
	if len(violations) > 0 {
		sort.Strings(violations)
		return artifacts.VerificationArtifact{}, true, fmt.Errorf(
			"deterministic coding verification failed: %s", strings.Join(violations, "; "),
		)
	}

	evidenceIDs := make([]int64, 0, len(records))
	for _, record := range independentV3EvidenceRecords(records) {
		if record.ID > 0 {
			evidenceIDs = append(evidenceIDs, record.ID)
		}
	}
	if len(evidenceIDs) == 0 {
		return artifacts.VerificationArtifact{}, true, fmt.Errorf("deterministic coding verification has no persisted independent evidence ids")
	}

	coverage := make([]artifacts.ObjectiveCoverage, 0, len(intent.Objectives))
	supported := make([]string, 0, len(intent.Objectives))
	for _, objective := range intent.Objectives {
		coverage = append(coverage, artifacts.ObjectiveCoverage{
			ObjectiveID: strings.TrimSpace(objective.ID),
			Satisfied:   true,
			EvidenceIDs: append([]int64(nil), evidenceIDs...),
			Gaps:        []string{},
		})
		supported = append(supported, strings.TrimSpace(objective.Description))
	}
	artifact := artifacts.VerificationArtifact{
		Verdict:              artifacts.VerificationVerdictPass,
		IndependentChallenge: true,
		SupportedClaims:      cleanOrderedStrings(supported),
		UnsupportedClaims:    []string{},
		MissingEvidence:      []string{},
		Contradictions:       []string{},
		RoleViolations:       []string{},
		ObjectiveCoverage:    coverage,
		RecommendedActions:   []string{},
	}
	if err := validateV3VerificationArtifact(intent, artifact, records); err != nil {
		return artifacts.VerificationArtifact{}, true, err
	}
	return artifact, true, nil
}

func deterministicV3CodingResult(
	intent artifacts.IntentArtifact,
	results []artifacts.SubtaskResultArtifact,
) (artifacts.SubtaskResultArtifact, bool, error) {
	plan, handled := buildV3CodingCoordinatorPlan(intent)
	if !handled {
		return artifacts.SubtaskResultArtifact{}, false, nil
	}
	if len(plan.Subtasks) != 1 {
		return artifacts.SubtaskResultArtifact{}, true, fmt.Errorf("deterministic coding plan requires exactly one coordinator subtask")
	}
	if len(results) != 1 {
		return artifacts.SubtaskResultArtifact{}, true, fmt.Errorf("deterministic coding result requires exactly one subtask result; received %d", len(results))
	}
	expected := plan.Subtasks[0]
	result := results[0]
	if err := validateV3SubtaskResult(result); err != nil {
		return artifacts.SubtaskResultArtifact{}, true, err
	}
	violations := make([]string, 0, 6)
	if result.SubtaskID != expected.ID {
		violations = append(violations, "subtask id differs from the deterministic plan")
	}
	if result.Kind != expected.Kind || result.RoleID != expected.RoleID {
		violations = append(violations, "subtask kind or role differs from the deterministic plan")
	}
	if result.ObjectiveID != expected.ObjectiveID || result.Objective != expected.Objective {
		violations = append(violations, "subtask objective differs from the deterministic plan")
	}
	if result.Priority != expected.Priority {
		violations = append(violations, "subtask priority differs from the deterministic plan")
	}
	if len(differenceStrings(result.RequiredCapabilities, expected.RequiredCapabilities)) > 0 ||
		len(differenceStrings(expected.RequiredCapabilities, result.RequiredCapabilities)) > 0 {
		violations = append(violations, "subtask capabilities differ from the deterministic plan")
	}
	if len(violations) > 0 {
		sort.Strings(violations)
		return artifacts.SubtaskResultArtifact{}, true, fmt.Errorf("deterministic coding result rejected: %s", strings.Join(violations, "; "))
	}
	return result, true, nil
}
