package modelgauntlet

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"

	"github.com/gryph/omnidex/internal/assemblyline"
)

const (
	RequirementPartitionCasesSchemaV1  = "omnidex.model-gauntlet.requirement-partition-cases.v1"
	RequirementPartitionLabelsSchemaV1 = "omnidex.model-gauntlet.requirement-partition-labels.v1"
)

type requirementPartitionCasesFile struct {
	Schema string                     `json:"schema"`
	Cases  []RequirementPartitionCase `json:"cases"`
}

type requirementPartitionLabelsFile struct {
	Schema string                      `json:"schema"`
	Labels []RequirementPartitionLabel `json:"labels"`
}

func LoadRequirementPartitionCases(path string) ([]RequirementPartitionCase, error) {
	raw, err := readGauntletFile(path)
	if err != nil {
		return nil, fmt.Errorf("load requirement partition cases: %w", err)
	}
	var input requirementPartitionCasesFile
	if err := decodeExactJSON(string(raw), &input); err != nil {
		return nil, fmt.Errorf("decode requirement partition cases: %w", err)
	}
	if input.Schema != RequirementPartitionCasesSchemaV1 {
		return nil, fmt.Errorf("requirement partition cases schema must be %q", RequirementPartitionCasesSchemaV1)
	}
	config := RequirementPartitionConfig{
		StableModel: "validation", ReasoningModel: "validation", ContextTokens: 16384, KeepAlive: "1s",
	}
	if _, err := requirementPartitionAdvisoryCases(config, input.Cases, validationGenerator{}); err != nil {
		return nil, err
	}
	return input.Cases, nil
}

func LoadRequirementPartitionLabels(
	path string,
	cases []RequirementPartitionCase,
) ([]RequirementPartitionLabel, string, error) {
	raw, err := readGauntletFile(path)
	if err != nil {
		return nil, "", fmt.Errorf("load requirement partition labels: %w", err)
	}
	var input requirementPartitionLabelsFile
	if err := decodeExactJSON(string(raw), &input); err != nil {
		return nil, "", fmt.Errorf("decode requirement partition labels: %w", err)
	}
	if input.Schema != RequirementPartitionLabelsSchemaV1 {
		return nil, "", fmt.Errorf("requirement partition labels schema must be %q", RequirementPartitionLabelsSchemaV1)
	}
	inputs := make(map[string]assemblyline.RequirementPartitionInput, len(cases))
	for _, fixture := range cases {
		if _, exists := inputs[fixture.ID]; exists {
			return nil, "", fmt.Errorf("requirement partition case %q is duplicated", fixture.ID)
		}
		inputs[fixture.ID] = fixture.Input
	}
	if _, err := validateRequirementPartitionLabels(inputs, input.Labels); err != nil {
		return nil, "", err
	}
	sum := sha256.Sum256(raw)
	return input.Labels, hex.EncodeToString(sum[:]), nil
}

func WriteRequirementPartitionResult(path string, result RequirementPartitionResult) error {
	if result.Schema != RequirementPartitionResultSchemaV1 {
		return fmt.Errorf("requirement partition result schema must be %q", RequirementPartitionResultSchemaV1)
	}
	return writeExclusiveGauntletJSON(path, "requirement partition result", result)
}
