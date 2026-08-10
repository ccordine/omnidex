package queue

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/gryph/omnidex/internal/cognition"
	"github.com/gryph/omnidex/internal/cognitionstate"
	"github.com/gryph/omnidex/internal/taskstate"
	"github.com/jackc/pgx/v5"
)

type cognitionActionGraphMaterialization struct {
	Kind       cognition.LedgerProposalKind
	Graph      uint64
	Candidate  taskstate.EntryID
	Obligation *cognition.ObligationMaterialization
	Revision   *cognition.PlanRevisionMaterialization
}

func (value cognitionActionGraphMaterialization) descriptor() (string, string, int, error) {
	switch value.Kind {
	case cognition.ProposalObligation:
		if value.Obligation == nil || value.Revision != nil {
			break
		}
		return value.Obligation.ID, value.Obligation.SHA256, value.Obligation.ProposalIndex, nil
	case cognition.ProposalPlanRevision:
		if value.Revision == nil || value.Obligation != nil {
			break
		}
		return value.Revision.ID, value.Revision.SHA256, value.Revision.ProposalIndex, nil
	}
	return "", "", 0, fmt.Errorf("%w: graph materialization union is invalid", ErrCognitionConflict)
}

func insertCognitionGraphMaterializationSourceTx(
	ctx context.Context,
	tx pgx.Tx,
	episodeID cognition.EpisodeID,
	reconciliationID string,
	ledgerID taskstate.LedgerID,
	candidate taskstate.EntryID,
	kind cognition.LedgerProposalKind,
	proposalIndex int,
	descriptorID, descriptorSHA string,
) error {
	_, err := tx.Exec(ctx, `
		INSERT INTO cognition_graph_materialization_sources (
			descriptor_id,descriptor_sha256,episode_id,reconciliation_id,ledger_id,
			candidate_entry_id,proposal_kind,proposal_index
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8)
	`, descriptorID, descriptorSHA, episodeID, reconciliationID, ledgerID,
		candidate, kind, proposalIndex)
	if err != nil {
		return fmt.Errorf("insert cognition graph materialization source: %w", err)
	}
	return nil
}

func graphMaterializationCandidate(
	mutations []cognitionstate.EntryMutation,
	present bool,
	source cognitionstate.SourceKind,
	candidateKind string,
	proposalIndex int,
) (taskstate.EntryID, error) {
	if !present {
		return "", nil
	}
	var candidate taskstate.EntryID
	for _, mutation := range mutations {
		if mutation.Descriptor().SourceKind != source {
			continue
		}
		var metadata struct {
			CandidateKind string `json:"candidate_kind"`
			ProposalIndex int    `json:"proposal_index"`
		}
		if json.Unmarshal(mutation.Command().Metadata.Bytes(), &metadata) != nil ||
			metadata.CandidateKind != candidateKind || metadata.ProposalIndex != proposalIndex || candidate != "" {
			return "", fmt.Errorf("%w: graph candidate does not exactly bind its materialization", ErrCognitionConflict)
		}
		candidate = mutation.Descriptor().EntryID
	}
	if candidate == "" {
		return "", fmt.Errorf("%w: graph materialization has no exact model candidate", ErrCognitionConflict)
	}
	return candidate, nil
}

func loadCognitionActionGraphMaterializationTx(
	ctx context.Context,
	tx pgx.Tx,
	record CognitionActionRecord,
) (cognitionActionGraphMaterialization, bool, error) {
	var kind cognition.LedgerProposalKind
	err := tx.QueryRow(ctx, `
		SELECT proposal_kind FROM cognition_graph_materialization_sources
		WHERE reconciliation_id=$1
	`, record.ReconciliationID).Scan(&kind)
	if err == pgx.ErrNoRows {
		return cognitionActionGraphMaterialization{}, false, nil
	}
	if err != nil {
		return cognitionActionGraphMaterialization{}, false, err
	}
	switch kind {
	case cognition.ProposalObligation:
		ordinary, found, err := loadCognitionActionMaterializationTx(ctx, tx, record)
		if err != nil || !found {
			return cognitionActionGraphMaterialization{}, false, err
		}
		copy := ordinary.Materialization.Clone()
		return cognitionActionGraphMaterialization{
			Kind: kind, Graph: ordinary.GraphVersion, Candidate: ordinary.CandidateEntryID,
			Obligation: &copy,
		}, true, nil
	case cognition.ProposalPlanRevision:
		return loadCognitionActionPlanRevisionTx(ctx, tx, record)
	default:
		return cognitionActionGraphMaterialization{}, false,
			fmt.Errorf("%w: unregistered graph materialization kind %q", ErrCognitionConflict, kind)
	}
}
