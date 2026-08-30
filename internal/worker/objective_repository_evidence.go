package worker

import (
	"context"
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/gryph/omnidex/internal/assemblyline"
	"github.com/gryph/omnidex/internal/modelcontext"
	repositoryretrieval "github.com/gryph/omnidex/internal/repository/retrieval"
)

const (
	maxObjectiveRepositoryRequirementBytes    = 4 * 1024
	maxObjectiveRepositoryRelevanceModelCalls = maxObjectiveRepositoryEvidenceCapsules * exactSemanticLeafCalls
	maxObjectiveRepositoryEvidenceModelCalls  = maxObjectiveRepositoryRelevanceModelCalls
)

type objectiveRepositoryRelevanceCall func(
	string, []objectiveEvidence,
) (assemblyline.RepositoryEvidenceRelevanceDecision, objectiveStationReceipt, error)

type objectiveRepositoryAcquisitionCallLedger struct {
	relevanceReceipt  objectiveStationReceipt
	relevanceRecorded bool
}

func (r *nativeRuntimeV3) acquireObjectiveRepositoryEvidence(
	ctx context.Context,
	authority turnAuthority,
) (objectiveEvidenceAcquisition, error) {
	if r == nil || r.svc == nil || r.claim == nil || r.svc.repo == nil {
		return objectiveEvidenceAcquisition{}, fmt.Errorf("repository-read objective requires an active claimed job")
	}
	if ctx == nil || authority.JobID != r.claim.Job.ID || authority.Instruction != r.claim.Job.Instruction {
		return objectiveEvidenceAcquisition{}, fmt.Errorf("repository-read objective authority does not match the claimed job")
	}
	if r.svc.repositoryRetrieval == nil {
		return objectiveEvidenceAcquisition{}, fmt.Errorf("repository-read objective retrieval is unavailable")
	}
	scope, err := r.svc.workspaceScopeForV3Job(r.claim.Job)
	if err != nil {
		return objectiveEvidenceAcquisition{}, err
	}
	indexed, err := r.captureExistingRepositoryIndex(scope.Root)
	if err != nil {
		return objectiveEvidenceAcquisition{}, err
	}
	paths := make([]string, len(indexed.Snapshot.Files))
	for index, file := range indexed.Snapshot.Files {
		paths[index] = file.Path
	}
	provenance, err := modelcontext.NewArtifactIdentityProvenance(paths)
	if err != nil {
		return objectiveEvidenceAcquisition{}, fmt.Errorf(
			"derive repository-read artifact provenance: %w", err,
		)
	}
	projectID, err := r.svc.repo.JobProjectID(ctx, authority.JobID)
	if err != nil || projectID < 1 {
		return objectiveEvidenceAcquisition{}, fmt.Errorf("repository-read objective requires durable project authority: %w", err)
	}
	query, err := objectiveRepositoryQuery(authority)
	if err != nil {
		return objectiveEvidenceAcquisition{}, err
	}
	return acquireObjectiveRepositoryEvidenceClosure(
		authority.Instruction,
		query,
		provenance,
		func(codeOwnedQuery string) (repositoryretrieval.EvidencePack, error) {
			return buildObjectiveRepositoryEvidence(ctx, r.svc.repositoryRetrieval, projectID, indexed.Analyses, codeOwnedQuery)
		},
		func(exactRequirement string, evidence []objectiveEvidence) (assemblyline.RepositoryEvidenceRelevanceDecision, objectiveStationReceipt, error) {
			return r.resolveObjectiveRepositoryRelevance(ctx, exactRequirement, evidence)
		},
	)
}

func acquireObjectiveRepositoryEvidenceClosure(
	exactRequirement string,
	codeOwnedQuery string,
	provenance assemblyline.ArtifactIdentityProvenance,
	build existingRepositoryEvidenceBuild,
	relevance objectiveRepositoryRelevanceCall,
) (objectiveEvidenceAcquisition, error) {
	if strings.TrimSpace(exactRequirement) == "" || len(exactRequirement) > maxObjectiveRepositoryRequirementBytes ||
		!utf8.ValidString(exactRequirement) || strings.ContainsRune(exactRequirement, '\x00') {
		return objectiveEvidenceAcquisition{}, fmt.Errorf("repository-read evidence closure requires one exact bounded requirement")
	}
	if _, err := repositoryretrieval.NewQueryBinding(
		repositoryretrieval.OperationSemanticExcerpts, codeOwnedQuery,
	); err != nil {
		return objectiveEvidenceAcquisition{}, fmt.Errorf("repository-read code-owned query: %w", err)
	}
	if build == nil || relevance == nil {
		return objectiveEvidenceAcquisition{}, fmt.Errorf("repository-read evidence closure requires acquisition and relevance authority")
	}
	modelRequirement, identities, err := assemblyline.RedactArtifactIdentities(
		exactRequirement, provenance,
	)
	if err != nil {
		return objectiveEvidenceAcquisition{}, fmt.Errorf(
			"redact repository-read requirement identities: %w", err,
		)
	}
	if err := assemblyline.ValidatePathFreeModelContextWithProvenance(
		"repository-read model requirement", provenance, modelRequirement,
	); err != nil {
		return objectiveEvidenceAcquisition{}, err
	}
	pack, err := build(codeOwnedQuery)
	if err != nil {
		return objectiveEvidenceAcquisition{}, fmt.Errorf("repository-read deterministic acquisition: %w", err)
	}
	if err := pack.ValidateForRequest(
		repositoryretrieval.OperationSemanticExcerpts, codeOwnedQuery,
	); err != nil {
		return objectiveEvidenceAcquisition{}, fmt.Errorf("repository-read acquisition returned invalid evidence: %w", err)
	}
	evidence, err := repositoryEvidenceCapsules(pack, provenance)
	if err != nil {
		return objectiveEvidenceAcquisition{}, err
	}
	input, err := objectiveRepositoryRelevanceInput(modelRequirement, evidence)
	if err != nil {
		return objectiveEvidenceAcquisition{}, err
	}
	decision, receipt, err := relevance(modelRequirement, cloneObjectiveRepositoryEvidence(evidence))
	if err != nil {
		return objectiveEvidenceAcquisition{}, err
	}
	ledger := objectiveRepositoryAcquisitionCallLedger{}
	if err := ledger.recordRelevance(receipt); err != nil {
		return objectiveEvidenceAcquisition{}, err
	}
	if err := decision.ValidateFor(input); err != nil {
		return objectiveEvidenceAcquisition{}, err
	}
	if len(decision.EvidenceIDs) == 0 {
		return objectiveEvidenceAcquisition{}, fmt.Errorf(
			"%w: exact code-owned repository query returned no evidence relevant to the unresolved semantic requirement",
			repositoryretrieval.ErrInsufficientEvidence,
		)
	}
	selected, err := filterObjectiveRepositoryEvidence(evidence, decision.EvidenceIDs)
	if err != nil {
		return objectiveEvidenceAcquisition{}, err
	}
	return newObjectiveRepositoryEvidenceAcquisition(
		selected, ledger, modelRequirement, provenance, identities,
	)
}

func newObjectiveRepositoryEvidenceAcquisition(
	evidence []objectiveEvidence,
	ledger objectiveRepositoryAcquisitionCallLedger,
	groundedRequirement string,
	provenance assemblyline.ArtifactIdentityProvenance,
	identities []assemblyline.ArtifactIdentity,
) (objectiveEvidenceAcquisition, error) {
	modelCalls, err := ledger.totalForSuccess()
	if err != nil {
		return objectiveEvidenceAcquisition{}, err
	}
	result := objectiveEvidenceAcquisition{
		Evidence: evidence, ModelCalls: modelCalls,
		GroundedRequirement:  groundedRequirement,
		KnownArtifactPaths:   provenance.Paths(),
		ArtifactIdentities:   append([]assemblyline.ArtifactIdentity(nil), identities...),
		RepositoryCallLedger: ledger,
	}
	if err := validateObjectiveRepositoryEvidenceAcquisition(result); err != nil {
		return objectiveEvidenceAcquisition{}, err
	}
	return result, nil
}

func validateObjectiveRepositoryEvidenceAcquisition(acquisition objectiveEvidenceAcquisition) error {
	if len(acquisition.Evidence) < 1 || len(acquisition.Evidence) > maxRepositoryGroundedCitations {
		return fmt.Errorf("repository-read acquisition requires 1..%d selected evidence capsules", maxRepositoryGroundedCitations)
	}
	modelCalls, err := acquisition.RepositoryCallLedger.totalForSuccess()
	if err != nil {
		return err
	}
	if acquisition.ModelCalls != modelCalls {
		return fmt.Errorf(
			"repository-read acquisition model-call total %d differs from exact typed ledger %d",
			acquisition.ModelCalls, modelCalls,
		)
	}
	if strings.TrimSpace(acquisition.GroundedRequirement) == "" ||
		len(acquisition.GroundedRequirement) > maxObjectiveRepositoryRequirementBytes {
		return fmt.Errorf("repository-read acquisition requires one exact grounded requirement")
	}
	provenance, err := modelcontext.NewArtifactIdentityProvenance(
		acquisition.KnownArtifactPaths,
	)
	if err != nil {
		return fmt.Errorf("repository-read acquisition artifact provenance: %w", err)
	}
	if err := assemblyline.ValidatePathFreeModelContextWithProvenance(
		"repository-read grounded requirement", provenance,
		acquisition.GroundedRequirement,
	); err != nil {
		return err
	}
	knownPaths := make(map[string]struct{}, len(acquisition.KnownArtifactPaths))
	for _, path := range acquisition.KnownArtifactPaths {
		knownPaths[path] = struct{}{}
	}
	for _, identity := range acquisition.ArtifactIdentities {
		if _, exists := knownPaths[identity.Value]; !exists {
			return fmt.Errorf(
				"repository-read artifact token %s lacks current-tree provenance",
				identity.Token,
			)
		}
		if !strings.Contains(acquisition.GroundedRequirement, identity.Token) {
			return fmt.Errorf(
				"repository-read artifact token %s is absent from the grounded requirement",
				identity.Token,
			)
		}
	}
	if _, err := assemblyline.RestoreArtifactIdentities(
		acquisition.GroundedRequirement, acquisition.ArtifactIdentities,
	); err != nil {
		return fmt.Errorf("repository-read artifact identity bindings: %w", err)
	}
	return nil
}

func (ledger *objectiveRepositoryAcquisitionCallLedger) recordRelevance(receipt objectiveStationReceipt) error {
	if ledger == nil || ledger.relevanceRecorded {
		return fmt.Errorf("repository-read acquisition exceeded its one relevance round")
	}
	if err := validateObjectiveBoundedStationReceipt(
		"repository grounded relevance station",
		receipt,
		maxObjectiveRepositoryRelevanceModelCalls,
	); err != nil {
		return fmt.Errorf(
			"repository-read acquisition relevance-call receipt: %w", err,
		)
	}
	ledger.relevanceReceipt = receipt
	ledger.relevanceRecorded = true
	return nil
}

func (ledger objectiveRepositoryAcquisitionCallLedger) totalForSuccess() (int, error) {
	if !ledger.relevanceRecorded {
		return 0, fmt.Errorf("repository-read acquisition has no recorded relevance-call receipt")
	}
	if err := validateObjectiveBoundedStationReceipt(
		"repository grounded relevance station",
		ledger.relevanceReceipt,
		maxObjectiveRepositoryEvidenceModelCalls,
	); err != nil {
		return 0, fmt.Errorf(
			"repository-read acquisition relevance-call receipt: %w", err,
		)
	}
	return ledger.relevanceReceipt.Calls, nil
}

func objectiveRepositoryEvidenceCallTotal(acquisition objectiveEvidenceAcquisition) (int, error) {
	if err := validateObjectiveRepositoryEvidenceAcquisition(acquisition); err != nil {
		return 0, err
	}
	return acquisition.ModelCalls, nil
}

func objectiveRepositoryQuery(authority turnAuthority) (string, error) {
	query := strings.TrimSpace(authority.Instruction)
	if query == "" || len(query) > maxObjectiveRepositoryRequirementBytes ||
		!utf8.ValidString(query) || strings.ContainsRune(query, '\x00') {
		return "", fmt.Errorf("repository-read objective requires a bounded 1-%d byte exact requirement", maxObjectiveRepositoryRequirementBytes)
	}
	return query, nil
}
