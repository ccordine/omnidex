package queue

import (
	"context"
	"fmt"
	"math"

	"github.com/gryph/omnidex/internal/cognition"
	"github.com/gryph/omnidex/internal/taskstate"
	"github.com/jackc/pgx/v5"
)

func createCognitionRootObligationTx(
	ctx context.Context,
	tx pgx.Tx,
	header taskLedgerHeader,
	command CognitionEpisodeStart,
) error {
	metadataJSON, _, err := cognitionJSON(struct {
		Schema         string              `json:"schema"`
		EpisodeID      cognition.EpisodeID `json:"episode_id"`
		JobGeneration  int64               `json:"job_generation"`
		PlanGeneration uint64              `json:"plan_generation"`
	}{cognitionQueueIdentitySchemaV1, command.EpisodeID, command.Authority.Generation,
		cognition.InitialObligationGeneration})
	if err != nil {
		return err
	}
	metadata, err := taskstate.NewJSONObject(metadataJSON)
	if err != nil {
		return err
	}
	nodeID := taskstate.NodeID(command.Root.ID)
	addID, err := cognitionTaskCommandID(string(command.EpisodeID), string(nodeID), "add")
	if err != nil {
		return err
	}
	if _, err := applyQueueOwnedTaskCommandTx(ctx, tx, command.Authority.JobID, command.Authority.Generation,
		taskstate.AddNodeCommand{
			CommandID: addID, ExpectedVersion: header.Version, Actor: taskstate.AuthorityCode,
			ID: nodeID, ParentID: initialTaskRootNodeID, Kind: taskstate.NodeObjective,
			Title: "Cognition obligation " + string(command.Root.ID), Priority: 100,
			CreatedStepID: &command.Authority.StepID,
			AcceptanceCriteria: []string{
				"completion-check:" + string(command.Root.CompletionCheck.ID) + "@" + command.Root.CompletionCheck.Version,
			}, Metadata: metadata,
		}); err != nil {
		return fmt.Errorf("create cognition root obligation: %w", err)
	}
	header.Version++
	edgeID := cognitionEdgeID(command.EpisodeID, "goal:root", command.Root.ID, taskstate.EdgeDecomposes)
	edgeCommandID, err := cognitionTaskCommandID(string(command.EpisodeID), string(edgeID), "decompose")
	if err != nil {
		return err
	}
	if _, err := applyQueueOwnedTaskCommandTx(ctx, tx, command.Authority.JobID, command.Authority.Generation,
		taskstate.AddEdgeCommand{
			CommandID: edgeCommandID, ExpectedVersion: header.Version, Actor: taskstate.AuthorityCode,
			ID: edgeID, Kind: taskstate.EdgeDecomposes, From: initialTaskRootNodeID, To: nodeID,
		}); err != nil {
		return fmt.Errorf("link cognition root obligation: %w", err)
	}
	header.Version++
	readyID, err := cognitionTaskCommandID(string(command.EpisodeID), "promote-root")
	if err != nil {
		return err
	}
	if _, err := applyQueueOwnedTaskCommandTx(ctx, tx, command.Authority.JobID, command.Authority.Generation,
		taskstate.PromoteReadyNodesCommand{
			CommandID: readyID, ExpectedVersion: header.Version, Actor: taskstate.AuthorityCode,
		}); err != nil {
		return fmt.Errorf("ready cognition root obligation: %w", err)
	}
	header.Version++
	activeID, err := cognitionTaskCommandID(string(command.EpisodeID), "activate-root")
	if err != nil {
		return err
	}
	if _, err := applyQueueOwnedTaskCommandTx(ctx, tx, command.Authority.JobID, command.Authority.Generation,
		taskstate.TransitionNodeCommand{
			CommandID: activeID, ExpectedVersion: header.Version, Actor: taskstate.AuthorityCode,
			NodeID: nodeID, To: taskstate.NodeActive,
		}); err != nil {
		return fmt.Errorf("activate cognition root obligation: %w", err)
	}
	return nil
}

func insertCognitionObligationProjectionTx(
	ctx context.Context,
	tx pgx.Tx,
	command CognitionEpisodeStart,
	ledgerID taskstate.LedgerID,
) error {
	return insertCognitionObligationProjectionRecordTx(
		ctx, tx, command.EpisodeID, command.Authority.JobID,
		command.Authority.Generation, cognition.InitialObligationGeneration, ledgerID, command.Root,
	)
}

func insertCognitionObligationProjectionRecordTx(
	ctx context.Context,
	tx pgx.Tx,
	episodeID cognition.EpisodeID,
	jobID, jobGeneration int64,
	planGeneration uint64,
	ledgerID taskstate.LedgerID,
	spec cognition.ObligationSpec,
) error {
	if jobGeneration <= 0 || planGeneration == 0 || planGeneration > math.MaxInt64 {
		return fmt.Errorf("cognition obligation generations exceed PostgreSQL authority")
	}
	desiredJSON, desiredSHA, err := cognitionJSON(spec.Desired)
	if err != nil {
		return err
	}
	refsJSON, refsSHA, err := cognitionJSON(spec.SupportingRefs)
	if err != nil {
		return err
	}
	var createdVersion int64
	if err := tx.QueryRow(ctx, `
		SELECT created_version FROM task_nodes WHERE ledger_id=$1 AND id=$2
	`, ledgerID, spec.ID).Scan(&createdVersion); err != nil {
		return fmt.Errorf("load cognition obligation node version: %w", err)
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO cognition_obligations (
			episode_id,ledger_id,job_id,job_generation,node_id,parent_node_id,created_generation,
			desired_json,desired_sha256,completion_check_id,completion_check_version,
			completion_check_sha256,supporting_refs_json,supporting_refs_sha256,created_version
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15)
	`, episodeID, ledgerID, jobID, jobGeneration, spec.ID, nullableCognitionParent(spec.ParentID),
		int64(planGeneration), string(desiredJSON), desiredSHA,
		spec.CompletionCheck.ID, spec.CompletionCheck.Version,
		spec.CompletionCheck.SHA256, string(refsJSON), refsSHA, createdVersion); err != nil {
		return fmt.Errorf("insert cognition obligation %q: %w", spec.ID, err)
	}
	if err := insertCognitionObligationSupportingRefsTx(
		ctx, tx, episodeID, spec.ID, spec.SupportingRefs,
	); err != nil {
		return err
	}
	return insertCognitionObligationDependenciesTx(
		ctx, tx, episodeID, spec.ID, spec.DependsOn,
	)
}

func nullableCognitionParent(parent cognition.ObligationID) any {
	if parent == "" {
		return nil
	}
	return parent
}
