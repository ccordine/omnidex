package worker

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/gryph/omnidex/internal/assemblyline"
	"github.com/gryph/omnidex/internal/evidence"
	repositoryfacts "github.com/gryph/omnidex/internal/repository"
	repositoryretrieval "github.com/gryph/omnidex/internal/repository/retrieval"
	"github.com/gryph/omnidex/internal/specialist"
)

type repositoryEvidenceBuilder interface {
	Build(context.Context, repositoryretrieval.Request) (repositoryretrieval.EvidencePack, error)
}

func (session *directCodingSession) runExistingRepositoryChangeWorkflow() (string, error) {
	if session == nil || session.runtime == nil || session.repositoryIndex == nil {
		return "", fmt.Errorf("existing repository change workflow requires an immutable index")
	}
	authority := existingRepositoryAuthority(session.request)
	redacted, identities, err := assemblyline.RedactArtifactIdentities(authority)
	if err != nil {
		return "", err
	}
	partitionModel, err := session.workerModel(
		"coding_requirement_partition", specialist.RoleCodingRequirementPartitionStation,
	)
	if err != nil {
		return "", err
	}
	partition, err := partitionCodingRequirements(
		directCodingWorkerRuntime(session), partitionModel, redacted, identities,
	)
	if err != nil {
		return "", err
	}
	retrievalModel, err := session.repositorySemanticModel(
		"coding_repository_retrieval", specialist.RoleCodingRepositoryRetrievalStation,
	)
	if err != nil {
		return "", err
	}
	decision, err := classifyExistingRepositoryRetrieval(
		directCodingWorkerRuntime(session), retrievalModel, redacted, identities,
	)
	if err != nil {
		return "", err
	}
	if err := session.requireCurrentRepositoryAuthority("repository evidence acquisition"); err != nil {
		return "", err
	}
	pack, err := session.buildExistingRepositoryEvidence(decision)
	if err != nil {
		return "", err
	}
	if err := session.recordExistingRepositoryEvidence(decision, pack); err != nil {
		return "", err
	}
	changeModel, err := session.repositorySemanticModel(
		"coding_repository_change_surface", specialist.RoleCodingRepositoryChangeStation,
	)
	if err != nil {
		return "", err
	}
	if err := session.requireCurrentRepositoryAuthority("change-surface projection"); err != nil {
		return "", err
	}
	surface, err := selectExistingRepositoryChangeSurface(
		directCodingWorkerRuntime(session), changeModel, redacted,
		partition.FeatureQuotes, pack, identities,
	)
	if err != nil {
		return "", err
	}
	contract, err := session.buildExistingRepositoryChangeContract(pack, surface)
	if err != nil {
		return "", err
	}
	if err := session.recordExistingRepositoryChangeContract(contract); err != nil {
		return "", err
	}
	analysis, err := exactRepositoryChangeAnalysis(
		session.repositoryIndex.Analyses, contract.AnalysisID,
	)
	if err != nil {
		return "", err
	}
	commands, err := existingRepositoryGoVerificationCommands(
		session.repositoryIndex.Snapshot, analysis, contract,
	)
	if err != nil {
		return "", err
	}
	baseline, err := session.proveExistingRepositoryBaseline(
		session.repositoryIndex.Snapshot, contract.ID, commands,
	)
	if err != nil {
		return "", err
	}
	candidates, err := session.generateExistingRepositoryChangeCandidates(contract, baseline)
	if err != nil {
		return "", err
	}
	return session.applyExistingRepositoryChangeContract(contract, candidates, baseline)
}

func existingRepositoryAuthority(request directCodingRequest) string {
	parts := []string{request.Instruction}
	parts = append(parts, request.AdditionalAuthority...)
	parts = append(parts, request.Feedback...)
	return strings.TrimSpace(strings.Join(parts, "\n"))
}

func (session *directCodingSession) repositorySemanticModel(skillID, roleID string) (string, error) {
	modelName := session.runtime.svc.v3SpecialistModel(
		session.runtime.claim.Job, session.runtime.routing, skillID, roleID, session.runtime.routing.Glue,
	)
	return requireDirectCodingModel(roleID, modelName)
}

func (session *directCodingSession) buildExistingRepositoryEvidence(
	decision assemblyline.RepositoryRetrievalDecision,
) (repositoryretrieval.EvidencePack, error) {
	if session.runtime.svc.repositoryRetrieval == nil {
		return repositoryretrieval.EvidencePack{}, fmt.Errorf("repository evidence retrieval is unavailable")
	}
	projectID, err := session.runtime.svc.repo.JobProjectID(session.runtime.ctx, session.runtime.claim.Job.ID)
	if err != nil {
		return repositoryretrieval.EvidencePack{}, fmt.Errorf("resolve repository evidence project authority: %w", err)
	}
	if projectID < 1 {
		return repositoryretrieval.EvidencePack{}, fmt.Errorf("repository evidence requires durable project authority")
	}
	analyses := append([]repositoryfacts.Analysis(nil), session.repositoryIndex.Analyses...)
	sort.Slice(analyses, func(left, right int) bool { return analyses[left].ID < analyses[right].ID })
	packs := make([]repositoryretrieval.EvidencePack, 0, len(analyses))
	for _, analysis := range analyses {
		pack, buildErr := session.runtime.svc.repositoryRetrieval.Build(session.runtime.ctx, repositoryretrieval.Request{
			ProjectID: projectID, AnalysisID: analysis.ID,
			Operation: decision.Operation, Query: decision.QueryQuote,
			Limits: repositoryretrieval.Limits{
				MaxSymbols: 8, MaxEdges: 32, MaxSpanBytes: 4 * 1024, MaxPackBytes: 9 * 1024,
			},
		})
		if errors.Is(buildErr, repositoryretrieval.ErrInsufficientEvidence) {
			continue
		}
		if buildErr != nil {
			return repositoryretrieval.EvidencePack{}, buildErr
		}
		packs = append(packs, pack)
	}
	if len(packs) == 0 {
		return repositoryretrieval.EvidencePack{}, fmt.Errorf(
			"%w: %s %q matched no complete analysis",
			repositoryretrieval.ErrInsufficientEvidence, decision.Operation, decision.QueryQuote,
		)
	}
	if len(packs) > 1 {
		return repositoryretrieval.EvidencePack{}, fmt.Errorf(
			"repository evidence spans %d language analyses; composite evidence is required", len(packs),
		)
	}
	return packs[0], nil
}

func (session *directCodingSession) recordExistingRepositoryEvidence(
	decision assemblyline.RepositoryRetrievalDecision,
	pack repositoryretrieval.EvidencePack,
) error {
	if err := pack.ValidateForRequest(decision.Operation, decision.QueryQuote); err != nil {
		return fmt.Errorf("record repository evidence: %w", err)
	}
	return session.runtime.writeEvidence(evidence.Record{
		Kind: evidence.KindRepositoryEvidence, SourceType: "repository", SourceRef: pack.ID,
		Summary:    fmt.Sprintf("Bounded repository evidence contains %d symbols and %d relations.", len(pack.Symbols), len(pack.Relations)),
		Confidence: 1,
		Metadata: map[string]any{
			"snapshot_id": pack.SnapshotID, "analysis_id": pack.AnalysisID,
			"operation": decision.Operation, "query_quote": decision.QueryQuote,
			"pack_bytes": evidencePackBytes(pack),
		},
	})
}

func evidencePackBytes(pack repositoryretrieval.EvidencePack) int {
	raw, _ := json.Marshal(pack)
	return len(raw)
}
