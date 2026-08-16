package worker

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/gryph/omnidex/internal/assemblyline"
	repositoryretrieval "github.com/gryph/omnidex/internal/repository/retrieval"
)

const (
	maxObjectiveRepositoryRequirementBytes   = 4 * 1024
	maxObjectiveRepositorySearchTermRounds   = 1
	maxObjectiveRepositoryRelevanceRounds    = 4
	maxObjectiveRepositoryEvidenceModelCalls = maxTypedWorkerAttempts * (maxObjectiveRepositorySearchTermRounds + maxObjectiveRepositoryRelevanceRounds)
)

type objectiveRepositorySearchTermCall func(
	string,
) (assemblyline.RepositorySearchTermDecision, objectiveStationReceipt, error)

type objectiveRepositoryRelevanceCall func(
	string, []objectiveEvidence,
) (assemblyline.RepositoryEvidenceRelevanceDecision, objectiveStationReceipt, error)

type objectiveRepositoryAcquisitionCallLedger struct {
	searchTermCalls int
	relevanceCalls  []int
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
	indexed, err := r.refreshExistingRepositoryIndex(scope.Root)
	if err != nil {
		return objectiveEvidenceAcquisition{}, err
	}
	projectID, err := r.svc.repo.JobProjectID(ctx, authority.JobID)
	if err != nil || projectID < 1 {
		return objectiveEvidenceAcquisition{}, fmt.Errorf("repository-read objective requires durable project authority: %w", err)
	}
	_, err = objectiveRepositoryQuery(authority)
	if err != nil {
		return objectiveEvidenceAcquisition{}, err
	}
	return acquireObjectiveRepositoryEvidenceClosure(
		authority.Instruction,
		func(searchTerm string) (repositoryretrieval.EvidencePack, error) {
			return buildObjectiveRepositoryEvidence(ctx, r.svc.repositoryRetrieval, projectID, indexed.Analyses, searchTerm)
		},
		func(unresolved string) (assemblyline.RepositorySearchTermDecision, objectiveStationReceipt, error) {
			return r.resolveObjectiveRepositorySearchTerm(ctx, unresolved)
		},
		func(exactRequirement string, evidence []objectiveEvidence) (assemblyline.RepositoryEvidenceRelevanceDecision, objectiveStationReceipt, error) {
			return r.resolveObjectiveRepositoryRelevance(ctx, exactRequirement, evidence)
		},
	)
}

func acquireObjectiveRepositoryEvidenceClosure(
	exactRequirement string,
	build existingRepositoryEvidenceBuild,
	searchTerm objectiveRepositorySearchTermCall,
	relevance objectiveRepositoryRelevanceCall,
) (objectiveEvidenceAcquisition, error) {
	if strings.TrimSpace(exactRequirement) == "" || len(exactRequirement) > maxObjectiveRepositoryRequirementBytes ||
		!utf8.ValidString(exactRequirement) || strings.ContainsRune(exactRequirement, '\x00') {
		return objectiveEvidenceAcquisition{}, fmt.Errorf("repository-read evidence closure requires one exact bounded requirement")
	}
	if build == nil || searchTerm == nil || relevance == nil {
		return objectiveEvidenceAcquisition{}, fmt.Errorf("repository-read evidence closure requires acquisition, search-term, and relevance authority")
	}
	queries := []string{strings.TrimSpace(exactRequirement)}
	anchorsResolved := false
	ledger := objectiveRepositoryAcquisitionCallLedger{}
	if len(exactRequirement) > 512 {
		anchors, receipt, err := resolveObjectiveRepositorySearchTerm(exactRequirement, searchTerm)
		if err != nil {
			return objectiveEvidenceAcquisition{}, err
		}
		if err := ledger.recordSearchTerm(receipt); err != nil {
			return objectiveEvidenceAcquisition{}, err
		}
		query, queryErr := repositoryretrieval.BuildLexicalAnchorQuery(anchors)
		if queryErr != nil {
			return objectiveEvidenceAcquisition{}, queryErr
		}
		queries, anchorsResolved = []string{query}, true
	}
	for queryIndex := 0; ; {
		currentQuery := queries[queryIndex]
		pack, err := build(currentQuery)
		if err == nil {
			if err := pack.ValidateForRequest(repositoryretrieval.OperationSemanticExcerpts, currentQuery); err != nil {
				return objectiveEvidenceAcquisition{}, fmt.Errorf("repository-read acquisition returned invalid evidence: %w", err)
			}
			evidence, err := repositoryEvidenceCapsules(pack)
			if err != nil {
				return objectiveEvidenceAcquisition{}, err
			}
			input, err := objectiveRepositoryRelevanceInput(exactRequirement, evidence)
			if err != nil {
				return objectiveEvidenceAcquisition{}, err
			}
			decision, receipt, err := relevance(exactRequirement, cloneObjectiveRepositoryEvidence(evidence))
			if err != nil {
				return objectiveEvidenceAcquisition{}, err
			}
			if err := ledger.recordRelevance(receipt); err != nil {
				return objectiveEvidenceAcquisition{}, err
			}
			if err := decision.ValidateFor(input); err != nil {
				return objectiveEvidenceAcquisition{}, err
			}
			if decision.Outcome == assemblyline.RepositoryEvidenceRelevant {
				selected, err := filterObjectiveRepositoryEvidence(evidence, decision.EvidenceIDs)
				if err != nil {
					return objectiveEvidenceAcquisition{}, err
				}
				return newObjectiveRepositoryEvidenceAcquisition(selected, ledger)
			}
		} else if !errors.Is(err, repositoryretrieval.ErrInsufficientEvidence) {
			return objectiveEvidenceAcquisition{}, fmt.Errorf("repository-read deterministic acquisition: %w", err)
		}
		queryIndex++
		if queryIndex < len(queries) {
			continue
		}
		if anchorsResolved {
			return objectiveEvidenceAcquisition{}, fmt.Errorf(
				"%w: repository evidence remained insufficient or irrelevant after %d bounded search anchors",
				repositoryretrieval.ErrInsufficientEvidence, len(queries),
			)
		}
		anchors, receipt, err := resolveObjectiveRepositorySearchTerm(exactRequirement, searchTerm)
		if err != nil {
			return objectiveEvidenceAcquisition{}, err
		}
		if err := ledger.recordSearchTerm(receipt); err != nil {
			return objectiveEvidenceAcquisition{}, err
		}
		query, queryErr := repositoryretrieval.BuildLexicalAnchorQuery(anchors)
		if queryErr != nil {
			return objectiveEvidenceAcquisition{}, queryErr
		}
		queries, queryIndex, anchorsResolved = []string{query}, 0, true
	}
}

func newObjectiveRepositoryEvidenceAcquisition(
	evidence []objectiveEvidence,
	ledger objectiveRepositoryAcquisitionCallLedger,
) (objectiveEvidenceAcquisition, error) {
	modelCalls, err := ledger.totalForSuccess()
	if err != nil {
		return objectiveEvidenceAcquisition{}, err
	}
	result := objectiveEvidenceAcquisition{
		Evidence: evidence, ModelCalls: modelCalls,
		RepositoryCallLedger: objectiveRepositoryAcquisitionCallLedger{
			searchTermCalls: ledger.searchTermCalls,
			relevanceCalls:  append([]int(nil), ledger.relevanceCalls...),
		},
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
	return nil
}

func (ledger *objectiveRepositoryAcquisitionCallLedger) recordSearchTerm(receipt objectiveStationReceipt) error {
	if ledger == nil || ledger.searchTermCalls != 0 {
		return fmt.Errorf("repository-read acquisition exceeded its one search-term round")
	}
	if err := validateObjectiveRepositoryStationCalls("search term", receipt.Calls); err != nil {
		return err
	}
	ledger.searchTermCalls = receipt.Calls
	return nil
}

func (ledger *objectiveRepositoryAcquisitionCallLedger) recordRelevance(receipt objectiveStationReceipt) error {
	if ledger == nil || len(ledger.relevanceCalls) >= maxObjectiveRepositoryRelevanceRounds {
		return fmt.Errorf("repository-read acquisition exceeded its %d relevance rounds", maxObjectiveRepositoryRelevanceRounds)
	}
	if err := validateObjectiveRepositoryStationCalls("relevance", receipt.Calls); err != nil {
		return err
	}
	ledger.relevanceCalls = append(ledger.relevanceCalls, receipt.Calls)
	return nil
}

func (ledger objectiveRepositoryAcquisitionCallLedger) totalForSuccess() (int, error) {
	if len(ledger.relevanceCalls) < 1 || len(ledger.relevanceCalls) > maxObjectiveRepositoryRelevanceRounds {
		return 0, fmt.Errorf("repository-read acquisition requires 1..%d relevance rounds", maxObjectiveRepositoryRelevanceRounds)
	}
	if len(ledger.relevanceCalls) > 1 && ledger.searchTermCalls == 0 {
		return 0, fmt.Errorf("repository-read acquisition cannot repeat relevance without its one search-term round")
	}
	total := ledger.searchTermCalls
	for _, calls := range ledger.relevanceCalls {
		total += calls
	}
	if total < 1 || total > maxObjectiveRepositoryEvidenceModelCalls {
		return 0, fmt.Errorf("repository-read acquisition produced invalid model-call total %d", total)
	}
	return total, nil
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
