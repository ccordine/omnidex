package queue

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"strings"

	"github.com/gryph/omnidex/internal/model"
	"github.com/jackc/pgx/v5"
)

func transitionCodingAuthorityTx(
	ctx context.Context,
	tx pgx.Tx,
	jobID int64,
	target model.CodingPlanState,
	ownerTurnID string,
	ownerTurnText string,
) error {
	var ledgerID, currentRevision int64
	var frozenRevision *int64
	var state string
	err := tx.QueryRow(ctx, `
		SELECT id,current_revision,frozen_revision,state
		FROM coding_ledgers
		WHERE job_id=$1
		FOR UPDATE
	`, jobID).Scan(&ledgerID, &currentRevision, &frozenRevision, &state)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("lock coding ledger for job %d lifecycle transition: %w", jobID, err)
	}
	if state == "completed" || state == "cancelled" {
		return fmt.Errorf("%w: coding ledger for job %d is already %s", ErrCodingLedgerState, jobID, state)
	}

	switch target {
	case model.CodingPlanStateSuperseded:
		if state != "frozen" {
			return nil
		}
		result, updateErr := tx.Exec(ctx, `
			UPDATE coding_ledgers
			SET state='revising',frozen_revision=NULL,updated_at=clock_timestamp()
			WHERE id=$1 AND current_revision=$2 AND state='frozen'
		`, ledgerID, currentRevision)
		if updateErr != nil {
			return fmt.Errorf("open coding ledger %d for revision: %w", ledgerID, updateErr)
		}
		if result.RowsAffected() != 1 {
			return fmt.Errorf("%w: coding ledger %d changed during lifecycle transition", ErrCodingLedgerRevision, ledgerID)
		}
		return nil

	case model.CodingPlanStateCanceled:
		if err := validateCodingLedgerExternalOwnerTurnID(ownerTurnID); err != nil {
			return err
		}
		if strings.TrimSpace(ownerTurnText) == "" {
			return fmt.Errorf("coding ledger cancellation requires exact owner text")
		}
		if err := insertCodingLedgerOwnerTurnTx(
			ctx, tx, ledgerID, ownerTurnID, currentRevision, "cancel", ownerTurnText,
		); err != nil {
			return err
		}
		lastFrozenRevision := int64(0)
		if frozenRevision != nil {
			lastFrozenRevision = *frozenRevision
		} else if err := tx.QueryRow(ctx, `
			SELECT COALESCE(max(revision),0)
			FROM coding_ledger_revisions
			WHERE ledger_id=$1 AND revision<=$2 AND state='frozen'
		`, ledgerID, currentRevision).Scan(&lastFrozenRevision); err != nil {
			return fmt.Errorf("read last frozen revision for coding ledger %d: %w", ledgerID, err)
		}
		if lastFrozenRevision > 0 {
			if _, err := tx.Exec(ctx, `
				UPDATE coding_agent_tasks
				SET state='cancelled',state_version=state_version+1,
				    accepted_at=NULL,cancelled_at=clock_timestamp(),updated_at=clock_timestamp()
				WHERE ledger_id=$1 AND frozen_revision=$2 AND state IN ('blocked','ready')
			`, ledgerID, lastFrozenRevision); err != nil {
				return fmt.Errorf("cancel unfinished coding ledger tasks: %w", err)
			}
		}
		var frozenValue any
		if lastFrozenRevision > 0 {
			frozenValue = lastFrozenRevision
		}
		result, updateErr := tx.Exec(ctx, `
			UPDATE coding_ledgers
			SET state='cancelled',frozen_revision=$3,updated_at=clock_timestamp()
			WHERE id=$1 AND current_revision=$2 AND state IN ('proposed','frozen','revising')
		`, ledgerID, currentRevision, frozenValue)
		if updateErr != nil {
			return fmt.Errorf("cancel coding ledger %d: %w", ledgerID, updateErr)
		}
		if result.RowsAffected() != 1 {
			return fmt.Errorf("%w: coding ledger %d changed during cancellation", ErrCodingLedgerRevision, ledgerID)
		}
		return nil
	}
	return fmt.Errorf("coding ledger lifecycle target %q is unsupported", target)
}

func (r *Repository) FreezeCodingLedger(ctx context.Context, command FreezeCodingLedgerCommand) (CodingLedgerRevision, error) {
	if r == nil || r.pool == nil || command.LedgerID <= 0 || command.Revision <= 0 ||
		strings.TrimSpace(command.OperationID) == "" || strings.TrimSpace(command.OwnerTurnID) == "" ||
		strings.TrimSpace(command.OwnerTurnText) == "" {
		return CodingLedgerRevision{}, fmt.Errorf("coding ledger freeze requires exact ledger, revision, operation, and owner-turn authority")
	}
	if err := validateCodingLedgerOperationID(command.OperationID); err != nil {
		return CodingLedgerRevision{}, err
	}
	if err := validateCodingLedgerExternalOwnerTurnID(command.OwnerTurnID); err != nil {
		return CodingLedgerRevision{}, err
	}
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return CodingLedgerRevision{}, err
	}
	defer tx.Rollback(ctx)
	var current int64
	var state string
	if err := tx.QueryRow(ctx, `SELECT current_revision,state FROM coding_ledgers WHERE id=$1 FOR UPDATE`, command.LedgerID).Scan(&current, &state); err != nil {
		return CodingLedgerRevision{}, err
	}
	var replay, replayBase int64
	var replayOwner, replayText, replayState string
	err = tx.QueryRow(ctx, `
		SELECT revision.revision,revision.base_revision,revision.owner_turn_id,turn_record.exact_text,revision.state
		FROM coding_ledger_revisions revision
		JOIN coding_ledger_owner_turns turn_record
		  ON turn_record.ledger_id=revision.ledger_id AND turn_record.owner_turn_id=revision.owner_turn_id
		WHERE revision.ledger_id=$1 AND revision.operation_id=$2
	`, command.LedgerID, command.OperationID).Scan(&replay, &replayBase, &replayOwner, &replayText, &replayState)
	if err == nil {
		if replayState != "frozen" || replayBase != command.Revision || replayOwner != command.OwnerTurnID || replayText != command.OwnerTurnText {
			return CodingLedgerRevision{}, fmt.Errorf("coding ledger operation %q was already used by a different freeze", command.OperationID)
		}
		result, readErr := readCodingLedgerRevisionTx(ctx, tx, command.LedgerID, replay, CodingLedgerReadOptions{Limit: -1})
		if readErr != nil {
			return CodingLedgerRevision{}, readErr
		}
		if err := tx.Commit(ctx); err != nil {
			return CodingLedgerRevision{}, err
		}
		result.Applied = false
		return result, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return CodingLedgerRevision{}, err
	}
	if state != "proposed" || current != command.Revision {
		return CodingLedgerRevision{}, fmt.Errorf("%w: displayed revision %d is stale; current revision is %d in state %s",
			ErrCodingLedgerRevision, command.Revision, current, state)
	}
	proposal, err := readCodingLedgerRevisionTx(ctx, tx, command.LedgerID, current, CodingLedgerReadOptions{Limit: -1})
	if err != nil {
		return CodingLedgerRevision{}, err
	}
	for _, record := range ledgerRepositoryRecords(proposal.Ledger) {
		if record.item.Disposition == model.LedgerDispositionActive && record.item.ReviewState != model.LedgerReviewAgreed {
			return CodingLedgerRevision{}, fmt.Errorf("%w: active item %s is not agreed", ErrCodingLedgerState, record.item.ID)
		}
	}
	for _, dependency := range proposal.Ledger.Dependencies {
		if dependency.Disposition == model.LedgerDispositionActive && dependency.ReviewState != model.LedgerReviewAgreed {
			return CodingLedgerRevision{}, fmt.Errorf("%w: active dependency %s is not agreed", ErrCodingLedgerState, dependency.ID)
		}
	}
	if err := validateLedgerForRepository(proposal.Ledger); err != nil {
		return CodingLedgerRevision{}, err
	}
	if err := validateCodingLedgerRoutes(proposal.Routes, current); err != nil {
		return CodingLedgerRevision{}, err
	}
	var routesChangedSinceFreeze bool
	if err := tx.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1
			FROM coding_ledger_routes current_route
			JOIN LATERAL (
				SELECT prior_route.provider,prior_route.model,
				       prior_route.reasoning_effort,prior_route.service_tier
				FROM coding_ledger_revisions prior_frozen
				JOIN coding_ledger_routes prior_route
				  ON prior_route.ledger_id=prior_frozen.ledger_id
				 AND prior_route.revision=prior_frozen.revision
				 AND prior_route.role=current_route.role
				WHERE prior_frozen.ledger_id=current_route.ledger_id
				  AND prior_frozen.revision<current_route.revision
				  AND prior_frozen.state='frozen'
				ORDER BY prior_frozen.revision DESC
				LIMIT 1
			) prior_route ON true
			WHERE current_route.ledger_id=$1 AND current_route.revision=$2
			  AND ROW(current_route.provider,current_route.model,
			          current_route.reasoning_effort,current_route.service_tier)
			      IS DISTINCT FROM
			      ROW(prior_route.provider,prior_route.model,
			          prior_route.reasoning_effort,prior_route.service_tier)
		)
	`, command.LedgerID, current).Scan(&routesChangedSinceFreeze); err != nil {
		return CodingLedgerRevision{}, fmt.Errorf("inspect coding ledger route authority: %w", err)
	}
	if err := validateFrozenCodingAgentTasks(proposal.Ledger); err != nil {
		return CodingLedgerRevision{}, fmt.Errorf("%w: coding ledger cannot produce valid task packets: %v", ErrCodingLedgerState, err)
	}

	frozenRevision := current + 1
	ownerAction := "sign_off"
	if routesChangedSinceFreeze {
		// The frozen revision must be owned by the execution-configuration
		// approval, while the same exact command also supplies the required
		// sign-off record for its displayed base revision.
		ownerAction = "execution_configuration"
		if err := insertCodingLedgerOwnerTurnTx(ctx, tx, command.LedgerID,
			codingLedgerSignOffTurnID(command.OwnerTurnID), current, "sign_off", command.OwnerTurnText); err != nil {
			return CodingLedgerRevision{}, err
		}
	}
	if err := insertCodingLedgerOwnerTurnTx(ctx, tx, command.LedgerID, command.OwnerTurnID,
		current, ownerAction, command.OwnerTurnText); err != nil {
		return CodingLedgerRevision{}, err
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO coding_ledger_revisions(
			ledger_id,revision,base_revision,operation_id,owner_turn_id,state
		) VALUES($1,$2,$3,$4,$5,'frozen')
	`, command.LedgerID, frozenRevision, current, command.OperationID, command.OwnerTurnID); err != nil {
		return CodingLedgerRevision{}, fmt.Errorf("create frozen coding ledger revision: %w", err)
	}
	frozenLedger := proposal.Ledger
	frozenLedger.Revision = frozenRevision
	routes := append([]CodingLedgerRoute(nil), proposal.Routes...)
	for index := range routes {
		routes[index].ConfigurationRevision = proposal.Routes[index].ConfigurationRevision
	}
	if err := persistCodingLedgerSnapshotTx(ctx, tx, command.LedgerID, frozenRevision, current,
		command.OwnerTurnID, frozenLedger, routes); err != nil {
		return CodingLedgerRevision{}, err
	}
	if err := cancelRemovedCodingAgentTasksTx(ctx, tx, command.LedgerID, frozenRevision); err != nil {
		return CodingLedgerRevision{}, err
	}
	if routesChangedSinceFreeze {
		represented := model.OwnerTurnDisposition{Statements: []model.OwnerStatementDisposition{{Kind: model.OwnerTurnRepresented}}}
		for _, record := range ledgerRepositoryRecords(frozenLedger) {
			if record.item.Disposition == model.LedgerDispositionActive {
				represented.Statements[0].ItemIDs = append(represented.Statements[0].ItemIDs, record.item.ID)
			}
		}
		if err := insertCodingLedgerDispositionTx(ctx, tx, command.LedgerID, frozenRevision,
			command.OwnerTurnID, represented, frozenLedger); err != nil {
			return CodingLedgerRevision{}, err
		}
	}
	update, err := tx.Exec(ctx, `
		UPDATE coding_ledgers
		SET frozen_revision=$2,state='frozen',updated_at=clock_timestamp()
		WHERE id=$1 AND current_revision=$2 AND state='proposed'
	`, command.LedgerID, frozenRevision)
	if err != nil {
		return CodingLedgerRevision{}, fmt.Errorf("freeze coding ledger: %w", err)
	}
	if update.RowsAffected() != 1 {
		return CodingLedgerRevision{}, fmt.Errorf("%w: coding ledger changed during freeze", ErrCodingLedgerRevision)
	}
	if err := createFrozenCodingAgentTasksTx(ctx, tx, command.LedgerID, frozenRevision, frozenLedger); err != nil {
		return CodingLedgerRevision{}, err
	}
	result, err := readCodingLedgerRevisionTx(ctx, tx, command.LedgerID, frozenRevision, CodingLedgerReadOptions{Limit: -1})
	if err != nil {
		return CodingLedgerRevision{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return CodingLedgerRevision{}, err
	}
	result.Applied = true
	return result, nil
}

func createFrozenCodingAgentTasksTx(ctx context.Context, tx pgx.Tx, ledgerID, revision int64, ledger model.EngineeringLedger) error {
	continuations, err := codingContinuationPrioritiesTx(ctx, tx, ledgerID, ledger)
	if err != nil {
		return err
	}
	prerequisites := map[model.LedgerItemID][]model.LedgerItemID{}
	for _, dependency := range ledger.Dependencies {
		if dependency.Disposition == model.LedgerDispositionActive {
			prerequisites[dependency.TaskID] = append(prerequisites[dependency.TaskID], dependency.PrerequisiteID)
		}
	}
	priority := func(value model.LedgerPriority) int {
		switch value {
		case model.LedgerPriorityCritical:
			return 400
		case model.LedgerPriorityHigh:
			return 300
		case model.LedgerPriorityNormal:
			return 200
		default:
			return 100
		}
	}
	carried := make(map[model.LedgerItemID]bool)
	var priorRevision int64
	if err := tx.QueryRow(ctx, `
		SELECT COALESCE(max(revision),0)
		FROM coding_ledger_revisions
		WHERE ledger_id=$1 AND revision<$2 AND state='frozen'
	`, ledgerID, revision).Scan(&priorRevision); err != nil {
		return fmt.Errorf("read prior frozen coding ledger revision: %w", err)
	}
	if priorRevision > 0 {
		prior, err := readCodingLedgerRevisionTx(ctx, tx, ledgerID, priorRevision, CodingLedgerReadOptions{Limit: -1})
		if err != nil {
			return err
		}
		for _, task := range ledger.Tasks {
			if task.Disposition != model.LedgerDispositionActive {
				continue
			}
			var accepted, contractMatches bool
			if err := tx.QueryRow(ctx, `
				SELECT EXISTS (
					SELECT 1 FROM coding_agent_tasks
					WHERE ledger_id=$1 AND frozen_revision=$2 AND task_item_id=$3 AND state='accepted'
				), coding_agent_task_contract_matches($1,$2,$4,$3)
			`, ledgerID, priorRevision, task.ID, revision).Scan(&accepted, &contractMatches); err != nil {
				return fmt.Errorf("inspect prior coding task %s: %w", task.ID, err)
			}
			if !accepted || !contractMatches {
				continue
			}
			priorTask, priorErr := codingAgentTaskFromLedger(prior.Ledger, task.ID)
			currentTask, currentErr := codingAgentTaskFromLedger(ledger, task.ID)
			if priorErr == nil && currentErr == nil && reflect.DeepEqual(priorTask, currentTask) {
				carried[task.ID] = true
			}
		}
		for changed := true; changed; {
			changed = false
			for taskID := range carried {
				for _, prerequisiteID := range prerequisites[taskID] {
					if !carried[prerequisiteID] {
						delete(carried, taskID)
						changed = true
						break
					}
				}
			}
		}
	}

	inserted := make(map[model.LedgerItemID]bool, len(carried))
	for len(inserted) < len(carried) {
		advanced := false
		for _, task := range ledger.Tasks {
			if !carried[task.ID] || inserted[task.ID] {
				continue
			}
			ready := true
			for _, prerequisiteID := range prerequisites[task.ID] {
				ready = ready && inserted[prerequisiteID]
			}
			if !ready {
				continue
			}
			if _, err := tx.Exec(ctx, `
				INSERT INTO coding_agent_tasks(
					ledger_id,frozen_revision,task_item_id,state,priority,accepted_at
				) VALUES($1,$2,$3,'accepted',$4,clock_timestamp())
			`, ledgerID, revision, task.ID, priority(task.Priority)); err != nil {
				return fmt.Errorf("carry accepted frozen coding task %s: %w", task.ID, err)
			}
			inserted[task.ID] = true
			advanced = true
		}
		if !advanced {
			return fmt.Errorf("carried coding tasks contain an unresolved dependency cycle")
		}
	}

	for _, task := range ledger.Tasks {
		if task.Disposition != model.LedgerDispositionActive {
			continue
		}
		if carried[task.ID] {
			continue
		}
		state := "ready"
		for _, prerequisiteID := range prerequisites[task.ID] {
			if !carried[prerequisiteID] {
				state = "blocked"
				break
			}
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO coding_agent_tasks(ledger_id,frozen_revision,task_item_id,state,priority,continuation_priority)
			VALUES($1,$2,$3,$4,$5,$6)
		`, ledgerID, revision, task.ID, state, priority(task.Priority), continuations[task.ID]); err != nil {
			return fmt.Errorf("project frozen coding task %s: %w", task.ID, err)
		}
	}
	return nil
}

func cancelRemovedCodingAgentTasksTx(ctx context.Context, tx pgx.Tx, ledgerID, revision int64) error {
	var priorRevision int64
	if err := tx.QueryRow(ctx, `
		SELECT COALESCE(max(revision),0)
		FROM coding_ledger_revisions
		WHERE ledger_id=$1 AND revision<$2 AND state='frozen'
	`, ledgerID, revision).Scan(&priorRevision); err != nil {
		return fmt.Errorf("read prior frozen revision for removed tasks: %w", err)
	}
	if priorRevision == 0 {
		return nil
	}
	if _, err := tx.Exec(ctx, `
		UPDATE coding_agent_tasks task
		SET state='cancelled',state_version=task.state_version+1,
		    accepted_at=NULL,cancelled_at=clock_timestamp(),updated_at=clock_timestamp()
		FROM coding_ledger_revision_items projection
		JOIN coding_ledger_item_versions version
		  ON (version.ledger_id,version.item_id,version.revision)=
		     (projection.ledger_id,projection.item_id,projection.item_revision)
		WHERE task.ledger_id=$1 AND task.frozen_revision=$2
		  AND task.state IN ('blocked','ready')
		  AND projection.ledger_id=task.ledger_id
		  AND projection.revision=$3
		  AND projection.item_id=task.task_item_id
		  AND version.disposition='removed'
	`, ledgerID, priorRevision, revision); err != nil {
		return fmt.Errorf("cancel removed coding ledger tasks: %w", err)
	}
	return nil
}
