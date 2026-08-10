package queue

import (
	"context"
	"fmt"

	"github.com/gryph/omnidex/internal/cognition"
	"github.com/gryph/omnidex/internal/taskstate"
	"github.com/jackc/pgx/v5"
)

func planRevisionTaskMetadata(
	episode CognitionEpisode,
	value cognition.PlanRevisionMaterialization,
) (taskstate.JSONObject, error) {
	raw, _, err := cognitionJSON(struct {
		Schema         string              `json:"schema"`
		EpisodeID      cognition.EpisodeID `json:"episode_id"`
		JobGeneration  int64               `json:"job_generation"`
		PlanGeneration uint64              `json:"plan_generation"`
		PlanRevisionID string              `json:"plan_revision_id"`
	}{cognitionQueueIdentitySchemaV1, episode.EpisodeID, episode.Authority.Generation,
		value.NextGeneration, value.ID})
	if err != nil {
		return taskstate.JSONObject{}, err
	}
	return taskstate.NewJSONObject(raw)
}

func addPlanRevisionTaskNode(
	spec cognition.ObligationSpec,
	parent taskstate.NodeID,
	priority int,
	episode CognitionEpisode,
	metadata taskstate.JSONObject,
	version uint64,
	commandID cognitionTaskCommandIDFactory,
	apply cognitionTaskApply,
) error {
	id, err := commandID("add-" + string(spec.ID))
	if err != nil {
		return err
	}
	stepID := episode.Authority.StepID
	return apply("node", taskstate.AddNodeCommand{
		CommandID: id, ExpectedVersion: version, Actor: taskstate.AuthorityCode,
		ID: taskstate.NodeID(spec.ID), ParentID: parent, Kind: taskstate.NodeObjective,
		Title: "Cognition obligation " + string(spec.ID), Priority: priority,
		CreatedStepID: &stepID,
		AcceptanceCriteria: []string{
			"completion-check:" + string(spec.CompletionCheck.ID) + "@" + spec.CompletionCheck.Version,
		}, Metadata: metadata,
	})
}

func addPlanRevisionTaskEdge(
	from, to cognition.ObligationID,
	kind taskstate.EdgeKind,
	episode CognitionEpisode,
	version uint64,
	commandID cognitionTaskCommandIDFactory,
	apply cognitionTaskApply,
) error {
	id, err := commandID("edge-" + string(kind) + "-" + string(from) + "-" + string(to))
	if err != nil {
		return err
	}
	return apply("edge", taskstate.AddEdgeCommand{
		CommandID: id, ExpectedVersion: version, Actor: taskstate.AuthorityCode,
		ID: cognitionEdgeID(episode.EpisodeID, from, to, kind), Kind: kind,
		From: taskstate.NodeID(from), To: taskstate.NodeID(to),
	})
}

func promoteAndActivatePlanRevisionNode(
	node cognition.ObligationID,
	version *uint64,
	commandID cognitionTaskCommandIDFactory,
	applyEvent func(string, taskstate.Command) (taskstate.Event, error),
	apply cognitionTaskApply,
) error {
	promoteID, err := commandID("ready-" + string(node))
	if err != nil {
		return err
	}
	event, err := applyEvent("readiness", taskstate.PromoteReadyNodesCommand{
		CommandID: promoteID, ExpectedVersion: *version, Actor: taskstate.AuthorityCode,
	})
	if err != nil {
		return err
	}
	found := false
	for _, id := range event.NodeIDs {
		found = found || id == taskstate.NodeID(node)
	}
	if !found {
		return fmt.Errorf("%w: revised obligation was not deterministically readied", ErrCognitionConflict)
	}
	activateID, err := commandID("activate-" + string(node))
	if err != nil {
		return err
	}
	return apply("activation", taskstate.TransitionNodeCommand{
		CommandID: activateID, ExpectedVersion: *version, Actor: taskstate.AuthorityCode,
		NodeID: taskstate.NodeID(node), To: taskstate.NodeActive,
	})
}

func requirePlanRevisionTaskProjectionTx(
	ctx context.Context,
	tx pgx.Tx,
	ledgerID taskstate.LedgerID,
	value cognition.PlanRevisionMaterialization,
	retired []taskstate.NodeID,
) error {
	var rootStatus, nextStatus taskstate.NodeStatus
	if err := tx.QueryRow(ctx, `
		SELECT root.status,next.status FROM task_nodes root,task_nodes next
		WHERE root.ledger_id=$1 AND root.id=$2 AND next.ledger_id=$1 AND next.id=$3
	`, ledgerID, value.Root.ID, value.Next.ID).Scan(&rootStatus, &nextStatus); err != nil {
		return err
	}
	if rootStatus != taskstate.NodeBlocked || nextStatus != taskstate.NodeActive {
		return fmt.Errorf("%w: revised Task Ledger lifecycle diverged", ErrCognitionConflict)
	}
	var count int
	if err := tx.QueryRow(ctx, `
		SELECT COUNT(*) FROM task_node_generation_supersessions
		WHERE ledger_id=$1 AND retiring_generation=$2 AND superseded_at_generation=$3
	`, ledgerID, int64(value.PreviousGeneration), int64(value.NextGeneration)).Scan(&count); err != nil {
		return err
	}
	if count != len(retired) {
		return fmt.Errorf("%w: revised Task Ledger supersession set changed", ErrCognitionConflict)
	}
	return nil
}
