package scrum

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/gryph/omnidex/internal/exactjson"
	"github.com/gryph/omnidex/internal/model"
	"github.com/gryph/omnidex/internal/modelconfig"
)

type JobMetadata struct {
	CardID             string                `json:"scrum_card_id"`
	CardTitle          string                `json:"scrum_card_title"`
	CardDescription    string                `json:"scrum_card_description"`
	Checklist          string                `json:"scrum_checklist"`
	TestCriteria       string                `json:"scrum_test_criteria"`
	ReturnColumn       string                `json:"scrum_return_column"`
	ChannelOrigin      bool                  `json:"scrum_channel_origin"`
	ChannelOperationID string                `json:"scrum_channel_operation_id"`
	ModelConfig        modelconfig.Config    `json:"model_config"`
	CodingScopeMode    model.CodingScopeMode `json:"coding_scope_mode"`
}

func (metadata JobMetadata) Validate() error {
	for name, value := range map[string]string{
		"scrum_card_id": metadata.CardID, "scrum_card_title": metadata.CardTitle,
		"scrum_card_description": metadata.CardDescription, "scrum_checklist": metadata.Checklist,
		"scrum_test_criteria": metadata.TestCriteria, "scrum_return_column": metadata.ReturnColumn,
	} {
		if !utf8.ValidString(value) || strings.ContainsRune(value, '\x00') || len(value) > 1<<20 {
			return fmt.Errorf("Scrum job metadata %s is outside the exact text bound", name)
		}
	}
	if metadata.CardID == "" || metadata.CardID != strings.TrimSpace(metadata.CardID) {
		return fmt.Errorf("Scrum job requires one canonical card ID")
	}
	if strings.TrimSpace(metadata.CardTitle) == "" {
		return fmt.Errorf("Scrum job requires a nonblank card title")
	}
	if metadata.ReturnColumn != "" {
		switch metadata.ReturnColumn {
		case "backlog", "ready", "assigned", "in_progress", "review", "blocked", "error", "done":
		default:
			return fmt.Errorf("Scrum return column %q is not registered", metadata.ReturnColumn)
		}
	}
	if metadata.ChannelOrigin {
		if !canonicalLifecycleOperationID(metadata.ChannelOperationID) {
			return fmt.Errorf("Scrum channel operation ID is required for channel-origin jobs")
		}
	} else if metadata.ChannelOperationID != "" {
		return fmt.Errorf("ordinary Scrum jobs forbid a channel operation ID")
	}
	if err := metadata.CodingScopeMode.Validate(); err != nil {
		return fmt.Errorf("Scrum job metadata coding scope authority: %w", err)
	}
	encodedConfig, err := json.Marshal(metadata.ModelConfig)
	if err != nil {
		return fmt.Errorf("encode Scrum job model routing snapshot: %w", err)
	}
	validatedConfig, err := modelconfig.FromJSON(encodedConfig)
	if err != nil {
		return fmt.Errorf("Scrum job model routing snapshot: %w", err)
	}
	if len(validatedConfig) != len(metadata.ModelConfig) {
		return fmt.Errorf("Scrum job model routing snapshot is not canonical")
	}
	return nil
}

func DecodeJobMetadata(raw json.RawMessage) (JobMetadata, error) {
	if err := exactjson.ValidateObject(raw, JobMetadata{}, "Scrum job metadata"); err != nil {
		return JobMetadata{}, err
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	var metadata JobMetadata
	if err := decoder.Decode(&metadata); err != nil {
		return JobMetadata{}, fmt.Errorf("decode Scrum job metadata: %w", err)
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fields); err != nil {
		return JobMetadata{}, fmt.Errorf("decode Scrum job metadata fields: %w", err)
	}
	for _, key := range []string{
		"scrum_card_id", "scrum_card_title",
		"scrum_card_description", "scrum_checklist", "scrum_test_criteria",
		"scrum_return_column", "scrum_channel_origin", "model_config",
		"coding_scope_mode",
		"scrum_channel_operation_id",
	} {
		if _, present := fields[key]; !present {
			return JobMetadata{}, fmt.Errorf("Scrum job metadata requires exact field %s", key)
		}
	}
	if err := metadata.Validate(); err != nil {
		return JobMetadata{}, err
	}
	return metadata, nil
}

func canonicalLifecycleOperationID(value string) bool {
	const prefix = "lifecycle_operation_"
	if len(value) != len(prefix)+64 || !strings.HasPrefix(value, prefix) {
		return false
	}
	for _, character := range value[len(prefix):] {
		if (character < '0' || character > '9') && (character < 'a' || character > 'f') {
			return false
		}
	}
	return true
}

func DecodeStoredJobMetadata(raw json.RawMessage) (JobMetadata, error) {
	return DecodeJobMetadata(raw)
}

type ChecklistItem struct {
	ID   string `json:"id"`
	Text string `json:"text"`
	Done bool   `json:"done"`
}

type CardContext struct {
	ID           string
	Title        string
	Description  string
	Checklist    []ChecklistItem
	TestCriteria []ChecklistItem
}

func FormatChecklist(items []ChecklistItem) (string, error) {
	lines := make([]string, 0, len(items))
	seen := make(map[string]struct{}, len(items))
	for index, item := range items {
		if !utf8.ValidString(item.ID) || strings.ContainsRune(item.ID, '\x00') ||
			item.ID == "" || item.ID != strings.TrimSpace(item.ID) || len(item.ID) > 256 {
			return "", fmt.Errorf("Scrum checklist item %d has a noncanonical ID", index+1)
		}
		if _, exists := seen[item.ID]; exists {
			return "", fmt.Errorf("Scrum checklist item ID %q is duplicated", item.ID)
		}
		seen[item.ID] = struct{}{}
		if !utf8.ValidString(item.Text) || strings.ContainsRune(item.Text, '\x00') ||
			strings.TrimSpace(item.Text) == "" || len(item.Text) > 1<<20 {
			return "", fmt.Errorf("Scrum checklist item %q has invalid text", item.ID)
		}
		state := "[ ]"
		if item.Done {
			state = "[x]"
		}
		lines = append(lines, state+" "+item.Text)
	}
	return strings.Join(lines, "\n"), nil
}

func AppendCardContextLines(lines []string, card CardContext) ([]string, error) {
	if strings.TrimSpace(card.Description) != "" {
		lines = append(lines, "Description:", card.Description)
	}
	checklist, err := FormatChecklist(card.Checklist)
	if err != nil {
		return nil, err
	}
	if checklist != "" {
		lines = append(lines, "Checklist:", checklist)
	}
	tests, err := FormatChecklist(card.TestCriteria)
	if err != nil {
		return nil, err
	}
	if tests != "" {
		lines = append(lines, "Test criteria (must pass before done):", tests)
	}
	return lines, nil
}

func ContextLinesFromMetadata(raw json.RawMessage) ([]string, error) {
	metadata, err := DecodeStoredJobMetadata(raw)
	if err != nil {
		return nil, err
	}
	lines := []string{"Scrum card: " + metadata.CardTitle}
	if metadata.CardDescription != "" {
		lines = append(lines, "Description:", metadata.CardDescription)
	}
	if metadata.Checklist != "" {
		lines = append(lines, "Checklist:", metadata.Checklist)
	}
	if metadata.TestCriteria != "" {
		lines = append(lines, "Test criteria (must pass before done):", metadata.TestCriteria)
	}
	return lines, nil
}
