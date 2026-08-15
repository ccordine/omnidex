package worker

import (
	"fmt"
	"strings"
)

func exactTypeScriptDiagnosticDelta(
	before *ExactTypeScriptReplayDiagnostic,
	after *ExactTypeScriptReplayDiagnostic,
) (ExactTypeScriptDiagnosticDelta, error) {
	if err := validateExactTypeScriptReplayDiagnostic(before); err != nil {
		return ExactTypeScriptDiagnosticDelta{}, fmt.Errorf("score prior TypeScript diagnostics: %w", err)
	}
	delta := ExactTypeScriptDiagnosticDelta{
		BeforeStage: before.Stage,
		Before:      before.Count,
	}
	if after == nil {
		delta.AfterStage = ExactTypeScriptVerificationCompiled
		delta.Resolved = before.Count
		delta.Assessment = ExactTypeScriptConvergenceCompiledAssessment
		return delta, nil
	}
	if err := validateExactTypeScriptReplayDiagnostic(after); err != nil {
		return ExactTypeScriptDiagnosticDelta{}, fmt.Errorf("score current TypeScript diagnostics: %w", err)
	}
	delta.AfterStage = after.Stage
	delta.After = after.Count

	beforeCounts := exactTypeScriptDiagnosticCounts(before.CompilerDiagnostics)
	afterCounts := exactTypeScriptDiagnosticCounts(after.CompilerDiagnostics)
	for identity, count := range beforeCounts {
		retained := count
		if afterCounts[identity] < retained {
			retained = afterCounts[identity]
		}
		delta.Retained += retained
		delta.Resolved += count - retained
	}
	for identity, count := range afterCounts {
		introduced := count - beforeCounts[identity]
		if introduced > 0 {
			delta.Introduced += introduced
		}
	}

	beforeRank := exactTypeScriptVerificationStageRank(before.Stage)
	afterRank := exactTypeScriptVerificationStageRank(after.Stage)
	switch {
	case afterRank > beforeRank:
		delta.Assessment = ExactTypeScriptConvergenceProgress
	case afterRank < beforeRank:
		delta.Assessment = ExactTypeScriptConvergenceRegression
	case delta.Resolved > delta.Introduced:
		delta.Assessment = ExactTypeScriptConvergenceProgress
	case delta.Introduced > delta.Resolved:
		delta.Assessment = ExactTypeScriptConvergenceRegression
	case delta.Resolved > 0:
		delta.Assessment = ExactTypeScriptConvergenceMixed
	default:
		delta.Assessment = ExactTypeScriptConvergenceUnchanged
	}
	return delta, nil
}

func validateExactTypeScriptReplayDiagnostic(diagnostic *ExactTypeScriptReplayDiagnostic) error {
	if diagnostic == nil {
		return fmt.Errorf("diagnostic is nil")
	}
	if diagnostic.Stage != ExactTypeScriptVerificationSyntax &&
		diagnostic.Stage != ExactTypeScriptVerificationTypecheck {
		return fmt.Errorf("verification stage %q is invalid", diagnostic.Stage)
	}
	if diagnostic.Count < 1 || diagnostic.Count != len(diagnostic.CompilerDiagnostics) {
		return fmt.Errorf(
			"diagnostic count %d differs from exact evidence count %d",
			diagnostic.Count, len(diagnostic.CompilerDiagnostics),
		)
	}
	for index, diagnostic := range diagnostic.CompilerDiagnostics {
		if strings.TrimSpace(diagnostic) == "" {
			return fmt.Errorf("diagnostic %d is empty", index+1)
		}
	}
	return nil
}

func exactTypeScriptDiagnosticCounts(diagnostics []string) map[string]int {
	counts := make(map[string]int, len(diagnostics))
	for _, diagnostic := range diagnostics {
		identity := exactTypeScriptReplayHistoricalDiagnostic(diagnostic)
		counts[identity]++
	}
	return counts
}

func exactTypeScriptVerificationStageRank(stage ExactTypeScriptVerificationStage) int {
	switch stage {
	case ExactTypeScriptVerificationSyntax:
		return 1
	case ExactTypeScriptVerificationTypecheck:
		return 2
	case ExactTypeScriptVerificationCompiled:
		return 3
	default:
		return 0
	}
}
