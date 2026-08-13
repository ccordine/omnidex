package worker

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/gryph/omnidex/internal/assemblyline"
	"github.com/gryph/omnidex/internal/evidence"
	repositoryfacts "github.com/gryph/omnidex/internal/repository"
	repositoryretrieval "github.com/gryph/omnidex/internal/repository/retrieval"
)

func (session *directCodingSession) buildExistingRepositoryChangeContract(
	resolutions []existingRepositoryRequirementResolution,
) (repositoryfacts.ChangeContract, error) {
	if session == nil || session.repositoryIndex == nil {
		return repositoryfacts.ChangeContract{}, fmt.Errorf("repository change contract requires one immutable index")
	}
	if len(resolutions) == 0 {
		return repositoryfacts.ChangeContract{}, fmt.Errorf("repository change contract requires resolved requirement evidence")
	}
	requests := make([]repositoryfacts.ChangeRequest, 0, len(resolutions))
	seenRequirements := make(map[string]struct{}, len(resolutions))
	seenSymbols := make(map[string]string)
	var snapshotID, analysisID string
	for index, resolution := range resolutions {
		acquisition := resolution.Acquisition
		if err := acquisition.Pack.ValidateForRequest(
			repositoryretrieval.OperationSemanticExcerpts, acquisition.Query,
		); err != nil {
			return repositoryfacts.ChangeContract{}, fmt.Errorf(
				"repository change evidence for requirement %q: %w",
				acquisition.RequirementQuote, err,
			)
		}
		if acquisition.RequirementQuote == "" ||
			acquisition.RequirementQuote != strings.TrimSpace(acquisition.RequirementQuote) {
			return repositoryfacts.ChangeContract{}, fmt.Errorf(
				"repository change evidence %d requires one trimmed requirement quote", index,
			)
		}
		if _, duplicate := seenRequirements[acquisition.RequirementQuote]; duplicate {
			return repositoryfacts.ChangeContract{}, fmt.Errorf(
				"repository change requirement %q is duplicated", acquisition.RequirementQuote,
			)
		}
		seenRequirements[acquisition.RequirementQuote] = struct{}{}
		if index == 0 {
			snapshotID, analysisID = acquisition.Pack.SnapshotID, acquisition.Pack.AnalysisID
		} else if acquisition.Pack.SnapshotID != snapshotID || acquisition.Pack.AnalysisID != analysisID {
			return repositoryfacts.ChangeContract{}, fmt.Errorf(
				"repository requirement evidence must share one snapshot and analysis",
			)
		}
		surfaceInput := assemblyline.RepositoryChangeSurfaceInput{
			ResearchNeed:      acquisition.RequirementQuote,
			RequirementQuotes: []string{acquisition.RequirementQuote},
			Evidence:          acquisition.Pack,
		}
		if err := resolution.Surface.ValidateFor(surfaceInput); err != nil {
			return repositoryfacts.ChangeContract{}, fmt.Errorf(
				"repository change surface for requirement %q: %w",
				acquisition.RequirementQuote, err,
			)
		}
		if len(resolution.Surface.UnresolvedRequirementQuotes) != 0 {
			return repositoryfacts.ChangeContract{}, fmt.Errorf(
				"repository change surface for requirement %q remains unresolved and requires desired-state compilation",
				acquisition.RequirementQuote,
			)
		}
		for _, target := range resolution.Surface.Targets {
			if priorRequirement, duplicate := seenSymbols[target.SymbolID]; duplicate {
				if priorRequirement != acquisition.RequirementQuote {
					return repositoryfacts.ChangeContract{}, fmt.Errorf(
						"repository change target %q maps to different requirements %q and %q; multi-requirement targets are unsupported",
						target.SymbolID, priorRequirement, acquisition.RequirementQuote,
					)
				}
				return repositoryfacts.ChangeContract{}, fmt.Errorf(
					"repository change target %q is duplicated for requirement %q",
					target.SymbolID, acquisition.RequirementQuote,
				)
			}
			seenSymbols[target.SymbolID] = acquisition.RequirementQuote
			requests = append(requests, repositoryfacts.ChangeRequest{
				SymbolID: target.SymbolID, RequirementQuote: target.RequirementQuote,
			})
		}
	}
	if session.repositoryIndex.Snapshot.ID != snapshotID {
		return repositoryfacts.ChangeContract{}, fmt.Errorf(
			"repository change evidence snapshot %q differs from current index %q",
			snapshotID, session.repositoryIndex.Snapshot.ID,
		)
	}
	analysis, err := exactRepositoryChangeAnalysis(session.repositoryIndex.Analyses, analysisID)
	if err != nil {
		return repositoryfacts.ChangeContract{}, fmt.Errorf("repository change evidence: %w", err)
	}
	contract, err := repositoryfacts.BuildChangeContract(
		session.repositoryIndex.Snapshot, analysis, requests,
	)
	if err != nil {
		return repositoryfacts.ChangeContract{}, fmt.Errorf("build repository change contract: %w", err)
	}
	return contract, nil
}

func (session *directCodingSession) recordExistingRepositoryChangeContract(
	contract repositoryfacts.ChangeContract,
) error {
	if session == nil || session.runtime == nil {
		return fmt.Errorf("record repository change contract requires a runtime")
	}
	raw, err := json.Marshal(contract)
	if err != nil {
		return fmt.Errorf("encode repository change contract: %w", err)
	}
	return session.runtime.writeEvidence(evidence.Record{
		Kind: evidence.KindRepositoryChangeContract, SourceType: "repository",
		SourceRef: contract.ID, Hash: strings.TrimPrefix(contract.ID, "change_contract_"),
		Excerpt: string(raw), Confidence: 1,
		Summary: fmt.Sprintf("Hash-bound repository change contract contains %d symbol targets.", len(contract.Targets)),
		Metadata: map[string]any{
			"snapshot_id": contract.SnapshotID, "analysis_id": contract.AnalysisID,
			"target_count": len(contract.Targets),
		},
	})
}
