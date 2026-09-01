package queue

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/gryph/omnidex/internal/exactjson"
	"github.com/gryph/omnidex/internal/model"
	"github.com/gryph/omnidex/internal/modelconfig"
	"github.com/gryph/omnidex/internal/scrum"
	"github.com/jackc/pgx/v5"
)

func scrumPlayAuthorityTx(
	ctx context.Context,
	tx pgx.Tx,
	card DBScrumCard,
	modelAuthority modelconfig.Authority,
	codingScopeMode model.CodingScopeMode,
) (scrum.JobMetadata, string, error) {
	var settings json.RawMessage
	if err := tx.QueryRow(ctx, `SELECT settings FROM projects WHERE id=$1 FOR UPDATE`, card.ProjectID).Scan(&settings); err != nil {
		return scrum.JobMetadata{}, "", err
	}
	modelOverrides, err := modelconfig.FromSettingsJSON(settings)
	if err != nil {
		return scrum.JobMetadata{}, "", fmt.Errorf("parse locked Scrum project model routing: %w", err)
	}
	modelSnapshot, err := modelAuthority.Resolve(modelOverrides)
	if err != nil {
		return scrum.JobMetadata{}, "", fmt.Errorf("resolve locked Scrum model routing snapshot: %w", err)
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
		CardID:    card.ID,
		CardTitle: card.Title, CardDescription: card.Description,
		Checklist: formattedChecklist, TestCriteria: formattedTests, ModelConfig: modelSnapshot,
		CodingScopeMode: codingScopeMode,
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
