package queue

import (
	"context"
	"fmt"
	"strings"

	"github.com/gryph/omnidex/internal/codingagent"
	"github.com/gryph/omnidex/internal/model"
	"github.com/jackc/pgx/v5"
)

func (repository *Repository) FrozenCodingLedgerConfiguration(
	ctx context.Context,
	authority model.StepAttemptAuthority,
) (codingagent.Configuration, error) {
	if repository == nil || repository.pool == nil {
		return codingagent.Configuration{}, fmt.Errorf("frozen coding ledger configuration requires repository authority")
	}
	// The queue's active-attempt authority check intentionally locks the exact
	// attempt. Route selection must share that authoritative transaction rather
	// than weakening it to a non-locking read.
	tx, err := repository.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return codingagent.Configuration{}, err
	}
	defer tx.Rollback(ctx)
	jobStatus, stepStatus, _, err := requireActiveStepAttemptTx(ctx, tx, authority)
	if err != nil {
		return codingagent.Configuration{}, err
	}
	if jobStatus != model.JobStatusRunning || stepStatus != model.StepStatusRunning {
		return codingagent.Configuration{}, staleStepAttemptError(authority, "coding ledger route selection requires current execution", nil)
	}
	var action string
	if err := tx.QueryRow(ctx, `
		SELECT action FROM job_steps
		WHERE id=$1 AND job_id=$2 AND generation=$3
	`, authority.StepID, authority.JobID, authority.Generation).Scan(&action); err != nil {
		return codingagent.Configuration{}, err
	}
	if action != "v3_coding" {
		return codingagent.Configuration{}, fmt.Errorf("frozen coding ledger routes require the execution step")
	}
	configuration, err := frozenCodingLedgerConfigurationTx(ctx, tx, authority.JobID)
	if err != nil {
		return codingagent.Configuration{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return codingagent.Configuration{}, err
	}
	return configuration, nil
}

func frozenCodingLedgerConfigurationTx(ctx context.Context, tx pgx.Tx, jobID int64) (codingagent.Configuration, error) {
	rows, err := tx.Query(ctx, `
		SELECT route.role,route.provider,route.model,route.reasoning_effort,route.service_tier
		FROM coding_ledgers ledger
		JOIN coding_ledger_routes route
		  ON route.ledger_id=ledger.id AND route.revision=ledger.frozen_revision
		WHERE ledger.job_id=$1 AND ledger.state='frozen' AND ledger.frozen_revision IS NOT NULL
		ORDER BY route.role
	`, jobID)
	if err != nil {
		return codingagent.Configuration{}, err
	}
	defer rows.Close()
	configuration := codingagent.Configuration{Mode: codingagent.Agents}
	count := 0
	for rows.Next() {
		var role string
		var route codingagent.Route
		if err := rows.Scan(&role, &route.Provider, &route.Model, &route.Effort, &route.ServiceTier); err != nil {
			return codingagent.Configuration{}, err
		}
		switch role {
		case "planner":
			configuration.Planner = route
		case "executor":
			configuration.Executor = route
		case "reviewer":
			configuration.Reviewer = route
		default:
			return codingagent.Configuration{}, fmt.Errorf("frozen coding ledger has unknown route %q", role)
		}
		count++
	}
	if err := rows.Err(); err != nil {
		return codingagent.Configuration{}, err
	}
	if count != 3 {
		return codingagent.Configuration{}, fmt.Errorf("frozen coding ledger requires exactly three routes")
	}
	return configuration, configuration.Validate()
}

// NextIncompleteCodingLedgerTask is the execution scheduler for a frozen task
// ledger. The database chooses one unfinished task whose prerequisites have
// already been accepted. The caller executes and independently reviews only
// the returned task, records task_accepted, and asks again.
func (repository *Repository) NextIncompleteCodingLedgerTask(
	ctx context.Context,
	authority model.StepAttemptAuthority,
) (*codingagent.Task, error) {
	if repository == nil || repository.pool == nil {
		return nil, fmt.Errorf("next coding ledger task requires repository authority")
	}
	tx, err := repository.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)
	jobStatus, stepStatus, _, err := requireActiveStepAttemptTx(ctx, tx, authority)
	if err != nil {
		return nil, err
	}
	if jobStatus != model.JobStatusRunning || stepStatus != model.StepStatusRunning {
		return nil, staleStepAttemptError(authority, "coding ledger task selection requires current execution", nil)
	}
	var action string
	if err := tx.QueryRow(ctx, `
		SELECT action FROM job_steps
		WHERE id=$1 AND job_id=$2 AND generation=$3
	`, authority.StepID, authority.JobID, authority.Generation).Scan(&action); err != nil {
		return nil, err
	}
	if action != "v3_coding" {
		return nil, fmt.Errorf("coding ledger task selection requires the execution step")
	}
	var ledgerID, revision int64
	var taskID model.LedgerItemID
	err = tx.QueryRow(ctx, `
		SELECT ledger.id,ledger.frozen_revision,task.task_item_id
		FROM coding_ledgers ledger
		JOIN coding_agent_tasks task
		  ON task.ledger_id=ledger.id
		 AND task.frozen_revision=ledger.frozen_revision
		JOIN coding_ledger_revision_items projection
		  ON projection.ledger_id=task.ledger_id
		 AND projection.revision=task.frozen_revision
		 AND projection.item_id=task.task_item_id
		JOIN coding_ledger_item_versions version
		  ON version.ledger_id=projection.ledger_id
		 AND version.item_id=projection.item_id
		 AND version.revision=projection.item_revision
		WHERE ledger.job_id=$1
		  AND ledger.state='frozen'
		  AND ledger.frozen_revision IS NOT NULL
		  AND task.state='ready'
		ORDER BY version.rank,task.fairness_key
		LIMIT 1
	`, authority.JobID).Scan(&ledgerID, &revision, &taskID)
	if err != nil {
		if err == pgx.ErrNoRows {
			var incomplete bool
			if err := tx.QueryRow(ctx, `
				SELECT EXISTS(
				    SELECT 1
				    FROM coding_ledgers ledger
				    JOIN coding_agent_tasks task
				      ON task.ledger_id=ledger.id
				     AND task.frozen_revision=ledger.frozen_revision
				    WHERE ledger.job_id=$1
				      AND ledger.state='frozen'
				      AND task.state NOT IN ('accepted','cancelled')
				)
			`, authority.JobID).Scan(&incomplete); err != nil {
				return nil, err
			}
			if incomplete {
				return nil, fmt.Errorf("coding task ledger has incomplete tasks but none are dependency-ready")
			}
			if err := tx.Commit(ctx); err != nil {
				return nil, err
			}
			return nil, nil
		}
		return nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	detail, err := repository.completeCodingLedgerTaskDetail(ctx, ledgerID, revision, taskID)
	if err != nil {
		return nil, err
	}
	return codingAgentTaskFromLedgerDetail(detail)
}

func (repository *Repository) AcceptCodingLedgerTask(
	ctx context.Context,
	authority model.StepAttemptAuthority,
	taskID string,
) error {
	if repository == nil || repository.pool == nil || strings.TrimSpace(taskID) == "" {
		return fmt.Errorf("coding ledger task acceptance requires repository and task authority")
	}
	tx, err := repository.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	jobStatus, stepStatus, _, err := requireActiveStepAttemptTx(ctx, tx, authority)
	if err != nil {
		return err
	}
	if jobStatus != model.JobStatusRunning || stepStatus != model.StepStatusRunning {
		return staleStepAttemptError(authority, "coding ledger task acceptance requires current execution", nil)
	}
	var action string
	if err := tx.QueryRow(ctx, `
		SELECT action FROM job_steps
		WHERE id=$1 AND job_id=$2 AND generation=$3
	`, authority.StepID, authority.JobID, authority.Generation).Scan(&action); err != nil {
		return err
	}
	if action != "v3_coding" {
		return fmt.Errorf("coding ledger task acceptance requires the execution step")
	}
	var ledgerID, revision int64
	var state string
	err = tx.QueryRow(ctx, `
		SELECT task.ledger_id,task.frozen_revision,task.state
		FROM coding_agent_tasks task
		JOIN coding_ledgers ledger ON ledger.id=task.ledger_id AND ledger.frozen_revision=task.frozen_revision
		WHERE ledger.job_id=$1 AND ledger.state='frozen' AND task.task_item_id=$2
		FOR UPDATE OF task
	`, authority.JobID, taskID).Scan(&ledgerID, &revision, &state)
	if err != nil {
		return err
	}
	if state == "accepted" {
		return tx.Commit(ctx)
	}
	if state != "ready" {
		return fmt.Errorf("coding ledger task %s is %s instead of ready", taskID, state)
	}
	detail, err := readCodingLedgerRevisionTx(ctx, tx, ledgerID, revision, CodingLedgerReadOptions{
		Limit: -1, RelatedTaskID: model.LedgerItemID(taskID),
	})
	if err != nil {
		return err
	}
	task, err := codingAgentTaskFromLedgerDetail(detail)
	if err != nil {
		return err
	}
	configuration, err := frozenCodingLedgerConfigurationTx(ctx, tx, authority.JobID)
	if err != nil {
		return err
	}
	entries, err := codingAgentCompletionEntriesTx(ctx, tx, authority.JobID, authority.Generation)
	if err != nil {
		return err
	}
	if err := validateCodingTaskAcceptanceJournal(entries, authority.Generation, configuration, *task); err != nil {
		return err
	}
	result, err := tx.Exec(ctx, `
		UPDATE coding_agent_tasks
		SET state='accepted',state_version=state_version+1,
		    accepted_at=clock_timestamp(),cancelled_at=NULL,updated_at=clock_timestamp()
		WHERE ledger_id=$1 AND frozen_revision=$2 AND task_item_id=$3 AND state='ready'
	`, ledgerID, revision, taskID)
	if err != nil {
		return err
	}
	if result.RowsAffected() != 1 {
		return fmt.Errorf("coding ledger task %s changed before acceptance", taskID)
	}
	return tx.Commit(ctx)
}

func (repository *Repository) completeCodingLedgerTaskDetail(
	ctx context.Context,
	ledgerID, revision int64,
	taskID model.LedgerItemID,
) (CodingLedgerRevision, error) {
	const pageSize = 1000
	var complete CodingLedgerRevision
	for offset := 0; ; offset += pageSize {
		page, err := repository.CodingLedgerTaskDetail(ctx, ledgerID, revision, taskID, CodingLedgerPage{
			Limit: pageSize, Offset: offset,
		})
		if err != nil {
			return CodingLedgerRevision{}, err
		}
		if offset == 0 {
			complete = page
			complete.Ledger.Requirements = nil
			complete.Ledger.Constraints = nil
			complete.Ledger.Groups = nil
			complete.Ledger.AcceptanceCriteria = nil
			complete.Ledger.Checks = nil
			complete.Ledger.DecisionsAndQuestions = nil
			complete.Ledger.Dependencies = nil
			complete.Ledger.Findings = nil
		} else if page.Revision != complete.Revision {
			return CodingLedgerRevision{}, fmt.Errorf("coding ledger changed while loading task %s", taskID)
		}
		complete.Ledger.Requirements = append(complete.Ledger.Requirements, page.Ledger.Requirements...)
		complete.Ledger.Constraints = append(complete.Ledger.Constraints, page.Ledger.Constraints...)
		complete.Ledger.Groups = append(complete.Ledger.Groups, page.Ledger.Groups...)
		complete.Ledger.AcceptanceCriteria = append(complete.Ledger.AcceptanceCriteria, page.Ledger.AcceptanceCriteria...)
		complete.Ledger.Checks = append(complete.Ledger.Checks, page.Ledger.Checks...)
		complete.Ledger.DecisionsAndQuestions = append(complete.Ledger.DecisionsAndQuestions, page.Ledger.DecisionsAndQuestions...)
		complete.Ledger.Dependencies = append(complete.Ledger.Dependencies, page.Ledger.Dependencies...)
		complete.Ledger.Findings = append(complete.Ledger.Findings, page.Ledger.Findings...)
		count := len(page.Ledger.Requirements) + len(page.Ledger.Constraints) + len(page.Ledger.Groups) +
			len(page.Ledger.AcceptanceCriteria) + len(page.Ledger.Checks) +
			len(page.Ledger.DecisionsAndQuestions) + len(page.Ledger.Dependencies) +
			len(page.Ledger.Findings)
		if count < pageSize {
			return complete, nil
		}
	}
}

func codingAgentTaskFromLedgerDetail(detail CodingLedgerRevision) (*codingagent.Task, error) {
	if len(detail.Ledger.Tasks) != 1 {
		return nil, fmt.Errorf("coding ledger task selection requires exactly one task")
	}
	return codingAgentTaskFromLedger(detail.Ledger, detail.Ledger.Tasks[0].ID)
}

func codingAgentTaskFromLedger(ledger model.EngineeringLedger, taskID model.LedgerItemID) (*codingagent.Task, error) {
	var ledgerTask *model.LedgerTask
	for index := range ledger.Tasks {
		if ledger.Tasks[index].ID == taskID && ledger.Tasks[index].Disposition == model.LedgerDispositionActive {
			ledgerTask = &ledger.Tasks[index]
			break
		}
	}
	if ledgerTask == nil {
		return nil, fmt.Errorf("coding ledger lacks active task %s", taskID)
	}
	task := &codingagent.Task{
		ID:                   string(ledgerTask.ID),
		Goal:                 ledgerTask.Goal,
		KnownAffectedSystems: append([]string(nil), ledgerTask.KnownAffectedSystems...),
		ReadFiles:            append([]string(nil), ledgerTask.ReadFiles...),
		WriteFiles:           append([]string(nil), ledgerTask.WriteFiles...),
		DeleteFiles:          append([]string(nil), ledgerTask.DeleteFiles...),
	}
	if strings.TrimSpace(ledgerTask.DeliveryObligation) != "" {
		task.Goal = strings.TrimSpace(task.Goal + "\nDelivery obligation: " + ledgerTask.DeliveryObligation)
	}
	for _, id := range ledgerTask.RequirementIDs {
		task.RequirementIDs = append(task.RequirementIDs, string(id))
	}
	requirements := make(map[model.LedgerItemID]model.LedgerRequirement, len(ledger.Requirements))
	for _, requirement := range ledger.Requirements {
		if requirement.Disposition == model.LedgerDispositionActive {
			requirements[requirement.ID] = requirement
		}
	}
	for _, id := range ledgerTask.RequirementIDs {
		requirement, exists := requirements[id]
		if !exists || strings.TrimSpace(requirement.Outcome) == "" {
			return nil, fmt.Errorf("coding ledger task %s lacks requirement %s", ledgerTask.ID, id)
		}
		task.Guidance = append(task.Guidance, codingagent.TaskGuidance{ID: string(id), Kind: "requirement", Text: requirement.Outcome})
	}
	constraints := make(map[model.LedgerItemID]model.LedgerConstraint, len(ledger.Constraints))
	for _, constraint := range ledger.Constraints {
		if constraint.Disposition == model.LedgerDispositionActive {
			constraints[constraint.ID] = constraint
		}
	}
	for _, id := range ledgerTask.ConstraintIDs {
		constraint, exists := constraints[id]
		if !exists || strings.TrimSpace(constraint.Rule) == "" {
			return nil, fmt.Errorf("coding ledger task %s lacks constraint %s", ledgerTask.ID, id)
		}
		task.Guidance = append(task.Guidance, codingagent.TaskGuidance{ID: string(id), Kind: "constraint", Text: constraint.Rule})
	}
	if ledgerTask.GroupID != "" {
		found := false
		for _, group := range ledger.Groups {
			if group.ID == ledgerTask.GroupID && group.Disposition == model.LedgerDispositionActive && strings.TrimSpace(group.Title) != "" {
				task.Guidance = append(task.Guidance, codingagent.TaskGuidance{ID: string(group.ID), Kind: "group", Text: group.Title})
				found = true
				break
			}
		}
		if !found {
			return nil, fmt.Errorf("coding ledger task %s lacks group %s", ledgerTask.ID, ledgerTask.GroupID)
		}
	}
	decisionTargets := map[model.LedgerItemID]bool{ledgerTask.ID: true}
	if ledgerTask.GroupID != "" {
		decisionTargets[ledgerTask.GroupID] = true
	}
	for _, id := range append(append(append([]model.LedgerItemID{}, ledgerTask.RequirementIDs...), ledgerTask.ConstraintIDs...), ledgerTask.AcceptanceCriterionIDs...) {
		decisionTargets[id] = true
	}
	for _, check := range ledger.Checks {
		if check.Disposition == model.LedgerDispositionActive && check.Required &&
			ledgerIDsOverlap(ledgerTask.AcceptanceCriterionIDs, check.AcceptanceCriterionIDs) &&
			(len(ledgerTask.RequirementIDs) == 0 || ledgerIDsOverlap(ledgerTask.RequirementIDs, check.RequirementIDs)) {
			decisionTargets[check.ID] = true
		}
	}
	for _, decision := range ledger.DecisionsAndQuestions {
		if decision.Disposition != model.LedgerDispositionActive || strings.TrimSpace(decision.Answer) == "" {
			continue
		}
		relevant := false
		for _, id := range decision.Affects {
			relevant = relevant || decisionTargets[id]
		}
		if !relevant {
			continue
		}
		task.Guidance = append(task.Guidance, codingagent.TaskGuidance{
			ID: string(decision.ID), Kind: "decision", Text: decision.Prompt + ": " + decision.Answer,
		})
	}
	criteria := make(map[model.LedgerItemID]model.LedgerAcceptanceCriterion, len(ledger.AcceptanceCriteria))
	for _, criterion := range ledger.AcceptanceCriteria {
		if criterion.Disposition != model.LedgerDispositionActive {
			continue
		}
		criteria[criterion.ID] = criterion
	}
	for _, id := range ledgerTask.AcceptanceCriterionIDs {
		criterion, exists := criteria[id]
		if !exists || strings.TrimSpace(criterion.Outcome) == "" {
			return nil, fmt.Errorf("coding ledger task %s lacks acceptance criterion %s", ledgerTask.ID, id)
		}
		task.Acceptance = append(task.Acceptance, criterion.Outcome)
	}
	for _, dependency := range ledger.Dependencies {
		if dependency.Disposition == model.LedgerDispositionActive && dependency.TaskID == ledgerTask.ID {
			task.Dependencies = append(task.Dependencies, string(dependency.PrerequisiteID))
		}
	}
	checkIDs := make(map[model.LedgerItemID]bool)
	for _, check := range ledger.Checks {
		if check.Disposition != model.LedgerDispositionActive || !check.Required ||
			!ledgerIDsOverlap(ledgerTask.AcceptanceCriterionIDs, check.AcceptanceCriterionIDs) ||
			(len(ledgerTask.RequirementIDs) > 0 && !ledgerIDsOverlap(ledgerTask.RequirementIDs, check.RequirementIDs)) {
			continue
		}
		if checkIDs[check.ID] {
			return nil, fmt.Errorf("coding ledger task %s repeats check %s", ledgerTask.ID, check.ID)
		}
		checkIDs[check.ID] = true
		if len(check.Argv) > 0 {
			task.Checks = append(task.Checks, codingagent.Check{ID: string(check.ID), Argv: append([]string(nil), check.Argv...)})
		} else {
			task.Guidance = append(task.Guidance, codingagent.TaskGuidance{ID: string(check.ID), Kind: "verification_observation", Text: check.Observation})
		}
	}
	if strings.TrimSpace(task.ID) == "" || strings.TrimSpace(task.Goal) == "" || len(task.Acceptance) == 0 {
		return nil, fmt.Errorf("coding ledger selected an incomplete task context")
	}
	return task, nil
}

func validateFrozenCodingAgentTasks(ledger model.EngineeringLedger) error {
	count := 0
	for _, ledgerTask := range ledger.Tasks {
		if ledgerTask.Disposition != model.LedgerDispositionActive {
			continue
		}
		task, err := codingAgentTaskFromLedger(ledger, ledgerTask.ID)
		if err != nil {
			return err
		}
		if err := task.Validate(); err != nil {
			return fmt.Errorf("task %s: %w", ledgerTask.ID, err)
		}
		deliveryOnly := len(ledgerTask.RequirementIDs) == 0 && strings.TrimSpace(ledgerTask.DeliveryObligation) != ""
		for _, requirementID := range ledgerTask.RequirementIDs {
			covered := false
			for _, check := range ledger.Checks {
				covered = covered || check.Disposition == model.LedgerDispositionActive && check.Required &&
					ledgerIDsOverlap([]model.LedgerItemID{requirementID}, check.RequirementIDs) &&
					ledgerIDsOverlap(ledgerTask.AcceptanceCriterionIDs, check.AcceptanceCriterionIDs)
			}
			if !covered {
				return fmt.Errorf("task %s requirement %s lacks a bound required check", ledgerTask.ID, requirementID)
			}
		}
		for _, acceptanceID := range ledgerTask.AcceptanceCriterionIDs {
			covered := false
			for _, check := range ledger.Checks {
				covered = covered || check.Disposition == model.LedgerDispositionActive && check.Required &&
					ledgerIDsOverlap([]model.LedgerItemID{acceptanceID}, check.AcceptanceCriterionIDs) &&
					(deliveryOnly || ledgerIDsOverlap(ledgerTask.RequirementIDs, check.RequirementIDs))
			}
			if !covered {
				return fmt.Errorf("task %s acceptance criterion %s lacks a bound required check", ledgerTask.ID, acceptanceID)
			}
		}
		count++
	}
	if count == 0 {
		return fmt.Errorf("coding ledger requires executable tasks")
	}
	return nil
}

func (r *Repository) readCompleteCodingLedger(
	ctx context.Context,
	ledgerID, revision int64,
) (CodingLedgerRevision, error) {
	var result CodingLedgerRevision
	for offset := 0; ; offset += 1000 {
		page, err := r.ReadCodingLedger(ctx, ledgerID, CodingLedgerReadOptions{
			Revision: revision, Limit: 1000, Offset: offset,
		})
		if err != nil {
			return CodingLedgerRevision{}, err
		}
		if offset == 0 {
			result = page
			revision = page.Revision
			result.Ledger = model.EngineeringLedger{ID: page.Ledger.ID, Revision: page.Ledger.Revision}
		} else if page.Revision != result.Revision {
			return CodingLedgerRevision{}, fmt.Errorf("coding ledger changed while loading complete revision")
		}
		result.Ledger.Objectives = append(result.Ledger.Objectives, page.Ledger.Objectives...)
		result.Ledger.Requirements = append(result.Ledger.Requirements, page.Ledger.Requirements...)
		result.Ledger.Constraints = append(result.Ledger.Constraints, page.Ledger.Constraints...)
		result.Ledger.Groups = append(result.Ledger.Groups, page.Ledger.Groups...)
		result.Ledger.Tasks = append(result.Ledger.Tasks, page.Ledger.Tasks...)
		result.Ledger.AcceptanceCriteria = append(result.Ledger.AcceptanceCriteria, page.Ledger.AcceptanceCriteria...)
		result.Ledger.Checks = append(result.Ledger.Checks, page.Ledger.Checks...)
		result.Ledger.DecisionsAndQuestions = append(result.Ledger.DecisionsAndQuestions, page.Ledger.DecisionsAndQuestions...)
		result.Ledger.Dependencies = append(result.Ledger.Dependencies, page.Ledger.Dependencies...)
		result.Ledger.Findings = append(result.Ledger.Findings, page.Ledger.Findings...)
		count := len(ledgerRepositoryRecords(page.Ledger)) + len(page.Ledger.Dependencies)
		if count < 1000 {
			return result, nil
		}
	}
}

func ledgerIDsOverlap(left, right []model.LedgerItemID) bool {
	for _, a := range left {
		for _, b := range right {
			if a == b {
				return true
			}
		}
	}
	return false
}
