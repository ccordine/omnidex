package assemblyline

import (
	"fmt"
	"strings"
	"unicode/utf8"
)

// SemanticUncertaintyContract is code-owned justification for one bounded
// semantic call. These descriptions are not execution evidence or model context.
// WorkKind selects the current answers; there is no separate versioned identity.
// Registry lookups return values, so callers cannot mutate the registry.
type SemanticUncertaintyContract struct {
	WorkKind                WorkKind `json:"work_kind"`
	ExactQuestion           string   `json:"exact_question"`
	DeterministicLimitation string   `json:"deterministic_limitation"`
	RequiredInformation     string   `json:"required_information"`
	SingleResult            string   `json:"single_result"`
	DeterministicConsumer   string   `json:"deterministic_consumer"`
}

// SemanticUncertaintyContractForWorkKind resolves the one registered
// uncertainty contract for kind. There is deliberately no generic contract.
func SemanticUncertaintyContractForWorkKind(
	kind WorkKind,
) (SemanticUncertaintyContract, error) {
	contract, ok := registeredSemanticUncertaintyContract(kind)
	if !ok {
		return SemanticUncertaintyContract{}, fmt.Errorf(
			"semantic uncertainty contract for work kind %q is not registered", kind,
		)
	}
	if err := contract.Validate(); err != nil {
		return SemanticUncertaintyContract{}, fmt.Errorf(
			"semantic uncertainty contract for %q: %w", kind, err,
		)
	}
	return contract, nil
}

func registeredSemanticUncertaintyContract(
	kind WorkKind,
) (SemanticUncertaintyContract, bool) {
	resolvers := [...]func(WorkKind) (SemanticUncertaintyContract, bool){
		applicationSemanticUncertaintyContract,
		repositorySemanticUncertaintyContract,
		databaseSemanticUncertaintyContract,
		webSemanticUncertaintyContract,
		codingSemanticUncertaintyContract,
	}
	for _, resolve := range resolvers {
		if contract, ok := resolve(kind); ok {
			return contract, true
		}
	}
	return SemanticUncertaintyContract{}, false
}

func semanticUncertaintyContract(
	kind WorkKind,
	question string,
	limitation string,
	requiredInformation string,
	singleResult string,
	consumer string,
) SemanticUncertaintyContract {
	return SemanticUncertaintyContract{
		WorkKind: kind, ExactQuestion: question,
		DeterministicLimitation: limitation,
		RequiredInformation:     requiredInformation,
		SingleResult:            singleResult,
		DeterministicConsumer:   consumer,
	}
}

func (contract SemanticUncertaintyContract) Validate() error {
	if !validWorkKind(contract.WorkKind) {
		return fmt.Errorf("work kind %q is unsupported", contract.WorkKind)
	}
	fields := []struct {
		name  string
		value string
	}{
		{"exact question", contract.ExactQuestion},
		{"deterministic limitation", contract.DeterministicLimitation},
		{"required information", contract.RequiredInformation},
		{"single result", contract.SingleResult},
		{"deterministic consumer", contract.DeterministicConsumer},
	}
	for _, field := range fields {
		if err := validateSemanticUncertaintyContractField(field.name, field.value); err != nil {
			return err
		}
	}
	registered, ok := registeredSemanticUncertaintyContract(contract.WorkKind)
	if !ok {
		return fmt.Errorf("work kind %q has no registered semantic uncertainty", contract.WorkKind)
	}
	if contract != registered {
		return fmt.Errorf("contract differs from the exact code-owned registry value")
	}
	return nil
}

func validateSemanticUncertaintyContractField(name, value string) error {
	if value == "" || value != strings.TrimSpace(value) {
		return fmt.Errorf("%s must be non-empty and exactly trimmed", name)
	}
	if len(value) > 512 || !utf8.ValidString(value) || strings.ContainsAny(value, "\x00\r\n") {
		return fmt.Errorf("%s must be one bounded UTF-8 line", name)
	}
	return nil
}
