package queue

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/gryph/omnidex/internal/exactjson"
	"github.com/gryph/omnidex/internal/modelconfig"
	"github.com/gryph/omnidex/internal/scrum"
	"github.com/jackc/pgx/v5"
)

func scrumPlayAuthorityTx(ctx context.Context, tx pgx.Tx, card DBScrumCard) (scrum.JobMetadata, string, error) {
	if err := requireScrumAIActiveTx(ctx, tx); err != nil {
		return scrum.JobMetadata{}, "", err
	}
	var settings json.RawMessage
	if err := tx.QueryRow(ctx, `SELECT settings FROM projects WHERE id=$1 FOR UPDATE`, card.ProjectID).Scan(&settings); err != nil {
		return scrum.JobMetadata{}, "", err
	}
	modelSnapshot, err := modelconfig.FromSettingsJSON(settings)
	if err != nil {
		return scrum.JobMetadata{}, "", fmt.Errorf("parse locked Scrum project model routing: %w", err)
	}
	checklist, err := decodeScrumPlayItems(card.Checklist, "checklist")
	if err != nil {
		return scrum.JobMetadata{}, "", err
	}
	tests, err := decodeScrumPlayItems(card.TestCriteria, "test criteria")
	if err != nil {
		return scrum.JobMetadata{}, "", err
	}
	formattedChecklist, err := scrum.FormatChecklist(checklist)
	if err != nil {
		return scrum.JobMetadata{}, "", err
	}
	formattedTests, err := scrum.FormatChecklist(tests)
	if err != nil {
		return scrum.JobMetadata{}, "", err
	}
	metadata := scrum.JobMetadata{
		Source: scrum.JobMetadataSource, ProjectID: card.ProjectID, CardID: card.ID,
		CardTitle: card.Title, CardDescription: card.Description,
		Checklist: formattedChecklist, TestCriteria: formattedTests, ModelConfig: modelSnapshot,
	}
	if err := metadata.Validate(); err != nil {
		return scrum.JobMetadata{}, "", err
	}
	lines := []string{"Scrum task execution for card: " + card.Title}
	lines, err = scrum.AppendCardContextLines(lines, scrum.CardContext{
		Title: card.Title, Description: card.Description, Checklist: checklist, TestCriteria: tests,
	})
	if err != nil {
		return scrum.JobMetadata{}, "", err
	}
	lines = append(lines, "Omnidex owns completion from typed job and verification state.")
	return metadata, strings.Join(lines, "\n\n"), nil
}

func requireScrumAIActiveTx(ctx context.Context, tx pgx.Tx) error {
	if _, err := tx.Exec(ctx, `LOCK TABLE workspace_settings IN SHARE MODE`); err != nil {
		return fmt.Errorf("lock global AI control for Scrum play: %w", err)
	}
	var raw json.RawMessage
	err := tx.QueryRow(ctx, `SELECT value FROM workspace_settings WHERE key=$1`, aiControlKey).Scan(&raw)
	if err == pgx.ErrNoRows {
		return nil
	}
	if err != nil {
		return fmt.Errorf("load locked global AI control for Scrum play: %w", err)
	}
	var stored struct {
		Paused *bool `json:"paused"`
	}
	if err := exactjson.ValidateObject(raw, stored, "AI control state"); err != nil {
		return err
	}
	if err := json.Unmarshal(raw, &stored); err != nil {
		return fmt.Errorf("decode AI control state: %w", err)
	}
	if stored.Paused == nil {
		return fmt.Errorf("AI control state requires paused")
	}
	if *stored.Paused {
		return fmt.Errorf("AI is globally paused")
	}
	return nil
}

func decodeScrumPlayItems(raw json.RawMessage, name string) ([]scrum.ChecklistItem, error) {
	var items []json.RawMessage
	if !json.Valid(raw) || !bytes.HasPrefix(bytes.TrimSpace(raw), []byte("[")) ||
		json.Unmarshal(raw, &items) != nil || items == nil {
		return nil, fmt.Errorf("Scrum %s must be one JSON array", name)
	}
	result := make([]scrum.ChecklistItem, 0, len(items))
	for index, itemRaw := range items {
		if err := exactjson.ValidateObject(itemRaw, scrum.ChecklistItem{}, fmt.Sprintf("Scrum %s item %d", name, index+1)); err != nil {
			return nil, err
		}
		var item scrum.ChecklistItem
		if err := json.Unmarshal(itemRaw, &item); err != nil {
			return nil, err
		}
		result = append(result, item)
	}
	return result, nil
}

func appendScrumPlayMessageTx(ctx context.Context, tx pgx.Tx, projectID int64, cardID, role, content string) error {
	messageID, err := NewScrumMessageID(rand.Reader)
	if err != nil {
		return err
	}
	_, err = insertScrumCardMessageTx(ctx, tx, projectID, cardID, ScrumCardMessageAppend{
		ID: messageID, Role: role, Content: content,
	})
	return err
}

func touchScrumPlayProjectTx(ctx context.Context, tx pgx.Tx, projectID int64) error {
	tag, err := tx.Exec(ctx, `UPDATE projects SET last_seen_at=clock_timestamp(),updated_at=clock_timestamp() WHERE id=$1`, projectID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() != 1 {
		return fmt.Errorf("Scrum play project %d disappeared", projectID)
	}
	return nil
}
