package queue

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/gryph/omnidex/internal/codingagent"
	"github.com/gryph/omnidex/internal/model"
	"github.com/jackc/pgx/v5"
)

// Completion validates the durable journal inside the same transaction that
// terminalizes the current attempt. Agent claims alone cannot close the job.
func requireCodingAgentCompletionTx(ctx context.Context, tx pgx.Tx, job model.Job, stepID int64) error {
	configuration, err := codingagent.ConfigurationFromMetadata(job.Metadata)
	if err != nil {
		return err
	}
	if configuration.Mode != codingagent.Agents {
		return nil
	}
	var action string
	if err := tx.QueryRow(ctx, `SELECT action FROM job_steps WHERE id=$1 AND job_id=$2`, stepID, job.ID).Scan(&action); err != nil {
		return err
	}
	if action != "v3_coding" {
		return nil
	}
	plan, err := readCodingPlan(ctx, tx, job.ID, job.CurrentGeneration)
	if err != nil {
		return err
	}
	if plan.State != model.CodingPlanStateFrozen {
		return fmt.Errorf("agent completion requires the current frozen requirement plan")
	}
	rows, err := tx.Query(ctx, `SELECT id,payload_json-'frame' FROM evidence WHERE job_id=$1 AND kind='coding_agent' AND payload_json->>'generation'=$2 ORDER BY id`, job.ID, fmt.Sprint(job.CurrentGeneration))
	if err != nil {
		return err
	}
	defer rows.Close()
	var entries []codingagent.Entry
	for rows.Next() {
		var entry codingagent.Entry
		var raw []byte
		var id int64
		if err := rows.Scan(&id, &raw); err != nil {
			return err
		}
		if err := json.Unmarshal(raw, &entry); err != nil {
			return err
		}
		entry.ID = id
		entries = append(entries, entry)
	}
	if err := rows.Err(); err != nil {
		return err
	}
	if err := validateCodingAgentCompletionJournal(entries, job.CurrentGeneration, configuration); err != nil {
		return err
	}
	return validateCodingAgentRequirementCoverage(entries, plan)
}

func validateCodingAgentCompletionJournal(entries []codingagent.Entry, generation int64, configuration codingagent.Configuration) error {
	var workload *codingagent.Workload
	accepted := map[string]bool{}
	tasks := map[string]codingagent.Task{}
	var lastPhase string
	opened := map[int64]codingagent.Entry{}
	completed := map[int64]codingagent.Entry{}
	native := map[int64]bool{}
	for _, entry := range entries {
		if entry.Generation != generation {
			return fmt.Errorf("agent completion journal changed generation")
		}
		lastPhase = entry.Phase
		switch entry.Phase {
		case "call_opened":
			opened[entry.ID] = entry
		case "native_event":
			native[entry.CallID] = true
		case "call_completed":
			completed[entry.CallID] = entry
		case "plan_frozen":
			if err := codingAgentAcceptedCall(entry, codingagent.Plan, configuration.Planner, opened, completed, native); err != nil {
				return err
			}
			if workload != nil || entry.Workload == nil {
				return fmt.Errorf("agent completion requires one immutable task plan")
			}
			if err := entry.Workload.Validate(); err != nil {
				return err
			}
			workload = entry.Workload
			for _, task := range workload.Tasks {
				tasks[task.ID] = task
			}
		case "task_accepted":
			if err := codingAgentAcceptedCall(entry, codingagent.Review, configuration.Reviewer, opened, completed, native); err != nil {
				return err
			}
			task, known := tasks[entry.TaskID]
			if !known || !codingAgentChecksMatch(task.Checks, entry.Checks) {
				return fmt.Errorf("agent task acceptance lacks successful frozen checks")
			}
			for _, dependency := range task.Dependencies {
				if !accepted[dependency] {
					return fmt.Errorf("agent task acceptance precedes its dependency")
				}
			}
			accepted[entry.TaskID] = true
		case "task_reviewed", "task_reopened":
			if _, known := tasks[entry.TaskID]; !known {
				return fmt.Errorf("agent journal reopened an unknown task")
			}
			accepted[entry.TaskID] = false
		case "overall_accepted":
			if err := codingAgentAcceptedCall(entry, codingagent.OverallReview, configuration.Reviewer, opened, completed, native); err != nil {
				return err
			}
			if workload == nil || !codingAgentChecksMatch(workload.Checks, entry.Checks) {
				return fmt.Errorf("agent overall acceptance lacks successful frozen checks")
			}
			for id := range tasks {
				if !accepted[id] {
					return fmt.Errorf("agent overall acceptance contains unfinished task %s", id)
				}
			}
		}
	}
	if workload == nil || lastPhase != "overall_accepted" {
		return fmt.Errorf("agent completion requires current overall acceptance after every reopening or question")
	}
	return nil
}

func codingAgentChecksMatch(checks []codingagent.Check, results []codingagent.CheckResult) bool {
	if len(checks) == 0 || len(checks) != len(results) {
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
	if role == codingagent.Plan {
		return nil
	}
	var review codingagent.ReviewResult
	if err := json.Unmarshal(result.Content, &review); err != nil {
		return err
	}
	if review.Findings == nil || review.Questions == nil || len(review.Findings) != 0 || len(review.Questions) != 0 {
		return fmt.Errorf("agent acceptance contains unresolved independent findings or questions")
	}
	return nil
}
