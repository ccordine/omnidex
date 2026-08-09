package modelgauntlet

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/gryph/omnidex/internal/assemblyline"
)

const (
	CapabilityRelationCasesSchemaV1  = "omnidex.model-gauntlet.capability-relation-cases.v1"
	CapabilityRelationLabelsSchemaV1 = "omnidex.model-gauntlet.capability-relation-labels.v1"
	maxGauntletInputBytes            = 1024 * 1024
)

type capabilityRelationCasesFile struct {
	Schema string                   `json:"schema"`
	Cases  []CapabilityRelationCase `json:"cases"`
}

type capabilityRelationLabelsFile struct {
	Schema string                    `json:"schema"`
	Labels []CapabilityRelationLabel `json:"labels"`
}

func LoadCapabilityRelationCases(path string) ([]CapabilityRelationCase, error) {
	raw, err := readGauntletFile(path)
	if err != nil {
		return nil, fmt.Errorf("load capability relation cases: %w", err)
	}
	var input capabilityRelationCasesFile
	if err := decodeExactJSON(string(raw), &input); err != nil {
		return nil, fmt.Errorf("decode capability relation cases: %w", err)
	}
	if input.Schema != CapabilityRelationCasesSchemaV1 {
		return nil, fmt.Errorf("capability relation cases schema must be %q", CapabilityRelationCasesSchemaV1)
	}
	validationConfig := CapabilityRelationConfig{StableModel: "validation", ReasoningModel: "validation", ContextTokens: 16384, KeepAlive: "1s"}
	if err := validateRun(validationConfig, input.Cases, validationGenerator{}); err != nil {
		return nil, err
	}
	return input.Cases, nil
}

func LoadCapabilityRelationLabels(path string, cases []CapabilityRelationCase) ([]CapabilityRelationLabel, string, error) {
	raw, err := readGauntletFile(path)
	if err != nil {
		return nil, "", fmt.Errorf("load capability relation labels: %w", err)
	}
	var input capabilityRelationLabelsFile
	if err := decodeExactJSON(string(raw), &input); err != nil {
		return nil, "", fmt.Errorf("decode capability relation labels: %w", err)
	}
	if input.Schema != CapabilityRelationLabelsSchemaV1 {
		return nil, "", fmt.Errorf("capability relation labels schema must be %q", CapabilityRelationLabelsSchemaV1)
	}
	if err := validateLabels(cases, input.Labels); err != nil {
		return nil, "", err
	}
	sum := sha256.Sum256(raw)
	return input.Labels, hex.EncodeToString(sum[:]), nil
}

func WriteCapabilityRelationResult(path string, result CapabilityRelationResult) error {
	if result.Schema != CapabilityRelationResultSchemaV1 {
		return fmt.Errorf("capability relation result schema must be %q", CapabilityRelationResultSchemaV1)
	}
	return writeExclusiveGauntletJSON(path, "capability relation result", result)
}

func writeExclusiveGauntletJSON(path, label string, value any) error {
	if strings.TrimSpace(path) == "" {
		return fmt.Errorf("%s path is required", label)
	}
	raw, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return fmt.Errorf("encode %s: %w", label, err)
	}
	raw = append(raw, '\n')
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if errors.Is(err, os.ErrExist) {
		return fmt.Errorf("%s %q already exists", label, path)
	}
	if err != nil {
		return fmt.Errorf("create %s: %w", label, err)
	}
	if _, err := file.Write(raw); err != nil {
		_ = file.Close()
		return fmt.Errorf("write %s: %w", label, err)
	}
	if err := file.Sync(); err != nil {
		_ = file.Close()
		return fmt.Errorf("sync %s: %w", label, err)
	}
	if err := file.Close(); err != nil {
		return fmt.Errorf("close %s: %w", label, err)
	}
	return nil
}

func readGauntletFile(path string) ([]byte, error) {
	if strings.TrimSpace(path) == "" {
		return nil, fmt.Errorf("path is required")
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	raw, err := io.ReadAll(io.LimitReader(file, maxGauntletInputBytes+1))
	if err != nil {
		return nil, err
	}
	if len(raw) > maxGauntletInputBytes {
		return nil, fmt.Errorf("file exceeds %d-byte hard limit", maxGauntletInputBytes)
	}
	return raw, nil
}

func validateLabels(cases []CapabilityRelationCase, labels []CapabilityRelationLabel) error {
	caseInputs := make(map[string]assemblyline.CapabilityRelationInput, len(cases))
	for _, fixture := range cases {
		caseInputs[fixture.ID] = fixture.Input
	}
	seen := make(map[string]struct{}, len(labels))
	for _, label := range labels {
		input, exists := caseInputs[label.CaseID]
		if !exists {
			return fmt.Errorf("label references unknown case %q", label.CaseID)
		}
		if _, exists := seen[label.CaseID]; exists {
			return fmt.Errorf("label for case %q is duplicated", label.CaseID)
		}
		seen[label.CaseID] = struct{}{}
		decision := assemblyline.CapabilityRelationDecision{Schema: assemblyline.CapabilityRelationSchemaV1, Relation: label.Relation}
		if err := decision.ValidateFor(input); err != nil {
			return fmt.Errorf("label for case %q is invalid: %w", label.CaseID, err)
		}
	}
	for caseID := range caseInputs {
		if _, exists := seen[caseID]; !exists {
			return fmt.Errorf("missing label for case %q", caseID)
		}
	}
	return nil
}

type validationGenerator struct{}

func (validationGenerator) Generate(context.Context, GenerateRequest) (GenerateResponse, error) {
	return GenerateResponse{}, fmt.Errorf("validation generator must never be called")
}
