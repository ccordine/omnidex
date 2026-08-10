package queue

import (
	"context"
	"fmt"
	"sort"

	"github.com/gryph/omnidex/internal/cognition"
	"github.com/gryph/omnidex/internal/taskstate"
	"github.com/jackc/pgx/v5"
)

func persistCognitionPlanRevisionTaskTx(
	ctx context.Context,
	tx pgx.Tx,
	header taskLedgerHeader,
	episode CognitionEpisode,
	value cognition.PlanRevisionMaterialization,
	before, after cognition.ObligationGraphSnapshot,
) (taskLedgerHeader, error) {
	version := header.Version
	applyEvent := func(label string, command taskstate.Command) (taskstate.Event, error) {
		event, err := applyQueueOwnedTaskCommandTx(
			ctx, tx, episode.Authority.JobID, episode.Authority.Generation, command,
		)
		if err != nil {
			return taskstate.Event{}, fmt.Errorf("persist cognition plan revision %s: %w", label, err)
		}
		version = event.Version
		return event, nil
	}
	apply := func(label string, command taskstate.Command) error {
		_, err := applyEvent(label, command)
		return err
	}
	commandID := func(suffix string) (taskstate.CommandID, error) {
		return cognitionTaskCommandID(value.ID, suffix)
	}
	retired := revisedCognitionTaskNodes(before, after)
	retireID, err := commandID("supersede-previous-plan")
	if err != nil {
		return header, err
	}
	if err := apply("supersession", taskstate.SupersedeNodeGenerationCommand{
		CommandID: retireID, ExpectedVersion: version, Actor: taskstate.AuthorityCode,
		RetiringGeneration:     int64(value.PreviousGeneration),
		SupersededAtGeneration: int64(value.NextGeneration), NodeIDs: retired,
		Reason: fmt.Sprintf("Cognition plan generation %d superseded generation %d.",
			value.NextGeneration, value.PreviousGeneration),
	}); err != nil {
		return header, err
	}
	metadata, err := planRevisionTaskMetadata(episode, value)
	if err != nil {
		return header, err
	}
	if err := addPlanRevisionTaskNode(
		value.Root, initialTaskRootNodeID, 100, episode, metadata, version, commandID, apply,
	); err != nil {
		return header, err
	}
	if err := addPlanRevisionTaskEdge(
		cognition.ObligationID(initialTaskRootNodeID), value.Root.ID,
		taskstate.EdgeDecomposes, episode, version, commandID, apply,
	); err != nil {
		return header, err
	}
	if err := promoteAndActivatePlanRevisionNode(
		value.Root.ID, &version, commandID, applyEvent, apply,
	); err != nil {
		return header, err
	}
	if err := addPlanRevisionTaskNode(
		value.Next, taskstate.NodeID(value.Root.ID), 90, episode, metadata, version, commandID, apply,
	); err != nil {
		return header, err
	}
	if err := addPlanRevisionTaskEdge(
		value.Root.ID, value.Next.ID, taskstate.EdgeDecomposes, episode, version, commandID, apply,
	); err != nil {
		return header, err
	}
	if err := addPlanRevisionTaskEdge(
		value.Root.ID, value.Next.ID, taskstate.EdgeDependsOn, episode, version, commandID, apply,
	); err != nil {
		return header, err
	}
	blockID, err := commandID("block-root")
	if err != nil {
		return header, err
	}
	if err := apply("root block", taskstate.TransitionNodeCommand{
		CommandID: blockID, ExpectedVersion: version, Actor: taskstate.AuthorityCode,
		NodeID: taskstate.NodeID(value.Root.ID), To: taskstate.NodeBlocked,
		Reason: "The code-authorized revised prerequisite is active.",
	}); err != nil {
		return header, err
	}
	if err := promoteAndActivatePlanRevisionNode(
		value.Next.ID, &version, commandID, applyEvent, apply,
	); err != nil {
		return header, err
	}
	if err := insertCognitionObligationProjectionRecordTx(
		ctx, tx, episode.EpisodeID, episode.Authority.JobID, episode.Authority.Generation,
		value.NextGeneration, header.ID, value.Root,
	); err != nil {
		return header, err
	}
	if err := insertCognitionObligationProjectionRecordTx(
		ctx, tx, episode.EpisodeID, episode.Authority.JobID, episode.Authority.Generation,
		value.NextGeneration, header.ID, value.Next,
	); err != nil {
		return header, err
	}
	if err := requirePlanRevisionTaskProjectionTx(ctx, tx, header.ID, value, retired); err != nil {
		return header, err
	}
	header.Version = version
	return header, nil
}

func revisedCognitionTaskNodes(
	before, after cognition.ObligationGraphSnapshot,
) []taskstate.NodeID {
	afterByID := make(map[cognition.ObligationID]cognition.Obligation, len(after.Obligations))
	for _, obligation := range after.Obligations {
		afterByID[obligation.ID] = obligation
	}
	result := make([]taskstate.NodeID, 0)
	for _, obligation := range before.Obligations {
		if next := afterByID[obligation.ID]; next.Status == cognition.ObligationSuperseded &&
			obligation.Status != cognition.ObligationSuperseded {
			result = append(result, taskstate.NodeID(obligation.ID))
		}
	}
	sort.Slice(result, func(i, j int) bool { return result[i] < result[j] })
	return result
}
