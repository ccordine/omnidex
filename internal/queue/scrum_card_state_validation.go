package queue

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"
)

func validateStoredScrumCard(card DBScrumCard) error {
	if err := validateStoredScrumCardIdentity(card.ID, card.ProjectID, card.Title); err != nil {
		return err
	}
	for name, value := range map[string]string{
		"description": card.Description, "card_ticket": card.CardTicket,
		"card_prompt": card.CardPrompt, "job_id": card.JobID,
		"sync_job_id": card.SyncJobID,
	} {
		if err := validateStoredScrumText(name, value); err != nil {
			return fmt.Errorf("Scrum card %q: %w", card.ID, err)
		}
	}
	if _, err := ParseScrumCardColumn(card.Column); err != nil {
		return fmt.Errorf("Scrum card %q: %w", card.ID, err)
	}
	if err := validateStoredScrumPlayState(card.PlayState); err != nil {
		return fmt.Errorf("Scrum card %q: %w", card.ID, err)
	}
	for name, value := range map[string]string{"job_id": card.JobID, "sync_job_id": card.SyncJobID} {
		if value != "" && !canonicalPositiveDecimal(value) {
			return fmt.Errorf("Scrum card %q %s %q is not one canonical positive integer", card.ID, name, value)
		}
	}
	if card.QueueOrder < 0 || card.BoardOrder < 0 || card.StepContextCursor < 0 {
		return fmt.Errorf("Scrum card %q has a negative operational position or cursor", card.ID)
	}
	if card.ChannelMessageCount < 0 || card.ChannelMessageCount > maxScrumFlowWideCounter ||
		card.ChannelContentBytes < 0 || card.ChannelContentBytes > maxScrumFlowWideCounter {
		return fmt.Errorf("Scrum card %q channel counters are outside exact transport authority", card.ID)
	}
	for name, raw := range map[string]json.RawMessage{
		"checklist": card.Checklist, "ref_files": card.RefFiles,
		"tags": card.Tags, "test_criteria": card.TestCriteria,
	} {
		if err := validateStoredScrumArray(name, raw); err != nil {
			return fmt.Errorf("Scrum card %q: %w", card.ID, err)
		}
	}
	if _, err := canonicalScrumFlowMetricsForRevision(card.FlowMetrics, card.UpdatedAt); err != nil {
		return fmt.Errorf("Scrum card %q flow_metrics: %w", card.ID, err)
	}
	return validateDBScrumCursorAuthority(card)
}

func validateStoredScrumCardSummary(card DBScrumCardSummary) error {
	if err := validateStoredScrumCardIdentity(card.ID, card.ProjectID, card.Title); err != nil {
		return err
	}
	if err := validateStoredScrumText("description", card.Description); err != nil {
		return fmt.Errorf("Scrum card %q: %w", card.ID, err)
	}
	if _, err := ParseScrumCardColumn(card.Column); err != nil {
		return fmt.Errorf("Scrum card %q: %w", card.ID, err)
	}
	if err := validateStoredScrumPlayState(card.PlayState); err != nil {
		return fmt.Errorf("Scrum card %q: %w", card.ID, err)
	}
	if card.JobID != "" && !canonicalPositiveDecimal(card.JobID) {
		return fmt.Errorf("Scrum card %q job_id %q is not canonical", card.ID, card.JobID)
	}
	if card.QueueOrder < 0 || card.BoardOrder < 0 || card.ChatCount < 0 {
		return fmt.Errorf("Scrum card %q contains negative summary authority", card.ID)
	}
	if err := validateStoredScrumStringArray("tags", card.Tags); err != nil {
		return fmt.Errorf("Scrum card %q: %w", card.ID, err)
	}
	if _, err := canonicalScrumFlowMetricsForRevision(card.FlowMetrics, card.UpdatedAt); err != nil {
		return fmt.Errorf("Scrum card %q flow_metrics: %w", card.ID, err)
	}
	return nil
}

func validateStoredScrumCardIdentity(id string, projectID int64, title string) error {
	if projectID <= 0 || id == "" || id != strings.TrimSpace(id) || len(id) > MaxScrumCardIDBytes {
		return fmt.Errorf("stored Scrum card requires a positive project and canonical bounded ID")
	}
	if err := validateStoredScrumText("id", id); err != nil {
		return err
	}
	if err := validateStoredScrumText("title", title); err != nil {
		return err
	}
	if strings.TrimSpace(title) == "" {
		return fmt.Errorf("stored Scrum card title must not be blank")
	}
	return nil
}

func validateStoredScrumText(name, value string) error {
	if !utf8.ValidString(value) || strings.ContainsRune(value, '\x00') {
		return fmt.Errorf("stored Scrum %s must be valid UTF-8 without NUL", name)
	}
	return nil
}

func validateStoredScrumPlayState(value string) error {
	switch value {
	case "", "queued", "running", "paused":
		return nil
	default:
		return fmt.Errorf("stored Scrum play state %q is not registered", value)
	}
}

func validateStoredScrumArray(name string, raw json.RawMessage) error {
	var values []json.RawMessage
	if len(raw) == 0 || json.Unmarshal(raw, &values) != nil || values == nil {
		return fmt.Errorf("stored Scrum %s must be one JSON array", name)
	}
	if name == "ref_files" || name == "tags" {
		return validateStoredScrumStringArray(name, raw)
	}
	for index, value := range values {
		var object map[string]json.RawMessage
		if json.Unmarshal(value, &object) != nil || object == nil {
			return fmt.Errorf("stored Scrum %s item %d must be one object", name, index+1)
		}
	}
	return nil
}

func validateStoredScrumStringArray(name string, raw json.RawMessage) error {
	var values []string
	if len(raw) == 0 || json.Unmarshal(raw, &values) != nil || values == nil {
		return fmt.Errorf("stored Scrum %s must be one string array", name)
	}
	for index, value := range values {
		if err := validateStoredScrumText(name, value); err != nil {
			return fmt.Errorf("item %d: %w", index+1, err)
		}
	}
	return nil
}

func canonicalPositiveDecimal(value string) bool {
	parsed, err := strconv.ParseInt(value, 10, 64)
	return err == nil && parsed > 0 && strconv.FormatInt(parsed, 10) == value
}

func canonicalScrumFlowMetricsForRevision(raw json.RawMessage, revision time.Time) (json.RawMessage, error) {
	canonical, err := canonicalScrumFlowMetrics(raw)
	if err != nil || string(canonical) == "{}" {
		return canonical, err
	}
	var metrics ScrumFlowMetrics
	if err := json.Unmarshal(canonical, &metrics); err != nil {
		return nil, fmt.Errorf("decode canonical Scrum flow metrics: %w", err)
	}
	want := revision.UTC().Format("2006-01-02T15:04:05.999999Z")
	if revision.IsZero() || metrics.UpdatedAt != want {
		return nil, fmt.Errorf("updated_at %q differs from card revision %q", metrics.UpdatedAt, want)
	}
	return canonical, nil
}
