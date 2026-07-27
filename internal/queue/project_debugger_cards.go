package queue

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/gryph/omnidex/internal/model"
	"github.com/gryph/omnidex/internal/scrumcardllm"
)

type ProjectDebuggerCardInput struct {
	Title       string
	Description string
	Column      string
	Checklist   json.RawMessage
	RefFiles    json.RawMessage
	Tags        json.RawMessage
	CardPrompt  string
	TicketModel string
	Ticket      scrumcardllm.TicketRequest
}

func (r *Repository) CreateProjectDebuggerCardJob(ctx context.Context, projectID int64, input ProjectDebuggerCardInput) (DBScrumCard, model.Job, error) {
	if r == nil || r.pool == nil || ctx == nil || projectID <= 0 {
		return DBScrumCard{}, model.Job{}, fmt.Errorf("PostgreSQL, context, and project are required for a debugger card")
	}
	input.Title = strings.TrimSpace(input.Title)
	input.Description = strings.TrimSpace(input.Description)
	input.Column = strings.ToLower(strings.TrimSpace(input.Column))
	input.CardPrompt = strings.TrimSpace(input.CardPrompt)
	input.TicketModel = strings.TrimSpace(input.TicketModel)
	if input.Title == "" || input.Description == "" || input.Column != "backlog" || input.CardPrompt == "" || input.TicketModel == "" {
		return DBScrumCard{}, model.Job{}, fmt.Errorf("debugger card requires title, description, backlog column, prompt, and ticket model")
	}
	input.Ticket.Prompt = strings.TrimSpace(input.Ticket.Prompt)
	input.Ticket.CardPrompt = strings.TrimSpace(input.Ticket.CardPrompt)
	if !input.Ticket.PlanningMode || input.Ticket.Iterate || input.Ticket.Prompt == "" || input.Ticket.CardPrompt != input.CardPrompt {
		return DBScrumCard{}, model.Job{}, fmt.Errorf("debugger card ticket must be a non-iterative planning request matching the card prompt")
	}
	if err := validateDebuggerCardJSON(input.Checklist, input.RefFiles, input.Tags); err != nil {
		return DBScrumCard{}, model.Job{}, err
	}
	cardID, err := newProjectDebuggerCardID()
	if err != nil {
		return DBScrumCard{}, model.Job{}, err
	}
	metadata, err := scrumcardllm.JobMetadata(projectID, cardID, scrumcardllm.ActionCardTicket, "", input.TicketModel, input.Ticket)
	if err != nil {
		return DBScrumCard{}, model.Job{}, err
	}
	instruction := "Generate planning ticket for: " + input.Title

	tx, err := r.beginLockedProjectTx(ctx, projectID, "project debugger card creation")
	if err != nil {
		return DBScrumCard{}, model.Job{}, err
	}
	defer rollbackTx(ctx, tx, "project debugger card creation")
	_, err = tx.Exec(ctx, `
		INSERT INTO scrum_cards (
			id, project_id, title, description, column_name, checklist, ref_files, tags, card_prompt, board_order
		) VALUES (
			$1, $2, $3, $4, $5, $6::jsonb, $7::jsonb, $8::jsonb, $9,
			COALESCE((SELECT MAX(board_order) FROM scrum_cards WHERE project_id = $2 AND column_name = $5), -1) + 1
		)
	`, cardID, projectID, input.Title, input.Description, input.Column, string(input.Checklist), string(input.RefFiles), string(input.Tags), input.CardPrompt)
	if err != nil {
		return DBScrumCard{}, model.Job{}, err
	}
	job, err := r.enqueueJobTx(ctx, tx, instruction, model.PipelineScrumCardLLM, metadata)
	if err != nil {
		return DBScrumCard{}, model.Job{}, err
	}
	card, err := scanDBScrumCard(tx.QueryRow(ctx, `
		UPDATE scrum_cards
		SET ticket_job_id = $3, updated_at = NOW()
		WHERE project_id = $1 AND id = $2
		RETURNING id, project_id, title, description, column_name, checklist, ref_files, chat,
		          model_config, agent_config, card_ticket, card_prompt, recipe_id, recipe,
		          tags, planning_chat, coach_config, test_criteria, flow_metrics,
		          job_id, tags_job_id, ticket_job_id, console_log, play_state, queue_order, board_order, created_at, updated_at
	`, projectID, cardID, strconv.FormatInt(job.ID, 10)))
	if err != nil {
		return DBScrumCard{}, model.Job{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return DBScrumCard{}, model.Job{}, err
	}
	return card, job, nil
}

func newProjectDebuggerCardID() (string, error) {
	var random [16]byte
	if _, err := rand.Read(random[:]); err != nil {
		return "", fmt.Errorf("generate project debugger card ID: %w", err)
	}
	return "card_" + hex.EncodeToString(random[:]), nil
}

func validateDebuggerCardJSON(checklist, refFiles, tags json.RawMessage) error {
	var checklistItems []struct {
		Text string `json:"text"`
		Done bool   `json:"done"`
	}
	if err := decodeStrictDebuggerJSON(checklist, &checklistItems, "checklist"); err != nil {
		return fmt.Errorf("debugger card checklist must be a non-empty typed array: %w", err)
	}
	if len(checklistItems) == 0 {
		return fmt.Errorf("debugger card checklist must be a non-empty typed array")
	}
	for i, item := range checklistItems {
		if strings.TrimSpace(item.Text) == "" || item.Done {
			return fmt.Errorf("debugger card checklist item %d must be incomplete with text", i)
		}
	}
	for label, raw := range map[string]json.RawMessage{"reference files": refFiles, "tags": tags} {
		var values []string
		if err := decodeStrictDebuggerJSON(raw, &values, label); err != nil {
			return fmt.Errorf("debugger card %s must be a string array: %w", label, err)
		}
		for i, value := range values {
			if strings.TrimSpace(value) == "" {
				return fmt.Errorf("debugger card %s item %d is empty", label, i)
			}
		}
	}
	return nil
}

func decodeStrictDebuggerJSON(raw json.RawMessage, destination any, label string) error {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(destination); err != nil {
		return fmt.Errorf("decode %s: %w", label, err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		if err == nil {
			return fmt.Errorf("%s contains trailing JSON", label)
		}
		return fmt.Errorf("%s contains trailing data: %w", label, err)
	}
	return nil
}
