package webresearch

import (
	"context"
	"fmt"

	"github.com/gryph/omnidex/internal/assemblyline"
)

// EvidenceRequest is the exact authority required to acquire and select web
// evidence. It contains no provider, operation, or tool choice because code
// owns those mechanics through Acquisition.
type EvidenceRequest struct {
	ID                 ObjectiveID
	Question           string
	Context            assemblyline.ObjectiveContext
	InitialQuery       string
	KnownArtifactPaths []string
}

// EvidenceConfig contains only the code-owned bounds used before synthesis.
type EvidenceConfig struct {
	MaxFetchCandidates    int
	MaxProjectionBytes    int
	MaxRelevantCandidates int
	CandidateSummaryBytes int
}

// EvidenceResult contains only exact selected evidence and its bounded
// model-visible projection. Acquisition mechanics and ordering remain code
// owned. The semantic station returns only one raw relevance relation per
// candidate; code retains the exact query and builds the candidate-ID selection.
type EvidenceResult struct {
	Evidence            []Evidence
	Projected           []ProjectedEvidence
	AcquisitionAttempts int
	DiscoveryAttempts   int
	FetchAttempts       int
	RelevanceCalls      int
	SemanticCalls       int
	CallLedger          SemanticCallLedger
}

type evidenceMachine struct {
	objective Objective
	config    EvidenceConfig
	relevance RelevanceStation
	contracts acquisitionContracts
}

// GatherRelevantEvidence runs the shared deterministic web evidence sieve.
// Models never see or choose providers, fetch operations, or tool arguments.
func GatherRelevantEvidence(
	ctx context.Context,
	request EvidenceRequest,
	config EvidenceConfig,
	acquisition Acquisition,
	relevance RelevanceStation,
) (EvidenceResult, error) {
	if ctx == nil {
		return EvidenceResult{}, ErrNilContext
	}
	if err := ctx.Err(); err != nil {
		return EvidenceResult{}, err
	}
	objective := Objective{
		ID: request.ID, Question: request.Question,
		Context:            assemblyline.CloneObjectiveContext(request.Context),
		InitialQuery:       request.InitialQuery,
		KnownArtifactPaths: append([]string{}, request.KnownArtifactPaths...),
		Status:             ObjectivePending,
	}
	if err := validateObjective(objective); err != nil {
		return EvidenceResult{}, err
	}
	if err := validateEvidenceConfig(config); err != nil {
		return EvidenceResult{}, err
	}
	if nilInterface(acquisition) || nilInterface(relevance) {
		return EvidenceResult{}, fmt.Errorf(
			"%w: acquisition and relevance are required", ErrInvalidConfiguration,
		)
	}
	limits := acquisition.Limits()
	if limits.MaxDocuments < 1 || limits.MaxDocuments > 32 ||
		config.MaxFetchCandidates > limits.MaxDocuments {
		return EvidenceResult{}, fmt.Errorf(
			"%w: workflow fetch bound %d exceeds deterministic acquisition bound %d",
			ErrInvalidConfiguration, config.MaxFetchCandidates, limits.MaxDocuments,
		)
	}
	contracts, err := newAcquisitionContracts(acquisition, config.MaxFetchCandidates)
	if err != nil {
		return EvidenceResult{}, fmt.Errorf("%w: acquisition contracts: %w", ErrInvalidConfiguration, err)
	}
	machine := &evidenceMachine{
		objective: objective,
		config: EvidenceConfig{
			MaxFetchCandidates: config.MaxFetchCandidates, MaxProjectionBytes: config.MaxProjectionBytes,
			MaxRelevantCandidates: config.MaxRelevantCandidates, CandidateSummaryBytes: config.CandidateSummaryBytes,
		},
		relevance: relevance, contracts: contracts,
	}
	result := evidenceRun{
		Objective:               cloneObjective(objective),
		AcquisitionAttemptLimit: 2,
	}
	if err := machine.gatherRelevantEvidence(ctx, &result); err != nil {
		return evidenceResultFromRun(result, nil), err
	}
	selected, err := evidenceForProjection(result.Evidence, result.Projected)
	if err != nil {
		return evidenceResultFromRun(result, nil), err
	}
	return evidenceResultFromRun(result, selected), nil
}

func evidenceResultFromRun(result evidenceRun, selected []Evidence) EvidenceResult {
	return EvidenceResult{
		Evidence: cloneEvidence(selected), Projected: cloneProjection(result.Projected),
		AcquisitionAttempts: result.AcquisitionAttempts,
		DiscoveryAttempts:   result.DiscoveryAttempts, FetchAttempts: result.FetchAttempts,
		RelevanceCalls: result.RelevanceCalls,
		SemanticCalls:  result.SemanticCalls,
		CallLedger:     result.CallLedger.Clone(),
	}
}

func evidenceForProjection(evidence []Evidence, projected []ProjectedEvidence) ([]Evidence, error) {
	byID := make(map[EvidenceID]Evidence, len(evidence))
	for _, item := range evidence {
		byID[item.ID] = item
	}
	selected := make([]Evidence, len(projected))
	for index, projection := range projected {
		item, exists := byID[projection.EvidenceID]
		if !exists || item.CandidateID != projection.CandidateID {
			return nil, fmt.Errorf("%w: projected evidence %q lost acquisition authority", ErrInvalidAcquisition, projection.EvidenceID)
		}
		if projection.Truncated {
			item.Truncated = true
		}
		selected[index] = item
	}
	return selected, nil
}
