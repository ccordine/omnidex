package modelgauntlet

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"

	"github.com/gryph/omnidex/internal/assemblyline"
)

type repositoryRetrievalCasesFile struct {
	Schema string                    `json:"schema"`
	Cases  []RepositoryRetrievalCase `json:"cases"`
}

type repositoryRetrievalLabelsFile struct {
	Schema string                     `json:"schema"`
	Labels []RepositoryRetrievalLabel `json:"labels"`
}

func LoadRepositoryRetrievalCases(path string) ([]RepositoryRetrievalCase, error) {
	raw, err := readGauntletFile(path)
	if err != nil {
		return nil, fmt.Errorf("load repository retrieval cases: %w", err)
	}
	var input repositoryRetrievalCasesFile
	if err := decodeExactJSON(string(raw), &input); err != nil {
		return nil, fmt.Errorf("decode repository retrieval cases: %w", err)
	}
	if input.Schema != RepositoryRetrievalCasesSchemaV1 {
		return nil, fmt.Errorf("repository retrieval cases schema must be %q", RepositoryRetrievalCasesSchemaV1)
	}
	config := RepositoryRetrievalConfig{
		StableModel: "validation", ReasoningModel: "validation", ContextTokens: 16384, KeepAlive: "1s",
	}
	if _, err := repositoryRetrievalAdvisoryCases(config, input.Cases, validationGenerator{}); err != nil {
		return nil, err
	}
	return input.Cases, nil
}

func LoadRepositoryRetrievalLabels(
	path string,
	cases []RepositoryRetrievalCase,
) ([]RepositoryRetrievalLabel, string, error) {
	raw, err := readGauntletFile(path)
	if err != nil {
		return nil, "", fmt.Errorf("load repository retrieval labels: %w", err)
	}
	var input repositoryRetrievalLabelsFile
	if err := decodeExactJSON(string(raw), &input); err != nil {
		return nil, "", fmt.Errorf("decode repository retrieval labels: %w", err)
	}
	if input.Schema != RepositoryRetrievalLabelsSchemaV1 {
		return nil, "", fmt.Errorf("repository retrieval labels schema must be %q", RepositoryRetrievalLabelsSchemaV1)
	}
	inputs := make(map[string]assemblyline.RepositoryRetrievalInput, len(cases))
	for _, fixture := range cases {
		if _, duplicate := inputs[fixture.ID]; duplicate {
			return nil, "", fmt.Errorf("repository retrieval case %q is duplicated", fixture.ID)
		}
		inputs[fixture.ID] = fixture.Input
	}
	if _, err := validateRepositoryRetrievalLabels(inputs, input.Labels); err != nil {
		return nil, "", err
	}
	sum := sha256.Sum256(raw)
	return input.Labels, hex.EncodeToString(sum[:]), nil
}

func WriteRepositoryRetrievalResult(path string, result RepositoryRetrievalResult) error {
	if result.Schema != RepositoryRetrievalResultSchemaV1 {
		return fmt.Errorf("repository retrieval result schema must be %q", RepositoryRetrievalResultSchemaV1)
	}
	return writeExclusiveGauntletJSON(path, "repository retrieval result", result)
}
