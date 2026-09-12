package llm

import (
	"fmt"
	"strings"
)

// ProviderContentEncoding records only the transport fact consumed by response
// decoding. HTTP headers do not need their own archived byte stream or identity.
type ProviderContentEncoding string

const (
	ProviderEncodingIdentity    ProviderContentEncoding = "identity"
	ProviderEncodingUnsupported ProviderContentEncoding = "unsupported"
)

func ClassifyProviderContentEncoding(values []string, uncompressed bool) ProviderContentEncoding {
	if !uncompressed && (len(values) == 0 ||
		(len(values) == 1 && strings.EqualFold(values[0], "identity"))) {
		return ProviderEncodingIdentity
	}
	return ProviderEncodingUnsupported
}

func (encoding ProviderContentEncoding) Validate() error {
	if encoding != ProviderEncodingIdentity && encoding != ProviderEncodingUnsupported {
		return fmt.Errorf("provider content encoding is unavailable")
	}
	return nil
}

func (encoding ProviderContentEncoding) IsIdentity() bool {
	return encoding == ProviderEncodingIdentity
}
