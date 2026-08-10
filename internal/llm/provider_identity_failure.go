package llm

import (
	"fmt"

	"github.com/gryph/omnidex/internal/exactjson"
)

// ValidateFailure proves that exact request-scoped raw evidence actually
// represents a provider identity failure. The optional frozen expectation lets
// callers distinguish a successful observation of another provider from a
// successful observation of the expected provider.
func (evidence ProviderIdentityEvidence) ValidateFailure(
	selection ProviderIdentitySelection,
	expected *ProviderIdentityExpectation,
) error {
	if err := evidence.ValidateRequests(selection); err != nil {
		return err
	}
	for _, operation := range evidence.Operations {
		if operation.Disposition == ProviderIdentitySucceeded {
			continue
		}
		if operation.Disposition == ProviderIdentityInvalidJSON &&
			operation.ContentEncoding.IsIdentity() &&
			exactjson.ValidateUniqueObject(
				operation.ResponseCapture, "provider identity failure response",
			) == nil {
			return fmt.Errorf("provider identity invalid-JSON disposition has valid exact JSON")
		}
		return nil
	}
	derived, err := DeriveExactProviderIdentityExpectation(evidence, selection)
	if err != nil {
		return nil
	}
	if expected != nil && derived != *expected {
		return nil
	}
	return fmt.Errorf("provider identity evidence proves a successful expected observation")
}
