package worker

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/gryph/omnidex/internal/assemblyline"
	"github.com/gryph/omnidex/internal/evidence"
	repositoryretrieval "github.com/gryph/omnidex/internal/repository/retrieval"
	"github.com/gryph/omnidex/internal/station"
)

type repositoryEvidenceBuilder interface {
	Build(context.Context, repositoryretrieval.Request) (repositoryretrieval.EvidencePack, error)
}

func (session *directCodingSession) runExistingRepositoryChangeWorkflow() (string, error) {
	if session == nil || session.runtime == nil || session.repositoryIndex == nil {
		return "", fmt.Errorf("existing repository change workflow requires an immutable index")
	}
	authority := existingRepositoryAuthority(session.request)
	redacted, identities, err := assemblyline.RedactArtifactIdentities(
		authority, session.pathProvenance,
	)
	if err != nil {
		return "", err
	}
	partitionModel, err := session.workerModel(station.CodingRequirements)
	if err != nil {
		return "", err
	}
	applicationContext, err := assemblyline.BootstrapApplicationContext(
		redacted, assemblyline.ApplicationWorkspaceExisting, session.request.MemoryAuthorities,
	)
	if err != nil {
		return "", err
	}
	applicationContext, err = resolveDirectCodingApplicationContext(
		directCodingWorkerRuntime(session), partitionModel, redacted,
		applicationContext, identities, session.resolveApplicationRepositoryEvidenceNeed,
	)
	if err != nil {
		return "", err
	}
	directives, err := classifyExistingRepositoryArtifactDirectives(
		session, redacted, identities,
	)
	if err != nil {
		return "", err
	}
	featureQuotes, err := interpretRepositoryRequirements(
		directCodingWorkerRuntime(session), partitionModel, redacted, applicationContext, identities,
	)
	if err != nil {
		return "", err
	}
	analysis, err := exactExistingRepositoryGoAnalysis(
		session.repositoryIndex.Snapshot, session.repositoryIndex.Analyses,
	)
	if err != nil {
		return "", err
	}
	absenceQuotes, err := session.classifyPathFreeArtifactAbsence(
		featureQuotes, directives, identities,
	)
	if err != nil {
		return "", err
	}
	if containsArtifactAbsenceCandidate(directives) {
		directives, err = session.resolveNamedArtifactDeletionCandidates(
			featureQuotes, directives, identities, analysis,
		)
		if err != nil {
			return "", err
		}
	}
	if containsForbiddenArtifactDirective(directives) {
		return session.runNamedArtifactDeletion(
			authority, featureQuotes, directives, identities, analysis,
		)
	}
	if len(absenceQuotes) != 0 {
		return session.runPathFreeArtifactDeletion(
			authority, featureQuotes, absenceQuotes,
			identities, analysis,
		)
	}
	signature, quote, missing, err := explicitMissingGoArtifactCandidate(
		authority, featureQuotes, analysis,
	)
	if err != nil {
		return "", err
	}
	if missing {
		if err := validateDesiredCreationFeatureCoverage(
			quote, featureQuotes, directives,
		); err != nil {
			return "", err
		}
		boundaryModel, modelErr := session.workerModel(station.CodingDeclarationArtifactBoundary)
		if modelErr != nil {
			return "", modelErr
		}
		boundary, boundaryErr := classifyDeclarationArtifactBoundary(
			directCodingWorkerRuntime(session), boundaryModel,
			assemblyline.DeclarationArtifactBoundaryInput{
				RequirementQuote: quote, GoSignature: signature.Canonical,
				DeclarationID: "DECLARATION_1",
			},
		)
		if boundaryErr != nil {
			return "", boundaryErr
		}
		if boundary.Boundary != assemblyline.DeclarationBoundaryIndependentArtifact {
			return "", fmt.Errorf(
				"missing Go declaration has no accepted independent artifact boundary; ordinary bounded modification or explicit placement is required",
			)
		}
		graph, graphErr := compileDesiredArtifactCreation(
			authority, featureQuotes,
			session.repositoryIndex.Snapshot, analysis,
		)
		if graphErr != nil {
			return "", graphErr
		}
		if err := session.recordDesiredRepositoryGraph(graph); err != nil {
			return "", err
		}
		return session.runExistingRepositoryDesiredState(graph, analysis)
	}
	if err := session.openExistingRepositoryEvidenceNeeds(featureQuotes); err != nil {
		return "", err
	}
	resolutions, err := prepareExistingRepositoryRequirementResolutions(
		featureQuotes,
		func(query string) (repositoryretrieval.EvidencePack, error) {
			if authorityErr := session.requireCurrentRepositoryAuthority(
				"repository evidence acquisition",
			); authorityErr != nil {
				return repositoryretrieval.EvidencePack{}, authorityErr
			}
			return session.buildExistingRepositoryEvidence(query)
		},
		func(requirementQuote string) (assemblyline.RepositorySearchTermDecision, error) {
			searchTermModel, modelErr := session.repositorySemanticModel(station.CodingRepositorySearchTerm)
			if modelErr != nil {
				return assemblyline.RepositorySearchTermDecision{}, modelErr
			}
			return generateExistingRepositorySearchTerm(
				directCodingWorkerRuntime(session), searchTermModel, requirementQuote, identities,
			)
		},
		func(acquisition existingRepositoryEvidenceAcquisition) error {
			return session.recordExistingRepositoryInvestigation(acquisition)
		},
		func(acquisition existingRepositoryEvidenceAcquisition) (assemblyline.RepositoryChangeSurfaceDecision, error) {
			changeModel, modelErr := session.repositorySemanticModel(station.CodingRepositoryChange)
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
	for _, resolution := range resolutions {
		if err := session.recordExistingRepositoryInvestigationResolution(resolution); err != nil {
			return "", err
		}
	}
	contract, err := session.buildExistingRepositoryChangeContract(resolutions)
	if err != nil {
		return "", err
	}
	if err := session.recordExistingRepositoryChangeContract(contract); err != nil {
		return "", err
	}
	analysis, err = exactRepositoryChangeAnalysis(
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

func containsForbiddenArtifactDirective(directives []assemblyline.ArtifactDirective) bool {
	for _, directive := range directives {
		if directive.Disposition == assemblyline.ArtifactForbid {
			return true
		}
	}
	return false
}

func containsArtifactAbsenceCandidate(directives []assemblyline.ArtifactDirective) bool {
	for _, directive := range directives {
		if directive.Disposition == assemblyline.ArtifactAbsenceCandidate {
			return true
		}
	}
	return false
}

func existingRepositoryAuthority(request directCodingRequest) string {
	parts := []string{request.Instruction}
	parts = append(parts, request.AdditionalAuthority...)
	parts = append(parts, request.Feedback...)
	return strings.TrimSpace(strings.Join(parts, "\n"))
}

func (session *directCodingSession) repositorySemanticModel(id station.ID) (string, error) {
	modelName, err := stationModel(session.runtime.routing, id)
	if err != nil {
		return "", err
	}
	return requireDirectCodingModel(id, modelName)
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
	return buildObjectiveRepositoryEvidence(
		session.runtime.ctx, session.runtime.svc.repositoryRetrieval,
		projectID, session.repositoryIndex.Analyses, searchTerm,
	)
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
