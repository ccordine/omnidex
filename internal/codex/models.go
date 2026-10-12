package codex

import (
	"context"
	"fmt"
)

// Model is the portable account-visible catalog. IDs are usable model handles,
// not the app server's picker identities. Options remain catalog-owned strings.
type Model struct {
	ID                 string         `json:"id"`
	DisplayName        string         `json:"display_name"`
	Description        string         `json:"description"`
	IsDefault          bool           `json:"is_default"`
	Hidden             bool           `json:"hidden"`
	DefaultEffort      string         `json:"default_effort"`
	Efforts            []EffortOption `json:"efforts"`
	ServiceTiers       []ServiceTier  `json:"service_tiers"`
	DefaultServiceTier *string        `json:"default_service_tier"`
}

type EffortOption struct {
	Value       string `json:"value"`
	Description string `json:"description"`
}

type ServiceTier struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
}

type nativeModel struct {
	Model                     string `json:"model"`
	DisplayName               string `json:"displayName"`
	Description               string `json:"description"`
	IsDefault                 bool   `json:"isDefault"`
	Hidden                    bool   `json:"hidden"`
	DefaultReasoningEffort    string `json:"defaultReasoningEffort"`
	SupportedReasoningEfforts []struct {
		Value       string `json:"reasoningEffort"`
		Description string `json:"description"`
	} `json:"supportedReasoningEfforts"`
	ServiceTiers       []ServiceTier `json:"serviceTiers"`
	DefaultServiceTier *string       `json:"defaultServiceTier"`
}

type nativeModelPage struct {
	Data       []nativeModel `json:"data"`
	NextCursor *string       `json:"nextCursor"`
}

// Models reads discovery only: no thread, model turn, or task is created.
func (client *Client) Models(ctx context.Context) ([]Model, error) {
	connection, err := startConnection(ctx, client.command, client.directory)
	if err != nil {
		return nil, err
	}
	defer connection.close()
	if _, err := connection.call("initialize", map[string]any{
		"clientInfo":   map[string]string{"name": "omnidex", "title": "Omnidex", "version": "0.5.0"},
		"capabilities": map[string]bool{"experimentalApi": true}}, nil); err != nil {
		return nil, err
	}
	if err := connection.send("initialized", map[string]any{}, nil); err != nil {
		return nil, err
	}
	return readModelCatalog(func(cursor *string) (nativeModelPage, error) {
		var page nativeModelPage
		_, err := connection.call("model/list", map[string]any{"cursor": cursor, "includeHidden": true}, &page)
		return page, err
	})
}

func readModelCatalog(read func(*string) (nativeModelPage, error)) ([]Model, error) {
	models := []Model{}
	seenModels, seenCursors := map[string]bool{}, map[string]bool{}
	var cursor *string
	for {
		page, err := read(cursor)
		if err != nil {
			return nil, err
		}
		for _, native := range page.Data {
			if native.Model == "" || native.DisplayName == "" || native.DefaultReasoningEffort == "" || seenModels[native.Model] {
				return nil, fmt.Errorf("Codex returned an invalid or duplicate model catalog entry")
			}
			seenModels[native.Model] = true
			model := Model{ID: native.Model, DisplayName: native.DisplayName, Description: native.Description,
				IsDefault: native.IsDefault, Hidden: native.Hidden, DefaultEffort: native.DefaultReasoningEffort,
				Efforts: []EffortOption{}, ServiceTiers: append([]ServiceTier{}, native.ServiceTiers...), DefaultServiceTier: native.DefaultServiceTier}
			for _, effort := range native.SupportedReasoningEfforts {
				if effort.Value == "" {
					return nil, fmt.Errorf("Codex returned an empty reasoning option")
				}
				model.Efforts = append(model.Efforts, EffortOption{Value: effort.Value, Description: effort.Description})
			}
			models = append(models, model)
		}
		if page.NextCursor == nil {
			return models, nil
		}
		if *page.NextCursor == "" || seenCursors[*page.NextCursor] {
			return nil, fmt.Errorf("Codex returned a cyclic model catalog cursor")
		}
		seenCursors[*page.NextCursor] = true
		cursor = page.NextCursor
	}
}
