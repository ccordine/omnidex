package queue

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/gryph/omnidex/internal/cognition"
	"github.com/gryph/omnidex/internal/cognitionruntime"
	"github.com/gryph/omnidex/internal/cognitionstate"
	"github.com/gryph/omnidex/internal/taskstate"
	"github.com/jackc/pgx/v5"
)

func planCognitionObligationMaterialization(
	episode CognitionEpisode,
	graph CognitionObligationGraphRecord,
	prepared cognitionruntime.PreparedSnapshot,
	decision cognition.CognitionDecision,
	actionSchema cognition.ActionSchema,
) (*cognition.ObligationMaterialization, error) {
	proposalIndex := -1
	for index, proposal := range decision.Proposals {
		if proposal.Kind != cognition.ProposalObligation {
			continue
		}
		if proposalIndex >= 0 {
			return nil, fmt.Errorf("%w: a decision may materialize only one obligation", ErrCognitionConflict)
		}
		proposalIndex = index
	}
	if proposalIndex < 0 {
		return nil, nil
	}
	materialization, err := cognitionstate.MaterializeObligationProposal(
		cognitionstate.ObligationProposalInput{
			Graph: graph.Graph, Snapshot: prepared.Snapshot, Decision: decision,
			ActionSchema: actionSchema, ProposalIndex: proposalIndex,
			CompletionAuthority: episode.Completion,
		},
	)
	if err != nil {
		return nil, fmt.Errorf("materialize cognition obligation proposal: %w", err)
	}
	if materialization.ExpectedGraphSHA256 != graph.Graph.SHA256 ||
		materialization.SourceSnapshotSHA256 != prepared.Snapshot.SHA256() {
		return nil, fmt.Errorf("%w: obligation materialization changed source authority", ErrCognitionConflict)
	}
	return materializationPointer(materialization), nil
}

func insertCognitionObligationMaterializationTx(
	ctx context.Context,
	tx pgx.Tx,
	episode CognitionEpisode,
	graphVersion uint64,
	reconciliationID string,
	ledgerID taskstate.LedgerID,
	candidateEntryID taskstate.EntryID,
	materialization cognition.ObligationMaterialization,
) error {
	if err := materialization.Validate(); err != nil {
		return err
	}
	raw, rawSHA, err := cognitionJSON(materialization)
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `
		INSERT INTO cognition_obligation_materializations (
			materialization_id,materialization_sha256,episode_id,job_id,generation,step_id,
			reconciliation_id,ledger_id,candidate_entry_id,source_snapshot_sha256,source_decision_sha256,
			source_proposal_sha256,proposal_index,expected_graph_version,expected_graph_sha256,
			active_obligation_id,child_obligation_id,result_graph_sha256,
			descriptor_json,descriptor_json_sha256
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,$20)
	`, materialization.ID, materialization.SHA256, episode.EpisodeID,
		episode.Authority.JobID, episode.Authority.Generation, episode.Authority.StepID,
		reconciliationID, ledgerID, candidateEntryID, materialization.SourceSnapshotSHA256,
		materialization.SourceDecisionSHA256, materialization.SourceProposalSHA256,
		materialization.ProposalIndex, int64(graphVersion), materialization.ExpectedGraphSHA256,
		materialization.ActiveObligationID, materialization.Spec.ID,
		materialization.ResultGraphSHA256, string(raw), rawSHA)
	if err != nil {
		return fmt.Errorf("insert cognition obligation materialization: %w", err)
	}
	return insertCognitionGraphMaterializationSourceTx(
		ctx, tx, episode.EpisodeID, reconciliationID, ledgerID, candidateEntryID,
		cognition.ProposalObligation, materialization.ProposalIndex,
		materialization.ID, materialization.SHA256,
	)
}

func obligationMaterializationCandidate(
	mutations []cognitionstate.EntryMutation,
	materialization *cognition.ObligationMaterialization,
) (taskstate.EntryID, error) {
	if materialization == nil {
		return "", nil
	}
	var candidate taskstate.EntryID
	for _, mutation := range mutations {
		if mutation.Descriptor().SourceKind != cognitionstate.SourceModelObligation {
			continue
		}
		var metadata struct {
			CandidateKind string `json:"candidate_kind"`
			ProposalIndex int    `json:"proposal_index"`
		}
		if err := json.Unmarshal(mutation.Command().Metadata.Bytes(), &metadata); err != nil ||
			metadata.CandidateKind != "obligation" || metadata.ProposalIndex != materialization.ProposalIndex ||
			candidate != "" {
			return "", fmt.Errorf("%w: obligation candidate does not exactly bind its materialization", ErrCognitionConflict)
		}
		candidate = mutation.Descriptor().EntryID
	}
	if candidate == "" {
		return "", fmt.Errorf("%w: obligation materialization has no exact model candidate", ErrCognitionConflict)
	}
	return candidate, nil
}

func requireCognitionMaterializationReplayTx(
	ctx context.Context,
	tx pgx.Tx,
	reconciliationID string,
	want *cognition.ObligationMaterialization,
) error {
	var raw []byte
	var rawSHA string
	err := tx.QueryRow(ctx, `
		SELECT descriptor_json,descriptor_json_sha256
		FROM cognition_obligation_materializations WHERE reconciliation_id=$1
	`, reconciliationID).Scan(&raw, &rawSHA)
	if errors.Is(err, pgx.ErrNoRows) {
		if want == nil {
			return nil
		}
		return fmt.Errorf("%w: reconciliation lost its obligation materialization", ErrCognitionConflict)
	}
	if err != nil {
		return err
	}
	if want == nil {
		return fmt.Errorf("%w: reconciliation gained an obligation materialization", ErrCognitionConflict)
	}
	wantRaw, wantSHA, err := cognitionJSON(*want)
	if err != nil {
		return err
	}
	if rawSHA != wantSHA || !bytes.Equal(raw, wantRaw) {
		return fmt.Errorf("%w: obligation materialization replay changed content", ErrCognitionConflict)
	}
	var persisted cognition.ObligationMaterialization
	if err := json.Unmarshal(raw, &persisted); err != nil || persisted.Validate() != nil {
		return fmt.Errorf("%w: persisted obligation materialization is invalid", ErrCognitionConflict)
	}
	return nil
}

func materializationPointer(
	value cognition.ObligationMaterialization,
) *cognition.ObligationMaterialization {
	copy := value.Clone()
	return &copy
}
