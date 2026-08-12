package codingobjective

import (
	"context"
	"errors"
	"fmt"

	"github.com/gryph/omnidex/internal/assemblyline"
	repositoryfacts "github.com/gryph/omnidex/internal/repository"
	golangadapter "github.com/gryph/omnidex/internal/repository/adapters/golang"
	"github.com/gryph/omnidex/internal/repository/changeapply"
)

const maxObservedDeclarationBytes = 64 * 1024

type inspectedRepository struct {
	snapshot repositoryfacts.Snapshot
	analysis repositoryfacts.Analysis
	symbol   repositoryfacts.Symbol
	span     repositoryfacts.SourceSpan
	contract repositoryfacts.ChangeContract
	target   repositoryfacts.ChangeTarget
}

func inspectRepository(
	ctx context.Context,
	objective Objective,
	result *Result,
) (_ inspectedRepository, resultErr error) {
	snapshot, err := repositoryfacts.BuildGitSnapshot(
		ctx, objective.Root, repositoryfacts.SnapshotOptions{},
	)
	if err != nil {
		return inspectedRepository{}, fmt.Errorf("snapshot reference repository: %w", err)
	}
	result.BeforeSnapshotID = snapshot.ID
	result.Steps = append(result.Steps, StepRepositorySnapshotted)

	workspace, err := changeapply.NewSnapshotWorkspace(ctx, snapshot)
	if err != nil {
		return inspectedRepository{}, fmt.Errorf("create exact analysis workspace: %w", err)
	}
	defer func() { resultErr = errors.Join(resultErr, workspace.Cleanup()) }()
	evidenceSnapshot := snapshot
	evidenceSnapshot.Root = workspace.Root()
	analysis, err := golangadapter.Analyze(ctx, evidenceSnapshot)
	if err != nil {
		return inspectedRepository{}, fmt.Errorf("analyze reference Go repository: %w", err)
	}
	if !analysis.Complete {
		return inspectedRepository{}, fmt.Errorf("%w: Go analysis is incomplete", ErrRepositoryEvidence)
	}
	result.Steps = append(result.Steps, StepRepositoryAnalyzed)

	symbol, err := resolveExactTarget(analysis, objective.Target)
	if err != nil {
		return inspectedRepository{}, err
	}
	result.Steps = append(result.Steps, StepTargetResolved)
	span, err := repositoryfacts.ReadExactSymbolSpan(evidenceSnapshot, symbol, maxObservedDeclarationBytes)
	if err != nil {
		return inspectedRepository{}, fmt.Errorf("observe exact target declaration: %w", err)
	}
	result.Steps = append(result.Steps, StepTargetObserved)

	contract, err := repositoryfacts.BuildChangeContract(
		evidenceSnapshot,
		analysis,
		[]repositoryfacts.ChangeRequest{{
			SymbolID: symbol.ID, RequirementQuote: objective.RequirementQuote,
		}},
	)
	if err != nil {
		return inspectedRepository{}, fmt.Errorf("build exact reference change contract: %w", err)
	}
	if len(contract.Targets) != 1 || contract.Targets[0].SymbolID != symbol.ID {
		return inspectedRepository{}, fmt.Errorf("%w: change contract does not contain the exact target", ErrRepositoryEvidence)
	}
	target := contract.Targets[0]
	if len(target.VerificationSymbolIDs) == 0 {
		return inspectedRepository{}, fmt.Errorf(
			"%w: target has no direct verification test", ErrRepositoryEvidence,
		)
	}
	result.DirectCapabilities = len(target.DirectCapabilities)
	result.DirectTests = len(target.VerificationSymbolIDs)
	result.Steps = append(result.Steps, StepContractBuilt)
	if err := workspace.VerifyExact(ctx); err != nil {
		return inspectedRepository{}, fmt.Errorf("exact analysis workspace changed: %w", err)
	}
	return inspectedRepository{
		snapshot: snapshot, analysis: analysis, symbol: symbol,
		span: span, contract: contract, target: target,
	}, nil
}

func resolveExactTarget(
	analysis repositoryfacts.Analysis,
	wanted string,
) (repositoryfacts.Symbol, error) {
	exact := make([]repositoryfacts.Symbol, 0, 2)
	for _, symbol := range analysis.Symbols {
		if symbol.QualifiedName == wanted {
			exact = append(exact, symbol)
		}
	}
	switch len(exact) {
	case 0:
		return repositoryfacts.Symbol{}, fmt.Errorf(
			"%w: exact target %q is absent", ErrRepositoryEvidence, wanted,
		)
	case 1:
		if exact[0].Kind != "function" && exact[0].Kind != "method" {
			return repositoryfacts.Symbol{}, fmt.Errorf(
				"%w: exact target %q has unsupported kind %q",
				ErrRepositoryEvidence, wanted, exact[0].Kind,
			)
		}
		return exact[0], nil
	default:
		return repositoryfacts.Symbol{}, fmt.Errorf(
			"%w: exact target %q is ambiguous across %d declarations",
			ErrRepositoryEvidence, wanted, len(exact),
		)
	}
}

func fragmentInput(repository inspectedRepository) assemblyline.FragmentModificationInput {
	capabilities := make([]string, len(repository.target.DirectCapabilities))
	permitted := make([]string, len(repository.target.DirectCapabilities))
	for index, capability := range repository.target.DirectCapabilities {
		capabilities[index] = capability.Signature
		permitted[index] = capability.Name
	}
	return assemblyline.FragmentModificationInput{
		Language: "go", Signature: repository.target.Signature,
		CurrentDeclaration: repository.span.Content,
		RequirementQuote:   repository.target.RequirementQuote,
		Capabilities:       capabilities,
		PermittedSymbols:   permitted,
	}
}
