package specialists

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type Spec struct {
	ID              string   `json:"id"`
	Purpose         string   `json:"purpose"`
	PreferredModel  []string `json:"preferred_model,omitempty"`
	AllowedTools    []string `json:"allowed_tools,omitempty"`
	ForbiddenTools  []string `json:"forbidden_tools,omitempty"`
	ContextBudget   int      `json:"context_budget,omitempty"`
	InputSchema     string   `json:"input_schema,omitempty"`
	OutputSchema    string   `json:"output_schema,omitempty"`
	StopConditions  []string `json:"stop_conditions,omitempty"`
	RetryPolicy     string   `json:"retry_policy,omitempty"`
	RequireEvidence bool     `json:"require_evidence,omitempty"`
	Instructions    string   `json:"instructions,omitempty"`
	inputSchemaRaw  json.RawMessage
	outputSchemaRaw json.RawMessage
}

func (s Spec) Validate() error {
	if strings.TrimSpace(s.ID) == "" {
		return errors.New("specialist id is required")
	}
	if strings.TrimSpace(s.Purpose) == "" {
		return fmt.Errorf("specialist %s missing purpose", s.ID)
	}
	if s.ContextBudget < 0 {
		return fmt.Errorf("specialist %s has invalid context_budget", s.ID)
	}
	return nil
}

type Registry struct {
	Specs map[string]Spec
}

func LoadRegistry(root string) (*Registry, error) {
	entries, err := os.ReadDir(root)
	if err != nil {
		return nil, fmt.Errorf("read skills root: %w", err)
	}
	reg := &Registry{Specs: map[string]Spec{}}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		dir := filepath.Join(root, entry.Name())
		spec, err := loadSpec(dir)
		if err != nil {
			return nil, err
		}
		reg.Specs[spec.ID] = spec
	}
	return reg, nil
}

func loadSpec(dir string) (Spec, error) {
	specPath := filepath.Join(dir, "spec.json")
	instructionPath := filepath.Join(dir, "SKILL.md")

	raw, err := os.ReadFile(specPath)
	if err != nil {
		return Spec{}, fmt.Errorf("read %s: %w", specPath, err)
	}
	var spec Spec
	if err := json.Unmarshal(raw, &spec); err != nil {
		return Spec{}, fmt.Errorf("parse %s: %w", specPath, err)
	}
	if body, err := os.ReadFile(instructionPath); err == nil {
		spec.Instructions = strings.TrimSpace(string(body))
	}
	inputSchemaRaw, err := loadSchemaFile(dir, spec.InputSchema)
	if err != nil {
		return Spec{}, fmt.Errorf("specialist %s input schema: %w", spec.ID, err)
	}
	outputSchemaRaw, err := loadSchemaFile(dir, spec.OutputSchema)
	if err != nil {
		return Spec{}, fmt.Errorf("specialist %s output schema: %w", spec.ID, err)
	}
	spec.inputSchemaRaw = inputSchemaRaw
	spec.outputSchemaRaw = outputSchemaRaw
	if err := spec.Validate(); err != nil {
		return Spec{}, err
	}
	return spec, nil
}

func loadSchemaFile(dir string, name string) (json.RawMessage, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return nil, nil
	}
	path := filepath.Join(dir, name)
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}
	if !json.Valid(raw) {
		return nil, fmt.Errorf("parse %s: invalid json", path)
	}
	return json.RawMessage(raw), nil
}

func (s Spec) ValidateInputPayload(payload any) error {
	if len(s.inputSchemaRaw) == 0 {
		return nil
	}
	if err := ValidatePayloadAgainstSchema(s.inputSchemaRaw, payload); err != nil {
		return fmt.Errorf("specialist %s input payload: %w", s.ID, err)
	}
	return nil
}

func (s Spec) ValidateOutputPayload(payload any) error {
	if len(s.outputSchemaRaw) == 0 {
		return nil
	}
	if err := ValidatePayloadAgainstSchema(s.outputSchemaRaw, payload); err != nil {
		return fmt.Errorf("specialist %s output payload: %w", s.ID, err)
	}
	return nil
}
