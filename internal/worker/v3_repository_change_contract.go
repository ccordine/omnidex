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
	pack repositoryretrieval.EvidencePack,
	surface assemblyline.RepositoryChangeSurfaceDecision,
) (repositoryfacts.ChangeContract, error) {
	if session == nil || session.repositoryIndex == nil {
		return repositoryfacts.ChangeContract{}, fmt.Errorf("repository change contract requires one immutable index")
	}
	if session.repositoryIndex.Snapshot.ID != pack.SnapshotID {
		return repositoryfacts.ChangeContract{}, fmt.Errorf(
			"repository change evidence snapshot %q differs from current index %q",
			pack.SnapshotID, session.repositoryIndex.Snapshot.ID,
		)
	}
	analysis, err := exactRepositoryChangeAnalysis(session.repositoryIndex.Analyses, pack.AnalysisID)
	if err != nil {
		return repositoryfacts.ChangeContract{}, fmt.Errorf("repository change evidence: %w", err)
	}
	requests := make([]repositoryfacts.ChangeRequest, len(surface.Targets))
	for index, target := range surface.Targets {
		requests[index] = repositoryfacts.ChangeRequest{
			SymbolID: target.SymbolID, RequirementQuote: target.RequirementQuote,
		}
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
