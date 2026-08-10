package queue

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strconv"

	"github.com/gryph/omnidex/internal/cognition"
	"github.com/gryph/omnidex/internal/taskstate"
)

func cognitionJSON(value any) ([]byte, string, error) {
	raw, err := json.Marshal(value)
	if err != nil {
		return nil, "", fmt.Errorf("encode cognition authority: %w", err)
	}
	digest := sha256.Sum256(raw)
	return raw, hex.EncodeToString(digest[:]), nil
}

func cognitionTaskCommandID(parts ...string) (taskstate.CommandID, error) {
	return taskstate.NewCommandID(append([]string{cognitionQueueIdentitySchemaV1}, parts...)...)
}

func cognitionEdgeID(episodeID cognition.EpisodeID, from, to cognition.ObligationID, kind taskstate.EdgeKind) taskstate.EdgeID {
	payload := string(episodeID) + "\x00" + string(from) + "\x00" + string(to) + "\x00" + string(kind)
	digest := sha256.Sum256([]byte(payload))
	return taskstate.EdgeID(cognitionObligationEdgePrefix + hex.EncodeToString(digest[:]))
}

func cognitionActionID(episodeID cognition.EpisodeID, revision cognition.WorldRevision, callID string) cognition.ActionID {
	payload := cognitionQueueIdentitySchemaV1 + "\x00" + string(episodeID) + "\x00" +
		strconv.FormatUint(revision.Number, 10) + "\x00" + revision.SHA256 + "\x00" + callID
	digest := sha256.Sum256([]byte(payload))
	return cognition.ActionID("cognition_action_" + hex.EncodeToString(digest[:]))
}

func cognitionTransitionID(episodeID cognition.EpisodeID, transitionSHA string) string {
	digest := sha256.Sum256([]byte(cognitionQueueIdentitySchemaV1 + "\x00" + string(episodeID) + "\x00" + transitionSHA))
	return cognitionTransitionPrefix + hex.EncodeToString(digest[:])
}
