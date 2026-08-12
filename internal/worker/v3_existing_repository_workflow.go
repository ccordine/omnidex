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
	resolutions, err := prepareExistingRepositoryRequirementResolutions(
		partition.FeatureQuotes,
		func(query string) (repositoryretrieval.EvidencePack, error) {
			if authorityErr := session.requireCurrentRepositoryAuthority(
				"repository evidence acquisition",
			); authorityErr != nil {
				return repositoryretrieval.EvidencePack{}, authorityErr
			}
			return session.buildExistingRepositoryEvidence(query)
		},
		func(requirementQuote string) (assemblyline.RepositorySearchTermDecision, error) {
			searchTermModel, modelErr := session.repositorySemanticModel(
				"coding_repository_search_term", specialist.RoleCodingRepositorySearchTermStation,
			)
			if modelErr != nil {
				return assemblyline.RepositorySearchTermDecision{}, modelErr
			}
			return generateExistingRepositorySearchTerm(
				directCodingWorkerRuntime(session), searchTermModel, requirementQuote, identities,
			)
		},
		func(acquisition existingRepositoryEvidenceAcquisition) error {
			return session.recordExistingRepositoryEvidence(acquisition.Query, acquisition.Pack)
		},
		func(acquisition existingRepositoryEvidenceAcquisition) (assemblyline.RepositoryChangeSurfaceDecision, error) {
			changeModel, modelErr := session.repositorySemanticModel(
				"coding_repository_change_surface", specialist.RoleCodingRepositoryChangeStation,
			)
			if modelErr != nil {
				return assemblyline.RepositoryChangeSurfaceDecision{}, modelErr
			}
			if authorityErr := session.requireCurrentRepositoryAuthority(
				"change-surface projection",
			); authorityErr != nil {
				return assemblyline.RepositoryChangeSurfaceDecision{}, authorityErr
			}
			return selectExistingRepositoryRequirementSurface(
				directCodingWorkerRuntime(session), changeModel, acquisition, identities,
			)
		},
	)
	if err != nil {
		return "", err
	}
	contract, err := session.buildExistingRepositoryChangeContract(resolutions)
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
	searchTerm string,
) (repositoryretrieval.EvidencePack, error) {
	if session == nil || session.runtime == nil || session.runtime.svc == nil ||
		session.runtime.svc.repo == nil || session.repositoryIndex == nil {
		return repositoryretrieval.EvidencePack{}, fmt.Errorf(
			"repository evidence acquisition requires runtime, store, and immutable index authority",
		)
	}
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
		request, requestErr := newExistingRepositoryEvidenceRequest(projectID, analysis.ID, searchTerm)
		if requestErr != nil {
			return repositoryretrieval.EvidencePack{}, requestErr
		}
		pack, buildErr := session.runtime.svc.repositoryRetrieval.Build(session.runtime.ctx, request)
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
			"%w: code-owned lexical neighborhood for %q matched no complete analysis",
			repositoryretrieval.ErrInsufficientEvidence, searchTerm,
		)
	}
	if len(packs) > 1 {
		return repositoryretrieval.EvidencePack{}, fmt.Errorf(
			"repository evidence spans %d language analyses; composite evidence is required", len(packs),
		)
	}
	return packs[0], nil
}

func newExistingRepositoryEvidenceRequest(
	projectID int64,
	analysisID string,
	searchTerm string,
) (repositoryretrieval.Request, error) {
	if projectID < 1 {
		return repositoryretrieval.Request{}, fmt.Errorf("repository evidence request requires durable project authority")
	}
	if strings.TrimSpace(analysisID) == "" {
		return repositoryretrieval.Request{}, fmt.Errorf("repository evidence request requires one analysis ID")
	}
	if _, err := repositoryretrieval.NewQueryBinding(
		repositoryretrieval.OperationSemanticExcerpts, searchTerm,
	); err != nil {
		return repositoryretrieval.Request{}, fmt.Errorf("repository evidence search term: %w", err)
	}
	return repositoryretrieval.Request{
		ProjectID:  projectID,
		AnalysisID: analysisID,
		Operation:  repositoryretrieval.OperationSemanticExcerpts,
		Query:      searchTerm,
		Limits: repositoryretrieval.Limits{
			MaxSymbols: 8, MaxEdges: 32, MaxSpanBytes: 4 * 1024, MaxPackBytes: 9 * 1024,
		},
	}, nil
}

func (session *directCodingSession) recordExistingRepositoryEvidence(
	searchTerm string,
	pack repositoryretrieval.EvidencePack,
) error {
	if err := pack.ValidateForRequest(repositoryretrieval.OperationSemanticExcerpts, searchTerm); err != nil {
		return fmt.Errorf("record repository evidence: %w", err)
	}
	if session == nil || session.runtime == nil {
		return fmt.Errorf("record repository evidence requires a runtime")
	}
	return session.runtime.writeEvidence(evidence.Record{
		Kind: evidence.KindRepositoryEvidence, SourceType: "repository", SourceRef: pack.ID,
		Summary:    fmt.Sprintf("Bounded repository evidence contains %d symbols and %d relations.", len(pack.Symbols), len(pack.Relations)),
		Confidence: 1,
		Metadata: map[string]any{
			"snapshot_id": pack.SnapshotID, "analysis_id": pack.AnalysisID,
			"operation": repositoryretrieval.OperationSemanticExcerpts, "search_term": searchTerm,
			"pack_bytes": evidencePackBytes(pack),
		},
	})
}

func evidencePackBytes(pack repositoryretrieval.EvidencePack) int {
	raw, _ := json.Marshal(pack)
	return len(raw)
}
