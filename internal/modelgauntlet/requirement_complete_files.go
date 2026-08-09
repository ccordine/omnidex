package modelgauntlet

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
)

type completeRequirementCasesFile struct {
	Schema string                    `json:"schema"`
	Cases  []CompleteRequirementCase `json:"cases"`
}

type completeRequirementLabelsFile struct {
	Schema string                     `json:"schema"`
	Labels []CompleteRequirementLabel `json:"labels"`
}

func LoadCompleteRequirementCases(path string) ([]CompleteRequirementCase, string, error) {
	raw, err := readGauntletFile(path)
	if err != nil {
		return nil, "", fmt.Errorf("load complete requirement cases: %w", err)
	}
	var input completeRequirementCasesFile
	if err := decodeExactJSON(string(raw), &input); err != nil {
		return nil, "", fmt.Errorf("decode complete requirement cases: %w", err)
	}
	if input.Schema != CompleteRequirementCasesSchemaV1 {
		return nil, "", fmt.Errorf("complete requirement cases schema must be %q", CompleteRequirementCasesSchemaV1)
	}
	config := CompleteRequirementConfig{
		StableModel: "validation", ReasoningModel: "validation", ContextTokens: 16384,
		KeepAlive: "1s", Repetitions: 1, CasesSHA256: strings.Repeat("0", 64),
		HardwareClass: "validation", Backend: "validation",
	}
	if err := validateCompleteRequirementRun(context.Background(), config, input.Cases, validationGenerator{}); err != nil {
		return nil, "", err
	}
	sum := sha256.Sum256(raw)
	return input.Cases, hex.EncodeToString(sum[:]), nil
}

func LoadCompleteRequirementLabels(
	path string,
	cases []CompleteRequirementCase,
) ([]CompleteRequirementLabel, string, error) {
	raw, err := readGauntletFile(path)
	if err != nil {
		return nil, "", fmt.Errorf("load complete requirement labels: %w", err)
	}
	var input completeRequirementLabelsFile
	if err := decodeExactJSON(string(raw), &input); err != nil {
		return nil, "", fmt.Errorf("decode complete requirement labels: %w", err)
	}
	if input.Schema != CompleteRequirementLabelsSchemaV1 {
		return nil, "", fmt.Errorf("complete requirement labels schema must be %q", CompleteRequirementLabelsSchemaV1)
	}
	sources, err := completeCaseSources(cases)
	if err != nil {
		return nil, "", err
	}
	if _, err := validateCompleteRequirementLabels(sources, input.Labels); err != nil {
		return nil, "", err
	}
	sum := sha256.Sum256(raw)
	return input.Labels, hex.EncodeToString(sum[:]), nil
}

func WriteCompleteRequirementResult(path string, result CompleteRequirementResult) error {
	if result.Schema != CompleteRequirementResultSchemaV1 {
		return fmt.Errorf("complete requirement result schema must be %q", CompleteRequirementResultSchemaV1)
	}
	return writeExclusiveGauntletJSON(path, "complete requirement result", result)
}
