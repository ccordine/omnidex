package queue

import (
	"strconv"

	"github.com/gryph/omnidex/internal/cognition"
	"github.com/gryph/omnidex/internal/taskstate"
)

type cognitionTaskApply func(string, taskstate.Command) error
type cognitionTaskCommandIDFactory func(string) (taskstate.CommandID, error)

func persistSatisfiedCognitionObligationTx(
	episode CognitionEpisode,
	result cognition.CompletionResult,
	before, after cognition.ObligationGraphSnapshot,
	version *uint64,
	apply cognitionTaskApply,
	commandID cognitionTaskCommandIDFactory,
) error {
	id, err := commandID("satisfy")
	if err != nil {
		return err
	}
	stepID := episode.Authority.StepID
	if err := apply("satisfaction", taskstate.TransitionNodeCommand{
		CommandID: id, ExpectedVersion: *version, Actor: taskstate.AuthorityCode,
		NodeID: taskstate.NodeID(result.ObligationID), To: taskstate.NodeDone,
		CompletedStepID: &stepID, VerificationRefs: cognitionEvidenceTaskRefs(result.EvidenceRefs),
	}); err != nil {
		return err
	}
	return persistCognitionReadyDiff(before, after, version, apply, commandID)
}

func cognitionEvidenceTaskRefs(refs []cognition.EvidenceRef) []taskstate.Ref {
	result := make([]taskstate.Ref, len(refs))
	for index, ref := range refs {
		result[index] = taskstate.Ref{
			URI: "cognition:episode/" + string(ref.Revision.EpisodeID) + "/observation/" +
				string(ref.ObservationID),
			Version: strconv.FormatUint(ref.Revision.Number, 10), Hash: ref.SHA256,
			Relation: taskstate.RefEvidence,
		}
	}
	return result
}

func persistCognitionReadyDiff(
	before, after cognition.ObligationGraphSnapshot,
	version *uint64,
	apply cognitionTaskApply,
	commandID cognitionTaskCommandIDFactory,
) error {
	statuses := make(map[cognition.ObligationID]cognition.ObligationStatus, len(before.Obligations))
	for _, obligation := range before.Obligations {
		statuses[obligation.ID] = obligation.Status
	}
	promotePending := false
	for _, obligation := range after.Obligations {
		if obligation.Status != cognition.ObligationReady || statuses[obligation.ID] == cognition.ObligationReady {
			continue
		}
		if statuses[obligation.ID] != cognition.ObligationBlocked {
			promotePending = true
			continue
		}
		id, err := commandID("unblock-" + string(obligation.ID))
		if err != nil {
			return err
		}
		if err := apply("dependency resolution", taskstate.TransitionNodeCommand{
			CommandID: id, ExpectedVersion: *version, Actor: taskstate.AuthorityCode,
			NodeID: taskstate.NodeID(obligation.ID), To: taskstate.NodeReady,
			Reason: "Every registered cognition dependency is satisfied.",
		}); err != nil {
			return err
		}
	}
	if !promotePending {
		return nil
	}
	id, err := commandID("promote-ready")
	if err != nil {
		return err
	}
	return apply("dependent readiness", taskstate.PromoteReadyNodesCommand{
		CommandID: id, ExpectedVersion: *version, Actor: taskstate.AuthorityCode,
	})
}
