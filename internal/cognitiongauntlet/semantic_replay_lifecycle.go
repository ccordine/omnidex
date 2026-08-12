package cognitiongauntlet

import (
	"fmt"

	"github.com/gryph/omnidex/internal/cognitionreplay"
	"github.com/gryph/omnidex/internal/queue"
)

func (state *semanticReplayState) mapLifecycleRetirement(
	record queue.CognitionSealedTraceRecord,
	source cognitionreplay.SourceRecord,
) ([]semanticEventDraft, error) {
	if state.lifecycleRetirement != nil {
		return nil, fmt.Errorf("semantic lifecycle retirement is duplicated")
	}
	copy := record
	copy.Payload = append([]byte(nil), record.Payload...)
	state.lifecycleRetirement = &copy
	draft := sourceKnowledgeDraft(
		cognitionreplay.EventLeaseChanged, source,
		cognitionreplay.KnowledgeFailure, cognitionreplay.KnowledgeResolved,
		cognitionreplay.AuthorityCode,
	)
	draft.Knowledge.Ref = "lifecycle-retirement://" + record.ID
	return []semanticEventDraft{draft}, nil
}
