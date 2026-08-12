package queue

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"reflect"

	"github.com/gryph/omnidex/internal/cognition"
	"github.com/gryph/omnidex/internal/exactjson"
	"github.com/jackc/pgx/v5"
)

func loadCognitionAcceptedFactMaterializationTracePayloadTx(
	ctx context.Context,
	tx pgx.Tx,
	episode CognitionEpisode,
	record cognitionTraceRecord,
) ([]byte, error) {
	var raw, transitionRaw []byte
	var payloadSHA, transitionSHA string
	if err := tx.QueryRow(ctx, `
		SELECT batch.payload_json,batch.payload_json_sha256,
		       transitions.transition_json,transitions.transition_sha256
		FROM cognition_accepted_fact_materializations batch
		JOIN cognition_transitions transitions ON transitions.transition_id=batch.transition_id
		WHERE batch.episode_id=$1 AND batch.materialization_id=$2
		  AND batch.transition_revision=$3 AND batch.call_ordinal=$4
	`, episode.EpisodeID, record.ID, record.Sequence, record.CallOrdinal).Scan(
		&raw, &payloadSHA, &transitionRaw, &transitionSHA,
	); err != nil {
		return nil, fmt.Errorf("load sealed cognition accepted-fact materialization: %w", err)
	}
	value, err := DecodeCognitionAcceptedFactMaterialization(raw, payloadSHA)
	if err != nil || payloadSHA != record.SHA256 || transitionSHA != value.TransitionSHA256 {
		return nil, fmt.Errorf("%w: sealed accepted-fact materialization changed: %v", ErrCognitionConflict, err)
	}
	wantPhase := CognitionAcceptedFactMaterializationActionTracePhase
	if value.ActionID == "" {
		wantPhase = CognitionAcceptedFactMaterializationInitialTracePhase
	}
	if record.Phase != wantPhase || int64(value.CallOrdinal) != record.CallOrdinal ||
		int64(value.TransitionRevision) != record.Sequence || value.ID != record.ID {
		return nil, fmt.Errorf("%w: sealed accepted-fact materialization tuple changed", ErrCognitionConflict)
	}
	var transition cognition.Transition
	if json.Unmarshal(transitionRaw, &transition) != nil ||
		transition.ActionID != value.ActionID || transition.Current.Number != value.TransitionRevision {
		return nil, fmt.Errorf("%w: accepted-fact materialization transition changed", ErrCognitionConflict)
	}
	wantTransitionRaw, wantTransitionSHA, err := cognitionJSON(transition)
	if err != nil || !bytes.Equal(transitionRaw, wantTransitionRaw) || wantTransitionSHA != transitionSHA {
		return nil, fmt.Errorf("%w: accepted-fact materialization transition is not exact", ErrCognitionConflict)
	}
	preLedger, err := loadTaskLedgerAtVersionTx(
		ctx, tx, episode.Authority.JobID, value.PreFactLedgerVersion,
	)
	if err != nil || !reflect.DeepEqual(preLedger, value.PreFactLedger) {
		return nil, fmt.Errorf("%w: accepted-fact materialization pre-ledger changed: %v", ErrCognitionConflict, err)
	}
	canonical, err := exactjson.Canonical(value)
	if err != nil || !bytes.Equal(raw, canonical) {
		return nil, fmt.Errorf("%w: accepted-fact materialization payload is not canonical", ErrCognitionConflict)
	}
	return append([]byte(nil), raw...), nil
}
