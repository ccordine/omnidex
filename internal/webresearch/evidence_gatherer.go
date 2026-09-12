package webresearch

import (
	"context"
	"fmt"

	"github.com/gryph/omnidex/internal/assemblyline"
	"github.com/gryph/omnidex/internal/websearch"
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

// EvidenceResult distinguishes selected evidence and its model projection from
// bounded acquisition reports, which can also describe failed or cancelled work.
// Acquisition mechanics and ordering remain code owned. The semantic station
// returns only one raw relevance relation per candidate; code builds the selection.
type EvidenceResult struct {
	Evidence      []Evidence
	Projected     []ProjectedEvidence
	Discovery     []websearch.CandidateReport
	Fetches       []websearch.DocumentReport
	SemanticCalls int
}

type evidenceMachine struct {
	objective   Objective
	config      EvidenceConfig
	relevance   RelevanceStation
	acquisition Acquisition
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
	machine := &evidenceMachine{
		objective: objective,
		config:    config, relevance: relevance, acquisition: acquisition,
	}
	var result evidenceRun
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
	// Acquisition already detached these reports from the provider. Transfer
	// the completed run's owned values without another receipt or copy layer.
	return EvidenceResult{
		Evidence: selected, Projected: result.Projected,
		Discovery: result.Discovery, Fetches: result.Fetches,
		SemanticCalls: result.SemanticCalls,
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
		selected[index] = item
	}
	return selected, nil
}
