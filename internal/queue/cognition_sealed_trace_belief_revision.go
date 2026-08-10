package queue

import (
	"bytes"
	"encoding/json"
	"fmt"

	"github.com/gryph/omnidex/internal/cognitionstate"
)

const cognitionBeliefRevisionTraceQuery = `
	SELECT revisions.descriptor_json,revisions.descriptor_json_sha256
	FROM cognition_belief_revisions revisions
	JOIN cognition_runtime_snapshots snapshots
	  ON snapshots.snapshot_sha256=revisions.source_snapshot_sha256
	WHERE revisions.episode_id=$1 AND revisions.revision_id=$2
	  AND revisions.expected_ledger_version=$3
`

func validateCognitionBeliefRevisionTracePayload(raw []byte, sha256 string) error {
	if !json.Valid(raw) || cognitionPayloadSHA(raw) != sha256 {
		return fmt.Errorf("%w: sealed cognition belief revision payload changed", ErrCognitionConflict)
	}
	var value cognitionstate.BeliefRevisionMaterialization
	if err := json.Unmarshal(raw, &value); err != nil || value.Validate() != nil {
		return fmt.Errorf("%w: sealed cognition belief revision payload is invalid", ErrCognitionConflict)
	}
	canonical, err := json.Marshal(value)
	if err != nil || !bytes.Equal(canonical, raw) {
		return fmt.Errorf("%w: sealed cognition belief revision is not canonical", ErrCognitionConflict)
	}
	return nil
}
