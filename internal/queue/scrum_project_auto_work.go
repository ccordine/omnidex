package queue

import (
	"context"
	"fmt"
	"strconv"

	"github.com/jackc/pgx/v5"
)

type ProjectAutoWorkAction string

const (
	ProjectAutoWorkPlay  ProjectAutoWorkAction = "play"
	ProjectAutoWorkPause ProjectAutoWorkAction = "pause"
)

type ProjectAutoWorkCommand struct {
	ProjectID int64
	Action    ProjectAutoWorkAction
}

type ProjectAutoWorkResult struct {
	Config      ScrumAutoWorkConfig
	ActiveCard  *DBScrumCard
	JobID       int64
	PausedCards int
}

// ApplyProjectAutoWork owns the project setting, active-card selection,
// cancellation, queue transition, messages, jobs, events, and metrics in one
// locked transaction. Transport refresh/realtime happens only after commit.
func (r *Repository) ApplyProjectAutoWork(
	ctx context.Context,
	command ProjectAutoWorkCommand,
) (ProjectAutoWorkResult, error) {
	if r == nil || r.pool == nil || ctx == nil || command.ProjectID <= 0 {
		return ProjectAutoWorkResult{}, fmt.Errorf("PostgreSQL, context, and project are required for project auto-work")
	}
	if command.Action != ProjectAutoWorkPlay && command.Action != ProjectAutoWorkPause {
		return ProjectAutoWorkResult{}, fmt.Errorf("project auto-work action %q is not registered", command.Action)
	}
	tx, err := r.beginLockedProjectTx(ctx, command.ProjectID, "apply project auto-work")
	if err != nil {
		return ProjectAutoWorkResult{}, err
	}
	defer rollbackTx(ctx, tx, "apply project auto-work")
	var settings []byte
	if err := tx.QueryRow(ctx, `SELECT settings FROM projects WHERE id=$1 FOR UPDATE`, command.ProjectID).Scan(&settings); err != nil {
		return ProjectAutoWorkResult{}, err
	}
	if err := validateProjectSettings(settings); err != nil {
		return ProjectAutoWorkResult{}, fmt.Errorf(
			"validate current project settings before project auto-work mutation: %w",
			err,
		)
	}
	config, err := DecodeScrumAutoWorkConfig(settings)
	if err != nil {
		return ProjectAutoWorkResult{}, err
	}
	config.Enabled = command.Action == ProjectAutoWorkPlay
	encoded, err := encodeScrumAutoWorkSettings(settings, config)
	if err != nil {
		return ProjectAutoWorkResult{}, err
	}
	if _, err := tx.Exec(ctx, `
		UPDATE projects SET settings=$2::jsonb,updated_at=clock_timestamp(),last_seen_at=clock_timestamp()
		WHERE id=$1
	`, command.ProjectID, string(encoded)); err != nil {
		return ProjectAutoWorkResult{}, fmt.Errorf("persist project auto-work config: %w", err)
	}
	result := ProjectAutoWorkResult{Config: config}
	if command.Action == ProjectAutoWorkPause {
		if err := r.pauseProjectAutoWorkTx(ctx, tx, command.ProjectID, &result); err != nil {
			return ProjectAutoWorkResult{}, err
		}
	} else if err := r.startProjectAutoWorkTx(ctx, tx, command.ProjectID, config, &result); err != nil {
		return ProjectAutoWorkResult{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return ProjectAutoWorkResult{}, fmt.Errorf("commit project auto-work: %w", err)
	}
	return result, nil
}

func (r *Repository) startProjectAutoWorkTx(
	ctx context.Context,
	tx pgx.Tx,
	projectID int64,
	config ScrumAutoWorkConfig,
	result *ProjectAutoWorkResult,
) error {
	running, found, err := findRunningScrumCardTx(ctx, tx, projectID)
	if err != nil {
		return err
	}
	if found {
		if running.JobID == "" {
			return fmt.Errorf("running Scrum card %q has invalid job authority", running.ID)
		}
		parsed, err := strconv.ParseInt(running.JobID, 10, 64)
		if err != nil || parsed <= 0 || strconv.FormatInt(parsed, 10) != running.JobID {
			return fmt.Errorf("running Scrum card %q has noncanonical job ID %q", running.ID, running.JobID)
		}
		result.ActiveCard = &running
		result.JobID = parsed
		return nil
	}
	card, found, err := findProjectAutoWorkCardTx(ctx, tx, projectID, config.SourceColumns)
	if err != nil || !found {
		return err
	}
	play, err := r.prepareScrumCardPlayTx(ctx, tx, card)
	if err != nil {
		return err
	}
	result.ActiveCard, result.JobID = &play.Card, play.Job.ID
	return nil
}

func (r *Repository) pauseProjectAutoWorkTx(
	ctx context.Context,
	tx pgx.Tx,
	projectID int64,
	result *ProjectAutoWorkResult,
) error {
	rows, err := tx.Query(ctx, scrumCardSelectionSQL+`
		 AND play_state IN ('running','queued') ORDER BY
		 CASE play_state WHEN 'running' THEN 0 ELSE 1 END,queue_order,id FOR UPDATE
	`, projectID)
	if err != nil {
		return err
	}
	var cards []DBScrumCard
	for rows.Next() {
		card, err := scanDBScrumCard(rows)
		if err != nil {
			rows.Close()
			return err
		}
		cards = append(cards, card)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return err
	}
	rows.Close()
	runningCount := 0
	for index := range cards {
		if cards[index].PlayState == "running" {
			runningCount++
		}
	}
	if runningCount > 1 {
		return fmt.Errorf("Scrum project %d has %d running cards", projectID, runningCount)
	}
	for index := range cards {
		card := cards[index]
		if card.PlayState == "running" {
			command := ScrumCardPlayCommand{
				ProjectID: projectID, CardID: card.ID, ExpectedUpdatedAt: card.UpdatedAt,
			}
			if err := pauseRunningScrumCardTx(ctx, tx, command, &card); err != nil {
				return err
			}
		} else {
			if card.JobID != "" {
				return fmt.Errorf("queued Scrum card %q unexpectedly owns a job", card.ID)
			}
			previous := card
			if err := setScrumCardPausedTx(ctx, tx, &card, "Project auto-work paused"); err != nil {
				return err
			}
			if err := applyScrumCardStateMetricsTx(ctx, tx, previous, card, ""); err != nil {
				return err
			}
		}
		result.PausedCards++
	}
	return nil
}

func findProjectAutoWorkCardTx(
	ctx context.Context,
	tx pgx.Tx,
	projectID int64,
	columns []string,
) (DBScrumCard, bool, error) {
	rows, err := tx.Query(ctx, scrumCardSelectionSQL+`
		 AND (play_state='queued' OR (play_state NOT IN ('running','queued') AND column_name=ANY($2::text[])))
		 ORDER BY CASE WHEN play_state='queued' THEN 0 ELSE 1 END,
		 queue_order,array_position($2::text[],column_name),board_order,updated_at,id
		 LIMIT 1 FOR UPDATE
	`, projectID, columns)
	if err != nil {
		return DBScrumCard{}, false, err
	}
	defer rows.Close()
	if !rows.Next() {
		return DBScrumCard{}, false, rows.Err()
	}
	card, err := scanDBScrumCard(rows)
	return card, err == nil, err
}
