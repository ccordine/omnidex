package llm

// NewUndispatchedProviderIdentityEvidence records that provider authority was
// unavailable before the first exact identity request. Request bytes remain
// frozen while every operation is proven undispatched.
func NewUndispatchedProviderIdentityEvidence(
	selection ProviderIdentitySelection,
) (ProviderIdentityEvidence, error) {
	if err := selection.Validate(); err != nil {
		return ProviderIdentityEvidence{}, err
	}
	tokenizer, err := ExactProviderTokenizerRequestBytes(selection)
	if err != nil {
		return ProviderIdentityEvidence{}, err
	}
	preload, err := ExactProviderPreloadRequestBytes(selection)
	if err != nil {
		return ProviderIdentityEvidence{}, err
	}
	definitions := []struct {
		operation        ProviderIdentityOperation
		method, endpoint string
		request          []byte
	}{
		{ProviderIdentityVersion, "GET", "/api/version", nil},
		{ProviderIdentityInstalled, "GET", "/api/tags", nil},
		{ProviderIdentityTokenizer, "POST", "/api/show", tokenizer},
		{ProviderIdentityPreload, "POST", "/api/generate", preload},
		{ProviderIdentityRunner, "GET", "/api/ps", nil},
	}
	operations := make([]ProviderIdentityOperationEvidence, len(definitions))
	for index, definition := range definitions {
		disposition := ProviderIdentityNotDispatched
		if index == 0 {
			disposition = ProviderIdentityTransport
		}
		operations[index], err = NewProviderIdentityOperationEvidence(
			definition.operation, definition.method, definition.endpoint,
			ProviderRequestNotDispatched, definition.request, 0, disposition, false,
			ProviderContentEncodingEvidence{}, nil,
		)
		if err != nil {
			return ProviderIdentityEvidence{}, err
		}
	}
	return NewProviderIdentityEvidence(operations)
}
