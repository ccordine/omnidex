package queue

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/gryph/omnidex/internal/cognition"
	"github.com/gryph/omnidex/internal/cognitionruntime"
	"github.com/gryph/omnidex/internal/taskstate"
	"github.com/jackc/pgx/v5"
)

func loadCognitionActionPlanRevisionTx(
	ctx context.Context,
	tx pgx.Tx,
	record CognitionActionRecord,
) (cognitionActionGraphMaterialization, bool, error) {
	var raw []byte
	var rawSHA string
	var graphVersion int64
	var candidate taskstate.EntryID
	err := tx.QueryRow(ctx, `
		SELECT descriptor_json,descriptor_json_sha256,expected_graph_version,candidate_entry_id
		FROM cognition_plan_revisions WHERE reconciliation_id=$1
	`, record.ReconciliationID).Scan(&raw, &rawSHA, &graphVersion, &candidate)
	if errors.Is(err, pgx.ErrNoRows) {
		return cognitionActionGraphMaterialization{}, false, nil
	}
	if err != nil {
		return cognitionActionGraphMaterialization{}, false, err
	}
	if graphVersion <= 0 || candidate == "" || cognitionPayloadSHA(raw) != rawSHA {
		return cognitionActionGraphMaterialization{}, false,
			fmt.Errorf("%w: plan revision persistence changed", ErrCognitionConflict)
	}
	var value cognition.PlanRevisionMaterialization
	if json.Unmarshal(raw, &value) != nil || value.Validate() != nil {
		return cognitionActionGraphMaterialization{}, false,
			fmt.Errorf("%w: plan revision persistence is invalid", ErrCognitionConflict)
	}
	canonical, _, err := cognitionJSON(value)
	if err != nil || !bytes.Equal(canonical, raw) {
		return cognitionActionGraphMaterialization{}, false,
			fmt.Errorf("%w: plan revision persistence is not canonical", ErrCognitionConflict)
	}
	decisionSHA, err := cognitionruntime.DecisionSHA256(record.Decision)
	if err != nil || value.EpisodeID != record.EpisodeID ||
		value.SourceSnapshotSHA256 != record.SnapshotSHA256 ||
		value.SourceDecisionSHA256 != decisionSHA || value.ActiveObligationID != record.ObligationID {
		return cognitionActionGraphMaterialization{}, false,
			fmt.Errorf("%w: plan revision differs from its action", ErrCognitionConflict)
	}
	copy := value.Clone()
	return cognitionActionGraphMaterialization{
		Kind: cognition.ProposalPlanRevision, Graph: uint64(graphVersion),
		Candidate: candidate, Revision: &copy,
	}, true, nil
}
