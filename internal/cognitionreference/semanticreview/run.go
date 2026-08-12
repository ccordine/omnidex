package semanticreview

import (
	"context"
	"fmt"

	"github.com/gryph/omnidex/internal/cognitionreference"
)

func (machine Machine) Run(ctx context.Context) (Result, error) {
	result := machine.initialResult()
	if ctx == nil {
		return result, fmt.Errorf("%w: context is required", ErrInvalidMachine)
	}
	if err := ctx.Err(); err != nil {
		return result, err
	}
	current := cloneArtifact(machine.initial)
	result.VerificationCalls++
	receipt, err := verifyCurrent(ctx, machine.verifier, machine.objective, current, nil)
	if err != nil {
		return result, err
	}
	result.VerificationReceipts = append(result.VerificationReceipts, receipt)
	parent := machine.objective.ID
	for round := 1; round <= machine.limits.MaxReviewRounds; round++ {
		review, gap, err := buildReview(
			machine.objective, machine.specification, current, parent, round,
		)
		if err != nil {
			return result, err
		}
		result.Reviews = append(result.Reviews, review)
		if err := ctx.Err(); err != nil {
			result.failLatestReview()
			return result, err
		}
		result.StationCalls++
		selected, err := cognitionreference.SelectCandidate(ctx, machine.selector, gap)
		if err != nil {
			result.failLatestReview()
			return result, err
		}
		finding, err := materializeFinding(
			machine.objective, machine.specification, review, current, selected,
		)
		if err != nil {
			result.failLatestReview()
			return result, err
		}
		result.Reviews[len(result.Reviews)-1].Status = ObjectiveComplete
		result.Findings = append(result.Findings, finding)
		if finding.Kind == FindingNone {
			if err := ctx.Err(); err != nil {
				return result, err
			}
			if err := completeResult(&result, machine.specification, machine.rules); err != nil {
				return result, err
			}
			return result, nil
		}
		rule, err := machine.rules.rule(finding.FindingCode)
		if err != nil {
			return result, err
		}
		correction, err := deriveCorrectionObjective(
			machine.objective, review, finding, current, rule,
		)
		if err != nil {
			return result, err
		}
		if round == machine.limits.MaxReviewRounds {
			correction.Status = ObjectiveBoundBlocked
			result.Corrections = append(result.Corrections, correction)
			return result, fmt.Errorf(
				"%w: issue %q at review round %d cannot receive a fresh review",
				ErrReviewRoundBound, finding.FindingCode, round,
			)
		}
		result.Corrections = append(result.Corrections, correction)
		executor, err := machine.executors.executor(correction.ObjectiveKind)
		if err != nil {
			result.failLatestCorrection()
			return result, err
		}
		if err := ctx.Err(); err != nil {
			result.failLatestCorrection()
			return result, err
		}
		result.CorrectionCalls++
		value, executeErr := executor.Execute(
			ctx, cloneCorrectionObjective(correction), cloneArtifact(current),
		)
		if err := ctx.Err(); err != nil {
			result.failLatestCorrection()
			return result, err
		}
		if executeErr != nil {
			result.failLatestCorrection()
			return result, fmt.Errorf("%w: %v", ErrCorrection, executeErr)
		}
		candidate, err := newCorrectionArtifact(current, value)
		if err != nil {
			result.failLatestCorrection()
			return result, err
		}
		result.VerificationCalls++
		receipt, err = verifyCurrent(
			ctx, machine.verifier, machine.objective, candidate, &correction,
		)
		if err != nil {
			result.failLatestCorrection()
			return result, err
		}
		correction.OutputArtifactID = candidate.ID
		correction.Status = ObjectiveComplete
		result.Corrections[len(result.Corrections)-1] = cloneCorrectionObjective(correction)
		result.VerificationReceipts = append(result.VerificationReceipts, receipt)
		current = candidate
		result.CurrentArtifact = cloneArtifact(current)
		parent = correction.ID
	}
	return result, ErrReviewRoundBound
}

func (machine Machine) initialResult() Result {
	return Result{
		EvidenceClass:   EvidencePrimitiveContaminatedNonAutonomy,
		Objective:       cloneObjective(machine.objective),
		InitialArtifact: cloneArtifact(machine.initial),
		CurrentArtifact: cloneArtifact(machine.initial),
		Reviews:         []ReviewObjective{}, Findings: []ReviewFinding{},
		Corrections: []CorrectionObjective{}, VerificationReceipts: []VerificationReceipt{},
	}
}

func (result *Result) failLatestReview() {
	if len(result.Reviews) != 0 {
		result.Reviews[len(result.Reviews)-1].Status = ObjectiveFailed
	}
}

func (result *Result) failLatestCorrection() {
	if len(result.Corrections) != 0 {
		result.Corrections[len(result.Corrections)-1].Status = ObjectiveFailed
	}
}
