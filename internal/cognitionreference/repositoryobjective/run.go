package repositoryobjective

import (
	"context"
	"errors"
	"fmt"

	"github.com/gryph/omnidex/internal/cognitionreference"
	golangadapter "github.com/gryph/omnidex/internal/repository/adapters/golang"
)

func Run(
	ctx context.Context,
	objective Objective,
	selector cognitionreference.Selector,
) (result Result, resultErr error) {
	result = Result{
		Satisfied: []AcceptancePredicate{}, Steps: []Step{},
		DirectCalls: []SymbolEvidence{}, DirectCallers: []SymbolEvidence{},
		ApplicableTests: []SymbolEvidence{},
	}
	if ctx == nil {
		return result, fmt.Errorf("%w: context is required", ErrInvalidObjective)
	}
	if err := ctx.Err(); err != nil {
		return result, err
	}
	objective.Acceptance = cloneAcceptance(objective.Acceptance)
	if err := objective.validate(); err != nil {
		return result, err
	}
	result.ObjectiveID = objective.ID
	result.Acceptance = cloneAcceptance(objective.Acceptance)
	authority, err := captureProjectedAuthority(ctx, objective.Root)
	if err != nil {
		return result, err
	}
	defer func() {
		if cleanupErr := authority.workspace.Cleanup(); cleanupErr != nil {
			result.Complete = false
			resultErr = errors.Join(resultErr, cleanupErr)
		}
	}()
	result.BeforeSnapshotID = authority.authoritative.ID
	result.Steps = append(result.Steps, StepSnapshotCaptured, StepSnapshotProjected)

	analysis, err := golangadapter.Analyze(ctx, authority.projected)
	if err != nil {
		return result, fmt.Errorf("analyze projected repository objective: %w", err)
	}
	if err := analysis.Validate(authority.projected); err != nil {
		return result, fmt.Errorf("%w: analysis validation: %v", ErrRepositoryAuthority, err)
	}
	if !analysis.Complete {
		return result, fmt.Errorf("%w: Go analysis is incomplete", ErrRepositoryAuthority)
	}
	result.AnalysisID = analysis.ID
	result.Steps = append(result.Steps, StepRepositoryAnalyzed)

	candidates, err := discoverCandidates(ctx, authority.projected, analysis, objective.Subject)
	if err != nil {
		return result, err
	}
	result.Steps = append(result.Steps, StepCandidatesInspected)
	subject, selected, err := resolveSubject(ctx, objective, analysis.ID, candidates, selector, &result)
	if err != nil {
		return result, err
	}
	result.Subject = cloneSubjectFact(subject)
	state := objectiveState{subjectResolved: true, declarationObserved: true}
	result.Steps = append(result.Steps, StepSubjectResolved, StepDeclarationObserved)

	relations, err := inspectDirectRelations(ctx, authority.projected, analysis, selected.symbol.ID)
	if err != nil {
		return result, err
	}
	result.DirectCalls = cloneSymbolEvidence(relations.calls)
	result.DirectCallers = cloneSymbolEvidence(relations.callers)
	result.ApplicableTests = cloneSymbolEvidence(relations.tests)
	state.directRelationsKnown = true
	state.applicableTestsKnown = true
	result.Steps = append(result.Steps, StepRelationsTraversed, StepTestsTraversed)

	after, err := authority.verify(ctx)
	if err != nil {
		return result, err
	}
	result.AfterSnapshotID = after.ID
	result.Steps = append(result.Steps, StepProjectionVerified, StepAuthorityReconciled)
	satisfied, err := evaluateCompletion(objective, state, subject, selected, analysis.ID)
	if err != nil {
		return result, err
	}
	result.Satisfied = cloneAcceptance(satisfied)
	result.Steps = append(result.Steps, StepObjectiveCompleted)
	result.Complete = true
	return result, nil
}

func cloneSubjectFact(value SubjectFact) SubjectFact {
	value.Acceptance = cloneAcceptance(value.Acceptance)
	return value
}

func cloneSymbolEvidence(values []SymbolEvidence) []SymbolEvidence {
	return append([]SymbolEvidence{}, values...)
}
