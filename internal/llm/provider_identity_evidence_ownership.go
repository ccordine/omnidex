package llm

import "fmt"

func validateProviderIdentityOwnershipBounds(evidence ProviderIdentityEvidence) error {
	if len(evidence.Operations) != 5 {
		return fmt.Errorf("provider identity ownership requires exactly five operations")
	}
	total := 0
	for index := range evidence.Operations {
		requestBytes := len(evidence.Operations[index].Request)
		responseBytes := len(evidence.Operations[index].ResponseCapture)
		if requestBytes > MaxProviderIdentityComponentBytes ||
			responseBytes > MaxProviderIdentityComponentBytes+1 ||
			requestBytes > MaxProviderIdentityEvidenceBytes-total {
			return fmt.Errorf("provider identity operation %d exceeds its ownership bound", index)
		}
		total += requestBytes
		if responseBytes > MaxProviderIdentityEvidenceBytes-total {
			return fmt.Errorf("provider identity aggregate exceeds its ownership bound")
		}
		total += responseBytes
	}
	return nil
}

// OwnBoundedProviderIdentityEvidence checks every raw provider slice before
// allocating the immutable copy used by validation and persistence.
func OwnBoundedProviderIdentityEvidence(
	evidence ProviderIdentityEvidence,
) (ProviderIdentityEvidence, error) {
	if err := validateProviderIdentityOwnershipBounds(evidence); err != nil {
		return ProviderIdentityEvidence{}, err
	}
	return evidence.Clone(), nil
}

func ownBoundedObservedProviderIdentity(
	observed ObservedProviderIdentity,
) (ObservedProviderIdentity, error) {
	owned, err := OwnBoundedProviderIdentityEvidence(observed.Evidence)
	if err != nil {
		return ObservedProviderIdentity{}, err
	}
	observed.Evidence = owned
	return observed, nil
}
