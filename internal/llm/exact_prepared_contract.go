package llm

import "fmt"

// ExactPreparedContractClient is an explicit provider capability. Cognition
// policy must reject providers that cannot enforce every PreparedModel field.
type ExactPreparedContractClient interface {
	RequireExactPreparedContract() error
	ValidateExactPreparedContract(PreparedModel) error
}

func RequireExactPreparedContract(client Client) (ExactPreparedContractClient, error) {
	exact, ok := client.(ExactPreparedContractClient)
	if !ok {
		return nil, fmt.Errorf("configured generation provider does not enforce the exact prepared contract")
	}
	if err := exact.RequireExactPreparedContract(); err != nil {
		return nil, err
	}
	return exact, nil
}
