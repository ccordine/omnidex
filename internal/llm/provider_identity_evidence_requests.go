package llm

import (
	"bytes"
	"fmt"
)

func (evidence ProviderIdentityEvidence) Clone() ProviderIdentityEvidence {
	evidence.Operations = cloneProviderIdentityOperations(evidence.Operations)
	return evidence
}

// ValidateRequests binds every planned identity operation to the selected
// model. It applies equally to complete observations and to a failure prefix;
// an identity probe may fail, but it may never change what was probed.
func (evidence ProviderIdentityEvidence) ValidateRequests(
	selection ProviderIdentitySelection,
) error {
	if err := evidence.Validate(); err != nil {
		return err
	}
	if err := selection.Validate(); err != nil {
		return err
	}
	showRequest, err := ExactProviderTokenizerRequestBytes(selection)
	if err != nil {
		return err
	}
	preloadRequest, err := ExactProviderPreloadRequestBytes(selection)
	if err != nil {
		return err
	}
	want := [][]byte{nil, nil, showRequest, preloadRequest, nil}
	for index, operation := range evidence.Operations {
		if !bytes.Equal(operation.Request, want[index]) {
			return fmt.Errorf("provider identity operation %d changed its exact request", index)
		}
	}
	return nil
}
