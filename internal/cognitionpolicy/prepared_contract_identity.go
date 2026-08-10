package cognitionpolicy

import (
	"encoding/json"
	"fmt"

	"github.com/gryph/omnidex/internal/cognition"
	"github.com/gryph/omnidex/internal/llm"
)

const responseContractVersionV1 = "omnidex.cognition-response-contract.v1"

type responseContractIdentity struct {
	Version     string          `json:"version"`
	Format      string          `json:"format"`
	Schema      json.RawMessage `json:"schema"`
	Thinking    bool            `json:"thinking"`
	Temperature float64         `json:"temperature"`
}

func responseContractSHA(catalog cognition.ActionCatalog) (string, error) {
	schema, err := decisionSchemaJSON(catalog)
	if err != nil {
		return "", err
	}
	return responseContractIdentitySHA(responseContractIdentity{
		Version: responseContractVersionV1, Format: llm.ResponseFormatJSON,
		Schema: schema, Thinking: false, Temperature: 0,
	})
}

func preparedResponseContractSHA(prepared llm.PreparedModel) (string, error) {
	if prepared.Temperature == nil {
		return "", fmt.Errorf("prepared response contract has no exact temperature")
	}
	schema, err := json.Marshal(prepared.ResponseSchema)
	if err != nil {
		return "", err
	}
	return responseContractIdentitySHA(responseContractIdentity{
		Version: responseContractVersionV1, Format: prepared.ResponseFormat,
		Schema: schema, Thinking: prepared.ThinkingEnabled, Temperature: *prepared.Temperature,
	})
}

func responseContractIdentitySHA(identity responseContractIdentity) (string, error) {
	raw, err := json.Marshal(identity)
	if err != nil {
		return "", err
	}
	return policySHA256(string(raw)), nil
}
