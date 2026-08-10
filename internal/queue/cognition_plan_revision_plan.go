package queue

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"

	"github.com/gryph/omnidex/internal/cognition"
	"github.com/gryph/omnidex/internal/cognitionruntime"
	"github.com/gryph/omnidex/internal/cognitionstate"
	"github.com/gryph/omnidex/internal/taskstate"
	"github.com/jackc/pgx/v5"
)

func planCognitionPlanRevision(
	episode CognitionEpisode,
	graph CognitionObligationGraphRecord,
	prepared cognitionruntime.PreparedSnapshot,
	decision cognition.CognitionDecision,
	actionSchema cognition.ActionSchema,
) (*cognition.PlanRevisionMaterialization, error) {
	proposalIndex := -1
	for index, proposal := range decision.Proposals {
		if proposal.Kind == cognition.ProposalPlanRevision {
			if proposalIndex >= 0 {
				return nil, fmt.Errorf("%w: a decision may revise a plan only once", ErrCognitionConflict)
			}
			proposalIndex = index
		}
	}
	if proposalIndex < 0 {
		return nil, nil
	}
	value, err := cognitionstate.MaterializePlanRevisionProposal(cognitionstate.PlanRevisionProposalInput{
		Graph: graph.Graph, Snapshot: prepared.Snapshot, Decision: decision,
		ActionSchema: actionSchema, ProposalIndex: proposalIndex,
		CompletionAuthority: episode.Completion,
	})
	if err != nil {
		return nil, fmt.Errorf("materialize cognition plan revision: %w", err)
	}
	if value.NextGeneration > math.MaxInt64 || value.ExpectedGraphSHA256 != graph.Graph.SHA256 ||
		value.SourceSnapshotSHA256 != prepared.Snapshot.SHA256() {
		return nil, fmt.Errorf("%w: plan revision changed source authority", ErrCognitionConflict)
	}
	copy := value.Clone()
	return &copy, nil
}

func insertCognitionPlanRevisionTx(
	ctx context.Context,
	tx pgx.Tx,
	episode CognitionEpisode,
	graphVersion uint64,
	reconciliationID string,
	ledgerID taskstate.LedgerID,
	candidateEntryID taskstate.EntryID,
	value cognition.PlanRevisionMaterialization,
) error {
	if err := value.Validate(); err != nil || value.NextGeneration > math.MaxInt64 {
		return fmt.Errorf("invalid cognition plan revision materialization: %w", err)
	}
	raw, rawSHA, err := cognitionJSON(value)
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `
		INSERT INTO cognition_plan_revisions (
			plan_revision_id,plan_revision_sha256,episode_id,job_id,generation,step_id,
			reconciliation_id,ledger_id,candidate_entry_id,source_snapshot_sha256,
			source_decision_sha256,source_proposal_sha256,proposal_index,
			expected_graph_version,expected_graph_sha256,active_obligation_id,
			previous_generation,next_generation,root_obligation_id,next_obligation_id,
			result_graph_sha256,descriptor_json,descriptor_json_sha256
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,$20,$21,$22,$23)
	`, value.ID, value.SHA256, episode.EpisodeID, episode.Authority.JobID,
		episode.Authority.Generation, episode.Authority.StepID, reconciliationID, ledgerID,
		candidateEntryID, value.SourceSnapshotSHA256, value.SourceDecisionSHA256,
		value.SourceProposalSHA256, value.ProposalIndex, int64(graphVersion),
		value.ExpectedGraphSHA256, value.ActiveObligationID, int64(value.PreviousGeneration),
		int64(value.NextGeneration), value.Root.ID, value.Next.ID, value.ResultGraphSHA256,
		string(raw), rawSHA)
	if err != nil {
		return fmt.Errorf("insert cognition plan revision: %w", err)
	}
	return insertCognitionGraphMaterializationSourceTx(
		ctx, tx, episode.EpisodeID, reconciliationID, ledgerID, candidateEntryID,
		cognition.ProposalPlanRevision, value.ProposalIndex, value.ID, value.SHA256,
	)
}

func requireCognitionPlanRevisionReplayTx(
	ctx context.Context,
	tx pgx.Tx,
	reconciliationID string,
	want *cognition.PlanRevisionMaterialization,
) error {
	var raw []byte
	var rawSHA string
	err := tx.QueryRow(ctx, `
		SELECT descriptor_json,descriptor_json_sha256
		FROM cognition_plan_revisions WHERE reconciliation_id=$1
	`, reconciliationID).Scan(&raw, &rawSHA)
	if errors.Is(err, pgx.ErrNoRows) {
		if want == nil {
			return nil
		}
		return fmt.Errorf("%w: reconciliation lost its plan revision", ErrCognitionConflict)
	}
	if err != nil {
		return err
	}
	if want == nil {
		return fmt.Errorf("%w: reconciliation gained a plan revision", ErrCognitionConflict)
	}
	wantRaw, wantSHA, err := cognitionJSON(*want)
	if err != nil {
		return err
	}
	if rawSHA != wantSHA || !bytes.Equal(raw, wantRaw) {
		return fmt.Errorf("%w: plan revision replay changed content", ErrCognitionConflict)
	}
	var persisted cognition.PlanRevisionMaterialization
	if json.Unmarshal(raw, &persisted) != nil || persisted.Validate() != nil {
		return fmt.Errorf("%w: persisted plan revision is invalid", ErrCognitionConflict)
	}
	return nil
}

func planRevisionCandidate(
	mutations []cognitionstate.EntryMutation,
	value *cognition.PlanRevisionMaterialization,
) (taskstate.EntryID, error) {
	return graphMaterializationCandidate(
		mutations, value != nil, cognitionstate.SourceModelPlanRevision,
		"plan_revision", func() int {
			if value == nil {
				return -1
			}
			return value.ProposalIndex
		}(),
	)
}
