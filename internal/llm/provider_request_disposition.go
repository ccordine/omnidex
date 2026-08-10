package llm

import "fmt"

type ProviderRequestDisposition string

const (
	ProviderRequestNotDispatched   ProviderRequestDisposition = "not_dispatched"
	ProviderRequestDispatched      ProviderRequestDisposition = "dispatched"
	ProviderRequestWriteIndeterminate ProviderRequestDisposition = "write_indeterminate"
)

func (disposition ProviderRequestDisposition) Validate() error {
	switch disposition {
	case ProviderRequestNotDispatched, ProviderRequestDispatched,
		ProviderRequestWriteIndeterminate:
		return nil
	default:
		return fmt.Errorf("provider request disposition %q is not registered", disposition)
	}
}

// MayHaveReachedProvider is conservative: a partial write consumes the call
// allowance even though it cannot claim that inference occurred.
func (disposition ProviderRequestDisposition) MayHaveReachedProvider() bool {
	return disposition == ProviderRequestDispatched ||
		disposition == ProviderRequestWriteIndeterminate
}
