package queue

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"reflect"
	"strings"

	"github.com/gryph/omnidex/internal/codingagent"
	"github.com/gryph/omnidex/internal/model"
	"github.com/jackc/pgx/v5"
)

func (repository *Repository) CodingLedgerCompletionCounts(
	ctx context.Context,
	authority model.StepAttemptAuthority,
) (int, int, error) {
	if repository == nil || repository.pool == nil {
		return 0, 0, fmt.Errorf("coding ledger completion counts require repository authority")
	}
	tx, err := repository.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return 0, 0, err
	}
	defer tx.Rollback(ctx)
	jobStatus, stepStatus, _, err := requireActiveStepAttemptTx(ctx, tx, authority)
	if err != nil {
		return 0, 0, err
	}
	if jobStatus != model.JobStatusRunning || stepStatus != model.StepStatusRunning {
		return 0, 0, staleStepAttemptError(authority, "coding ledger completion counts require current execution", nil)
	}
	var action string
	if err := tx.QueryRow(ctx, `
		SELECT action FROM job_steps
		WHERE id=$1 AND job_id=$2 AND generation=$3
	`, authority.StepID, authority.JobID, authority.Generation).Scan(&action); err != nil {
		return 0, 0, err
	}
	if action != "v3_coding" {
		return 0, 0, fmt.Errorf("coding ledger completion counts require the execution step")
	}
	var ledgerID, revision int64
	if err := tx.QueryRow(ctx, `
		SELECT id,frozen_revision
		FROM coding_ledgers
		WHERE job_id=$1 AND state='frozen' AND frozen_revision IS NOT NULL
	`, authority.JobID).Scan(&ledgerID, &revision); err != nil {
		return 0, 0, err
	}
	var taskCount, acceptedCount int
	if err := tx.QueryRow(ctx, `
		SELECT count(*),count(*) FILTER (WHERE state='accepted')
		FROM coding_agent_tasks
		WHERE ledger_id=$1 AND frozen_revision=$2
	`, ledgerID, revision).Scan(&taskCount, &acceptedCount); err != nil {
		return 0, 0, err
	}
	if taskCount < 1 || acceptedCount != taskCount {
		return 0, 0, fmt.Errorf("coding ledger completion counts require every frozen task accepted")
	}
	projection, err := readCodingLedgerRevisionTx(ctx, tx, ledgerID, revision, CodingLedgerReadOptions{Limit: -1})
	if err != nil {
		return 0, 0, err
	}
	checkCount := 0
	for _, ledgerTask := range projection.Ledger.Tasks {
		if ledgerTask.Disposition != model.LedgerDispositionActive {
			continue
		}
		task, err := codingAgentTaskFromLedger(projection.Ledger, ledgerTask.ID)
		if err != nil {
			return 0, 0, err
		}
		checkCount += len(task.Checks)
	}
	if err := tx.Commit(ctx); err != nil {
		return 0, 0, err
	}
	return taskCount, checkCount, nil
}

// Completion validates the durable journal inside the same transaction that
// terminalizes the current attempt. Agent claims alone cannot close the job.
func requireCodingAgentCompletionTx(ctx context.Context, tx pgx.Tx, job model.Job, stepID int64) error {
	metadataConfiguration, err := codingagent.ConfigurationFromMetadata(job.Metadata)
	if err != nil {
		return err
	}
	if metadataConfiguration.Mode != codingagent.Agents {
		return nil
	}
	var action string
	if err := tx.QueryRow(ctx, `SELECT action FROM job_steps WHERE id=$1 AND job_id=$2`, stepID, job.ID).Scan(&action); err != nil {
		return err
	}
	if action != "v3_coding" {
		return nil
	}
	entries, err := codingAgentCompletionEntriesTx(ctx, tx, job.ID, job.CurrentGeneration)
	if err != nil {
		return err
	}
	var ledgerID, frozenRevision int64
	var ledgerState string
	err = tx.QueryRow(ctx, `
		SELECT id,state,frozen_revision
		FROM coding_ledgers
		WHERE job_id=$1
	`, job.ID).Scan(&ledgerID, &ledgerState, &frozenRevision)
	if errors.Is(err, pgx.ErrNoRows) {
		return fmt.Errorf("agent completion requires one frozen PostgreSQL coding ledger")
	}
	if err != nil {
		return err
	}
	if ledgerState != "frozen" || frozenRevision < 1 {
		return fmt.Errorf("agent completion requires the current frozen task ledger")
	}
	configuration, err := frozenCodingLedgerConfigurationTx(ctx, tx, job.ID)
	if err != nil {
		return err
	}
	projection, err := readCodingLedgerRevisionTx(ctx, tx, ledgerID, frozenRevision, CodingLedgerReadOptions{Limit: -1})
	if err != nil {
		return err
	}
	tasks := make(map[string]codingagent.Task)
	for _, ledgerTask := range projection.Ledger.Tasks {
		if ledgerTask.Disposition != model.LedgerDispositionActive {
			continue
		}
		task, err := codingAgentTaskFromLedger(projection.Ledger, ledgerTask.ID)
		if err != nil {
			return err
		}
		tasks[task.ID] = *task
	}
	durablyAccepted := make(map[string]bool, len(tasks))
	rows, err := tx.Query(ctx, `
		SELECT task_item_id,state
		FROM coding_agent_tasks
		WHERE ledger_id=$1 AND frozen_revision=$2
	`, ledgerID, frozenRevision)
	if err != nil {
		return err
	}
	for rows.Next() {
		var taskID, state string
		if err := rows.Scan(&taskID, &state); err != nil {
			rows.Close()
			return err
		}
		if _, known := tasks[taskID]; !known {
			rows.Close()
			return fmt.Errorf("coding task ledger contains unknown task %s", taskID)
		}
		durablyAccepted[taskID] = state == "accepted"
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return err
	}
	rows.Close()
	if len(durablyAccepted) != len(tasks) {
		return fmt.Errorf("coding task ledger does not project every active task")
	}
	if err := validateCodingTaskLedgerCompletionJournal(
		entries, job.CurrentGeneration, configuration, tasks, durablyAccepted,
	); err != nil {
		return err
	}
	return materializeCodingTaskLedgerCompletionTx(ctx, tx, ledgerID, frozenRevision, entries)
}

func materializeCodingTaskLedgerCompletionTx(
	ctx context.Context,
	tx pgx.Tx,
	ledgerID, revision int64,
	_ []codingagent.Entry,
) error {
	var incomplete bool
	if err := tx.QueryRow(ctx, `SELECT EXISTS(
		SELECT 1 FROM coding_agent_tasks
		WHERE ledger_id=$1 AND frozen_revision=$2 AND state<>'accepted'
	)`, ledgerID, revision).Scan(&incomplete); err != nil {
		return err
	}
	if incomplete {
		return fmt.Errorf("coding task ledger completion requires every queued task accepted")
	}
	result, err := tx.Exec(ctx, `
		UPDATE coding_ledgers
		SET state='completed',updated_at=clock_timestamp()
		WHERE id=$1 AND frozen_revision=$2 AND state='frozen'
	`, ledgerID, revision)
	if err != nil {
		return err
	}
	if result.RowsAffected() != 1 {
		return fmt.Errorf("coding task ledger changed before completion")
	}
	return nil
}

func codingAgentCompletionEntriesTx(ctx context.Context, tx pgx.Tx, jobID, generation int64) ([]codingagent.Entry, error) {
	rows, err := tx.Query(ctx, `SELECT id,payload_json-'frame' FROM evidence WHERE job_id=$1 AND kind='coding_agent' AND payload_json->>'generation'=$2 ORDER BY id`, jobID, fmt.Sprint(generation))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var entries []codingagent.Entry
	for rows.Next() {
		var entry codingagent.Entry
		var raw []byte
		var id int64
		if err := rows.Scan(&id, &raw); err != nil {
			return nil, err
		}
		if err := json.Unmarshal(raw, &entry); err != nil {
			return nil, err
		}
		entry.ID = id
		entries = append(entries, entry)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return entries, nil
}

func validateCodingTaskLedgerCompletionJournal(
	entries []codingagent.Entry,
	generation int64,
	configuration codingagent.Configuration,
	tasks map[string]codingagent.Task,
	durablyAccepted map[string]bool,
) error {
	if len(tasks) == 0 {
		return fmt.Errorf("agent completion requires frozen task ledger work")
	}
	accepted := map[string]bool{}
	opened := map[int64]codingagent.Entry{}
	completed := map[int64]codingagent.Entry{}
	native := map[int64]bool{}
	touched := map[string]bool{}
	for _, entry := range entries {
		if entry.Generation != generation {
			return fmt.Errorf("agent completion journal changed generation")
		}
		if entry.TaskID != "" {
			if _, known := tasks[entry.TaskID]; !known {
				return fmt.Errorf("agent completion journal refers to unknown task %s", entry.TaskID)
			}
			touched[entry.TaskID] = true
		}
		switch entry.Phase {
		case "call_opened":
			opened[entry.ID] = entry
		case "native_event":
			native[entry.CallID] = true
		case "call_completed":
			completed[entry.CallID] = entry
		case "questions", "answers":
			return fmt.Errorf("frozen execution journal cannot contain owner-question routing")
		case "task_accepted":
			task, known := tasks[entry.TaskID]
			if !known {
				return fmt.Errorf("agent acceptance refers to an unknown task")
			}
			if err := codingAgentAcceptedCall(entry, codingagent.Review, configuration.Reviewer, opened, completed, native); err != nil {
				return err
			}
			if err := codingAgentAcceptedReviewResponse(entry, configuration.LedgerParser, opened, completed, native, entries); err != nil {
				return err
			}
			reviewOpening := opened[entry.CallID]
			if reviewOpening.Assignment == nil || reviewOpening.Assignment.Task == nil ||
				!reflect.DeepEqual(*reviewOpening.Assignment.Task, task) ||
				reviewOpening.Assignment.ValidateTaskContext() != nil {
				return fmt.Errorf("agent task review changed its exact frozen ledger context")
			}
			executionThread, ok := codingAgentCompletedTaskExecutionBefore(
				task, entry.CallID, configuration.Executor, opened, completed, native, entries,
			)
			if !ok {
				return fmt.Errorf("agent task review lacks a completed isolated execution")
			}
			if reviewCompletion := completed[entry.CallID]; reviewCompletion.Result == nil ||
				reviewCompletion.Result.ThreadID == executionThread {
				return fmt.Errorf("agent task execution and review must use isolated channels")
			}
			if !codingAgentChecksMatch(task.Checks, entry.Checks) {
				return fmt.Errorf("agent task acceptance lacks successful frozen checks")
			}
			for _, dependency := range task.Dependencies {
				if !accepted[dependency] {
					return fmt.Errorf("agent task acceptance precedes its dependency")
				}
			}
			accepted[entry.TaskID] = true
		case "task_reviewed":
			if _, known := tasks[entry.TaskID]; !known {
				return fmt.Errorf("agent journal reopened an unknown task")
			}
			accepted[entry.TaskID] = false
		}
	}
	for taskID := range tasks {
		if !accepted[taskID] && (!durablyAccepted[taskID] || touched[taskID]) {
			return fmt.Errorf("agent completion contains unfinished task %s", taskID)
		}
	}
	return nil
}

func validateCodingTaskAcceptanceJournal(
	entries []codingagent.Entry,
	generation int64,
	configuration codingagent.Configuration,
	task codingagent.Task,
) error {
	opened := map[int64]codingagent.Entry{}
	completed := map[int64]codingagent.Entry{}
	native := map[int64]bool{}
	var accepted *codingagent.Entry
	for index := range entries {
		entry := entries[index]
		if entry.Generation != generation {
			return fmt.Errorf("agent task acceptance journal changed generation")
		}
		switch entry.Phase {
		case "call_opened":
			opened[entry.ID] = entry
		case "native_event":
			native[entry.CallID] = true
		case "call_completed":
			completed[entry.CallID] = entry
		case "questions", "answers":
			return fmt.Errorf("frozen execution journal cannot contain owner-question routing")
		case "task_accepted":
			if entry.TaskID == task.ID {
				copy := entry
				accepted = &copy
			}
		case "task_reviewed":
			if entry.TaskID == task.ID {
				accepted = nil
			}
		}
	}
	if accepted == nil {
		return fmt.Errorf("coding ledger task %s lacks accepted review evidence", task.ID)
	}
	if err := codingAgentAcceptedCall(*accepted, codingagent.Review, configuration.Reviewer, opened, completed, native); err != nil {
		return err
	}
	if err := codingAgentAcceptedReviewResponse(*accepted, configuration.LedgerParser, opened, completed, native, entries); err != nil {
		return err
	}
	reviewOpening := opened[accepted.CallID]
	if reviewOpening.Assignment == nil || reviewOpening.Assignment.Task == nil ||
		!reflect.DeepEqual(*reviewOpening.Assignment.Task, task) || reviewOpening.Assignment.ValidateTaskContext() != nil {
		return fmt.Errorf("agent task review changed its exact frozen ledger context")
	}
	executionThread, ok := codingAgentCompletedTaskExecutionBefore(
		task, accepted.CallID, configuration.Executor, opened, completed, native, entries,
	)
	if !ok {
		return fmt.Errorf("agent task review lacks a completed isolated execution")
	}
	if reviewCompletion := completed[accepted.CallID]; reviewCompletion.Result == nil ||
		reviewCompletion.Result.ThreadID == executionThread {
		return fmt.Errorf("agent task execution and review must use isolated channels")
	}
	if !codingAgentChecksMatch(task.Checks, accepted.Checks) {
		return fmt.Errorf("agent task acceptance lacks successful frozen checks")
	}
	return nil
}

func codingAgentCompletedTaskExecutionBefore(
	task codingagent.Task,
	beforeCallID int64,
	route codingagent.Route,
	opened map[int64]codingagent.Entry,
	completed map[int64]codingagent.Entry,
	native map[int64]bool,
	entries []codingagent.Entry,
) (string, bool) {
	var selected int64
	var thread string
	parsedDone := map[int64]bool{}
	for _, entry := range entries {
		if entry.Phase == "execution_response" && entry.ID < beforeCallID {
			parsedDone[entry.CallID] = entry.ResponseKind == codingagent.ExecutionDone
		}
	}
	for callID, completion := range completed {
		opening, known := opened[callID]
		if !known || callID >= beforeCallID || callID <= selected || !native[callID] || !parsedDone[callID] ||
			opening.Role != codingagent.Execute || completion.Role != codingagent.Execute ||
			opening.TaskID != task.ID || completion.TaskID != task.ID || opening.Assignment == nil ||
			opening.Assignment.Task == nil || !reflect.DeepEqual(*opening.Assignment.Task, task) ||
			opening.Assignment.Route != route || completion.Result == nil {
			continue
		}
		result := completion.Result
		if !result.Completed || result.Provider != route.Provider || result.Model != route.Model ||
			result.ThreadID == "" || result.TurnID == "" || !json.Valid(result.Content) {
			continue
		}
		var execution string
		if err := decodeCodingAgentJournalResult(result.Content, &execution); err != nil || strings.TrimSpace(execution) == "" {
			continue
		}
		selected, thread = callID, result.ThreadID
	}
	return thread, selected != 0
}

func codingAgentChecksMatch(checks []codingagent.Check, results []codingagent.CheckResult) bool {
	if len(checks) != len(results) {
		return false
	}
	expected := map[string]bool{}
	for _, check := range checks {
		expected[check.ID] = true
	}
	for _, result := range results {
		if !expected[result.ID] || result.ExitCode != 0 {
			return false
		}
		delete(expected, result.ID)
	}
	return len(expected) == 0
}

func codingAgentAcceptedCall(entry codingagent.Entry, role codingagent.Role, route codingagent.Route, opened, completed map[int64]codingagent.Entry, native map[int64]bool) error {
	opening, openedOK := opened[entry.CallID]
	completion, completedOK := completed[entry.CallID]
	if !openedOK || !completedOK || !native[entry.CallID] || entry.CallID <= 0 || entry.CallID >= entry.ID || completion.ID >= entry.ID || opening.Role != role || completion.Role != role || opening.TaskID != entry.TaskID || completion.TaskID != entry.TaskID || opening.Assignment == nil || opening.Assignment.Role != role || opening.Assignment.Route != route || completion.Result == nil {
		return fmt.Errorf("agent acceptance lacks its exact independent native call")
	}
	result := completion.Result
	if result.CallID != entry.CallID || !result.Completed || result.Provider != route.Provider || result.Model != route.Model || result.ThreadID == "" || result.TurnID == "" || !json.Valid(result.Content) {
		return fmt.Errorf("agent acceptance has incomplete or unbound native result")
	}
	return nil
}

func decodeCodingAgentJournalResult(raw json.RawMessage, target any) error {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		return fmt.Errorf("coding agent journal result contains trailing data")
	}
	return nil
}
