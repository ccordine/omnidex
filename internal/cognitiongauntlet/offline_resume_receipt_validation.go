package cognitiongauntlet

import (
	"fmt"
	"reflect"

	"github.com/gryph/omnidex/internal/model"
)

func (run OfflineResumeRunReceipt) validate(
	want OfflineResumeSchedule,
	baseline ResumeBaselineArtifact,
) error {
	if !reflect.DeepEqual(run.Schedule, want) || !validDigest(run.ScheduleEvidenceSHA256) ||
		!validDigest(run.PromotionReceiptSHA256) ||
		!validDigest(run.PublicRunAuthoritySHA256) || !validDigest(run.EpisodeSealSHA256) ||
		!validDigest(run.EvaluationArtifactSHA256) || run.Semantics.Validate() != nil ||
		run.Interruptions == nil || run.InferenceStartedAt.IsZero() ||
		run.InferenceExitedAt.Before(run.InferenceStartedAt) ||
		run.EvaluatorStartedAt.Before(run.InferenceExitedAt) ||
		run.EvaluatorCompletedAt.Before(run.EvaluatorStartedAt) {
		return fmt.Errorf("Resume run authority is invalid")
	}
	semanticsMatch := run.Semantics == baseline.Semantics
	if want.Kind == ResumeLiveInferenceExpiry {
		semanticsMatch = liveResumeSemanticsMatch(run.Semantics, baseline.Semantics)
	}
	if !semanticsMatch || !run.GoalSuccess || !run.ValidTerminalState ||
		!run.CausalAdmissionComplete || !run.CleanDeskQualified ||
		run.Recovery.RestorationMismatches != 0 || run.Recovery.ProjectionMismatches != 0 {
		return fmt.Errorf("Resume run did not preserve competent uninterrupted semantics")
	}
	if err := validateResumeInterruptions(run, baseline); err != nil {
		return err
	}
	return nil
}

func liveResumeSemanticsMatch(
	live ResumeEpisodeSemantics,
	baseline ResumeEpisodeSemantics,
) bool {
	return live.Validate() == nil && baseline.Validate() == nil &&
		live.LogicalProjectionSHA256 == baseline.LogicalProjectionSHA256 &&
		live.LogicalProjectionCount == baseline.LogicalProjectionCount &&
		live.ProjectionCount == baseline.ProjectionCount+1 &&
		live.ActionSequenceSHA256 == baseline.ActionSequenceSHA256 &&
		live.FinalRevision == baseline.FinalRevision && live.Outcome == baseline.Outcome &&
		live.ModelCalls == baseline.ModelCalls && live.ModelDecisions == baseline.ModelDecisions &&
		live.EnvironmentActions == baseline.EnvironmentActions
}

func validateResumeInterruptions(
	run OfflineResumeRunReceipt,
	baseline ResumeBaselineArtifact,
) error {
	wantCount := run.Schedule.RequiredKills
	switch run.Schedule.Kind {
	case ResumeUninterrupted:
		wantCount = 0
	case ResumeEveryDecision:
		wantCount = run.Semantics.ModelDecisions - 1
		if wantCount < 1 {
			return fmt.Errorf("every-decision Resume did not reach a replacement boundary")
		}
	case ResumeLiveInferenceExpiry:
		wantCount = 1
	}
	if len(run.Interruptions) != wantCount || run.Recovery.Restarts != wantCount {
		return fmt.Errorf("Resume interruption count differs from its schedule")
	}
	for index, interruption := range run.Interruptions {
		boundary := interruption.DecisionBoundary
		if run.Schedule.Kind == ResumeLiveInferenceExpiry {
			if boundary != 0 {
				return fmt.Errorf("live-inference Resume must expire on its first in-flight call")
			}
		} else if run.Schedule.Kind == ResumeEveryDecision {
			if boundary != uint32(index+1) {
				return fmt.Errorf("every-decision Resume omitted a decision boundary")
			}
		} else if boundary != run.Schedule.DecisionBoundaries[index] {
			return fmt.Errorf("Resume interruption changed its preregistered boundary")
		}
		checkpoint, err := baseline.checkpoint(boundary)
		if err != nil {
			return err
		}
		if err := interruption.validate(checkpoint); err != nil {
			return err
		}
		if index > 0 && run.Interruptions[index-1].Replacement != interruption.Original {
			return fmt.Errorf("Resume replacement chain changed authority")
		}
	}
	if run.Schedule.Kind == ResumeLiveInferenceExpiry {
		if run.LiveStaleProbe == nil || !validDigest(run.LiveStaleProbeSHA256) ||
			run.LiveStaleProbe.Validate() != nil || !run.LiveStaleProbe.Complete() ||
			len(run.Interruptions) != 1 ||
			run.LiveStaleProbe.PublicRunAuthoritySHA256 != run.PublicRunAuthoritySHA256 ||
			run.Recovery.StaleAttemptRejections < 5 {
			return fmt.Errorf("live-inference Resume lacks all stale-write rejection classes")
		}
	} else if run.LiveStaleProbe != nil || run.LiveStaleProbeSHA256 != "" {
		return fmt.Errorf("ordinary Resume run carries live-inference evidence")
	}
	return nil
}

func (receipt OfflineResumeInterruptionReceipt) validate(
	baseline ResumeBaselineCheckpoint,
) error {
	baselineSHA, err := digestJSON(baseline)
	if err != nil {
		return err
	}
	if receipt.DecisionBoundary != baseline.DecisionBoundary ||
		receipt.BaselineCheckpointSHA256 != baselineSHA ||
		!validTakeoverAttempt(receipt.Original) || !validTakeoverAttempt(receipt.Replacement) ||
		receipt.OriginalPID <= 0 || receipt.ReplacementPID <= 0 ||
		receipt.OriginalPID == receipt.ReplacementPID ||
		receipt.Continuity.Validate() != nil ||
		!sameResumeAttempt(receipt.Original, receipt.Continuity.Before) ||
		!sameResumeAttempt(receipt.Replacement, receipt.Continuity.After) {
		return fmt.Errorf("Resume interruption evidence is invalid")
	}
	if receipt.DecisionBoundary == 0 {
		if !receipt.OriginalDiedAt.IsZero() || receipt.OriginalStoppedAt.IsZero() ||
			receipt.OriginalResumedAt.Before(receipt.OriginalStoppedAt) ||
			receipt.OriginalExitedAt.Before(receipt.OriginalResumedAt) {
			return fmt.Errorf("live Resume interruption chronology is invalid")
		}
		return nil
	}
	if receipt.OriginalDiedAt.IsZero() || !receipt.OriginalStoppedAt.IsZero() ||
		!receipt.OriginalResumedAt.IsZero() || !receipt.OriginalExitedAt.IsZero() ||
		receipt.Continuity.Before.SemanticSHA256 != baseline.PreCall.SemanticSHA256 ||
		receipt.Continuity.Before.ProjectionRenderedSHA256 != baseline.PreCall.ProjectionRenderedSHA256 {
		return fmt.Errorf("killed Resume interruption evidence is invalid")
	}
	return nil
}

func sameResumeAttempt(
	authority model.StepAttemptAuthority,
	checkpoint SemanticPreCallCheckpoint,
) bool {
	bound := checkpoint.Bound.Attempt
	return authority.JobID == bound.JobID && authority.Generation == bound.Generation &&
		authority.StepID == bound.StepID && authority.Attempt == int64(bound.Attempt) &&
		authority.WorkerID == bound.WorkerID
}

func equalOfflineResumeGate(left, right OfflineResumeGate) bool {
	return reflect.DeepEqual(left, right)
}
