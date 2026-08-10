package queue

import (
	"context"
	"fmt"

	"github.com/gryph/omnidex/internal/cognition"
	"github.com/gryph/omnidex/internal/cognitionruntime"
	"github.com/jackc/pgx/v5"
)

func sealLifecycleCognitionEpisodeTx(
	ctx context.Context,
	tx pgx.Tx,
	descriptor lifecycleOperationDescriptor,
	episode CognitionEpisode,
	code cognitionruntime.CancellationCode,
	publicOutcome string,
) (cognitionLifecycleSealEntry, error) {
	graph, found, err := loadCurrentCognitionObligationGraphTx(ctx, tx, episode.EpisodeID, true)
	if err != nil || !found {
		return cognitionLifecycleSealEntry{}, fmt.Errorf("%w: load lifecycle cognition graph: %v", ErrCognitionConflict, err)
	}
	retirement, err := newCognitionLifecycleRetirement(descriptor, episode, graph, code)
	if err != nil {
		return cognitionLifecycleSealEntry{}, err
	}
	if err := validateLifecycleCognitionStateTx(ctx, tx, episode, graph); err != nil {
		return cognitionLifecycleSealEntry{}, err
	}
	if err := requireCognitionLifecycleRegistryTx(ctx, tx, retirement); err != nil {
		return cognitionLifecycleSealEntry{}, err
	}
	evidence, err := cognitionruntime.NewLifecycleCancellationEvidence(
		code, publicOutcome, descriptor.SHA256,
	)
	if err != nil {
		return cognitionLifecycleSealEntry{}, err
	}
	if err := insertCognitionLifecycleRetirementTx(ctx, tx, retirement); err != nil {
		return cognitionLifecycleSealEntry{}, err
	}
	if err := insertLifecycleCognitionCancellationTx(ctx, tx, retirement, evidence); err != nil {
		return cognitionLifecycleSealEntry{}, err
	}
	restored, err := cognition.RestoreObligationGraph(graph.Graph)
	if err != nil {
		return cognitionLifecycleSealEntry{}, err
	}
	root, exists := restored.Obligation(graph.Graph.RootID)
	if !exists {
		return cognitionLifecycleSealEntry{}, fmt.Errorf("%w: lifecycle cognition root is missing", ErrCognitionConflict)
	}
	completion, err := cognition.NewCompletionResult(
		root.ID, root.CompletionCheck, episode.CurrentRevision,
		cognition.CompletionUnsatisfied, []cognition.EvidenceRef{},
	)
	if err != nil {
		return cognitionLifecycleSealEntry{}, err
	}
	header, err := loadTaskLedgerHeaderTx(ctx, tx, retirement.JobID, true)
	if err != nil {
		return cognitionLifecycleSealEntry{}, err
	}
	header, err = cancelCognitionObligationNodesTx(
		ctx, tx, header, episode.EpisodeID, graph.Graph,
		retirement.JobID, retirement.JobGeneration, retirement.ID,
	)
	if err != nil {
		return cognitionLifecycleSealEntry{}, err
	}
	workingEvent, err := closeCognitionTerminalWorkingSetTx(
		ctx, tx, header, retirement.JobID, retirement.JobGeneration,
		episode.EpisodeID, CognitionEpisodeCanceled, episode.CurrentRevision,
		graph.Graph, nil, &retirement,
	)
	if err != nil {
		return cognitionLifecycleSealEntry{}, err
	}
	traceJSON, traceSHA, err := buildCognitionTraceAuthorityTx(
		ctx, tx, episode, graph, header.Version, workingEvent.Version,
	)
	if err != nil {
		return cognitionLifecycleSealEntry{}, err
	}
	if err := persistCognitionTerminalSealTx(
		ctx, tx, episode.EpisodeID, CognitionEpisodeCanceled, episode.CurrentRevision,
		completion, graph.Graph, header.Version, workingEvent.Version,
		traceJSON, traceSHA, publicOutcome, lifecycleCognitionTerminalAuthority(retirement),
	); err != nil {
		return cognitionLifecycleSealEntry{}, err
	}
	seal, err := loadCognitionTerminalSealTx(ctx, tx, episode.EpisodeID)
	if err != nil {
		return cognitionLifecycleSealEntry{}, err
	}
	if seal.AuthorityKind != cognitionTerminalAuthorityLifecycle ||
		seal.LifecycleOperationID != descriptor.ID || seal.TraceSHA256 != traceSHA {
		return cognitionLifecycleSealEntry{}, fmt.Errorf("%w: lifecycle cognition seal changed", ErrCognitionConflict)
	}
	return cognitionLifecycleSealEntry{
		EpisodeID: episode.EpisodeID, RetirementID: retirement.ID,
		RetirementSHA256: retirement.SHA256, TraceSHA256: traceSHA,
	}, nil
}
