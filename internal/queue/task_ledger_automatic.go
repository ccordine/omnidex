package queue

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strconv"
	"strings"

	"github.com/gryph/omnidex/internal/model"
	"github.com/gryph/omnidex/internal/taskstate"
	"github.com/jackc/pgx/v5"
)

const (
	initialTaskRootNodeID         taskstate.NodeID  = "goal:root"
	initialUserInstructionEntryID taskstate.EntryID = "entry:user-instruction"
	initialTaskAuthoritySchema                      = "omnidex.initial-task-authority.v1"
	initialTaskLedgerVersion      uint64            = 3
)

func seedInitialTaskAuthorityTx(ctx context.Context, tx pgx.Tx, job model.Job) error {
	goalTitle := telemetryPromptSummary(job.Instruction, 240)
	if goalTitle == "" {
		return fmt.Errorf("job instruction must contain non-whitespace user authority")
	}
	if job.ID <= 0 || job.CurrentGeneration != 1 {
		return fmt.Errorf("initial task authority requires a positive generation-one job")
	}
	addRootID, err := initialTaskCommandID(job, "add-root")
	if err != nil {
		return err
	}
	recordInstructionID, err := initialTaskCommandID(job, "record-user-instruction")
	if err != nil {
		return err
	}
	readyRootID, err := initialTaskCommandID(job, "ready-root")
	if err != nil {
		return err
	}
	commands := []taskstate.Command{
		taskstate.AddNodeCommand{
			CommandID: addRootID, ExpectedVersion: 0,
			Actor: taskstate.AuthorityCode, ID: initialTaskRootNodeID,
			Kind: taskstate.NodeGoal, Title: goalTitle, Priority: 100,
			AcceptanceCriteria: []string{}, Metadata: taskstate.EmptyJSONObject(),
		},
		taskstate.AddEntryCommand{
			CommandID: recordInstructionID, ExpectedVersion: 1,
			Actor: taskstate.AuthorityUser, ID: initialUserInstructionEntryID,
			ScopeNodeID: initialTaskRootNodeID, Kind: taskstate.EntryNote,
			Content: "Current user instruction", Metadata: taskstate.EmptyJSONObject(),
			Refs: []taskstate.Ref{initialInstructionRef(job)},
		},
		taskstate.PromoteReadyNodesCommand{
			CommandID: readyRootID, ExpectedVersion: 2,
			Actor: taskstate.AuthorityCode,
		},
	}
	for _, command := range commands {
		if _, err := applyQueueOwnedTaskCommandTx(ctx, tx, job.ID, job.CurrentGeneration, command); err != nil {
			return fmt.Errorf("seed initial task authority for job %d: %w", job.ID, err)
		}
	}
	return nil
}

func activateInitialTaskRootTx(ctx context.Context, tx pgx.Tx, jobID, generation int64) error {
	header, root, err := loadInitialTaskRootTx(ctx, tx, jobID, generation)
	if err != nil {
		return err
	}
	switch root.Status {
	case taskstate.NodeActive:
		return requireCanonicalRootActivationEventTx(ctx, tx, header, root)
	case taskstate.NodeReady:
	default:
		return fmt.Errorf("%w: initial root goal %q cannot activate from %q", taskstate.ErrInvalidState, root.ID, root.Status)
	}
	commandID, err := initialLifecycleCommandID(jobID, generation, 0, "activate-root")
	if err != nil {
		return err
	}
	_, err = applyQueueOwnedTaskCommandTx(ctx, tx, jobID, generation, taskstate.TransitionNodeCommand{
		CommandID: commandID, ExpectedVersion: header.Version, Actor: taskstate.AuthorityCode,
		NodeID: initialTaskRootNodeID, To: taskstate.NodeActive,
	})
	return err
}

func transitionInitialTaskRootTx(
	ctx context.Context,
	tx pgx.Tx,
	jobID, generation, stepID int64,
	to taskstate.NodeStatus,
	proofContent, reason string,
) error {
	if acceptedIntentTerminalStatus(to) {
		if err := transitionAcceptedIntentObjectiveTx(
			ctx, tx, jobID, generation, stepID, to, proofContent, reason,
		); err != nil {
			return err
		}
	}
	header, root, err := loadInitialTaskRootTx(ctx, tx, jobID, generation)
	if err != nil {
		return err
	}
	if root.Status == to {
		return fmt.Errorf(
			"%w: nonterminal job %d already has root goal status %q",
			taskstate.ErrInvalidState, jobID, root.Status,
		)
	}
	commandID, err := initialLifecycleCommandID(jobID, generation, stepID, "root-"+string(to))
	if err != nil {
		return err
	}
	command := taskstate.TransitionNodeCommand{
		CommandID:       commandID,
		ExpectedVersion: header.Version, Actor: taskstate.AuthorityCode,
		NodeID: initialTaskRootNodeID, To: to, Reason: reason,
	}
	if to == taskstate.NodeDone {
		if stepID <= 0 {
			return fmt.Errorf("root completion requires a positive terminal step")
		}
		command.CompletedStepID = &stepID
		command.VerificationRefs = []taskstate.Ref{stepCompletionRef(jobID, generation, stepID, proofContent)}
	}
	_, err = applyQueueOwnedTaskCommandTx(ctx, tx, jobID, generation, command)
	return err
}

func loadInitialTaskRootTx(
	ctx context.Context, tx pgx.Tx, jobID, generation int64,
) (taskLedgerHeader, taskstate.Node, error) {
	header, err := loadTaskLedgerHeaderTx(ctx, tx, jobID, true)
	if err != nil {
		return taskLedgerHeader{}, taskstate.Node{}, err
	}
	if header.Generation != generation {
		return taskLedgerHeader{}, taskstate.Node{}, fmt.Errorf(
			"%w: initial root observed generation %d, current generation is %d",
			ErrStaleJobGeneration, generation, header.Generation,
		)
	}
	ledger, err := restoreTaskLedgerTx(ctx, tx, header)
	if err != nil {
		return taskLedgerHeader{}, taskstate.Node{}, err
	}
	root, ok := ledger.Node(initialTaskRootNodeID)
	if !ok || root.Kind != taskstate.NodeGoal {
		return taskLedgerHeader{}, taskstate.Node{}, fmt.Errorf(
			"%w: job %d has no canonical initial root goal", taskstate.ErrInvalidState, jobID,
		)
	}
	return header, root, nil
}

func initialTaskCommandID(job model.Job, operation string) (taskstate.CommandID, error) {
	return taskstate.NewCommandID(
		initialTaskAuthoritySchema,
		strconv.FormatInt(job.ID, 10), strconv.FormatInt(job.CurrentGeneration, 10), operation,
	)
}

func initialLifecycleCommandID(jobID, generation, stepID int64, operation string) (taskstate.CommandID, error) {
	return taskstate.NewCommandID(
		initialTaskAuthoritySchema,
		strconv.FormatInt(jobID, 10), strconv.FormatInt(generation, 10),
		strconv.FormatInt(stepID, 10), operation,
	)
}

func initialInstructionRef(job model.Job) taskstate.Ref {
	digest := sha256.Sum256([]byte(job.Instruction))
	return taskstate.Ref{
		URI: fmt.Sprintf("task://job/%d/instruction", job.ID), Version: "v1",
		Hash: hex.EncodeToString(digest[:]), Relation: taskstate.RefSource,
	}
}

func stepCompletionRef(jobID, generation, stepID int64, content string) taskstate.Ref {
	digest := sha256.Sum256([]byte(content))
	return taskstate.Ref{
		URI:     fmt.Sprintf("task://job/%d/generation/%d/step/%d/completion", jobID, generation, stepID),
		Version: "v1", Hash: hex.EncodeToString(digest[:]), Relation: taskstate.RefVerifies,
	}
}

func exactTaskFailureReason(reason string) (string, error) {
	reason = strings.TrimSpace(reason)
	if reason == "" {
		return "", fmt.Errorf("step failure reason is required")
	}
	return reason, nil
}
