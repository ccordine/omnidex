package cognitionpolicy

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
)

const maxProviderGenerationMetadataCaptureBytes = 4096

// providerGenerationWireBytes preserves an exact bounded value, or an exact
// length/hash plus the deterministic limit+1 prefix proving overflow. This
// makes evidence construction total after a provider request is dispatched.
type providerGenerationWireBytes struct {
	OriginalBytes  int    `json:"original_bytes"`
	OriginalSHA256 string `json:"original_sha256"`
	Complete       bool   `json:"complete"`
	Capture        []byte `json:"capture"`
}

func newProviderGenerationWireString(value string, limit int) providerGenerationWireBytes {
	hash := sha256.New()
	_, _ = io.WriteString(hash, value)
	captureBytes := len(value)
	complete := captureBytes <= limit
	if !complete {
		captureBytes = limit + 1
	}
	return providerGenerationWireBytes{
		OriginalBytes: len(value), OriginalSHA256: hex.EncodeToString(hash.Sum(nil)),
		Complete: complete, Capture: []byte(value[:captureBytes]),
	}
}

func newProviderGenerationWireBytes(value []byte, limit int) providerGenerationWireBytes {
	digest := sha256.Sum256(value)
	captureBytes := len(value)
	complete := captureBytes <= limit
	if !complete {
		captureBytes = limit + 1
	}
	return providerGenerationWireBytes{
		OriginalBytes: len(value), OriginalSHA256: hex.EncodeToString(digest[:]),
		Complete: complete, Capture: append([]byte{}, value[:captureBytes]...),
	}
}

func (value providerGenerationWireBytes) validate(limit int) error {
	if value.OriginalBytes < 0 || !validPolicySHA256(value.OriginalSHA256) || limit < 0 {
		return fmt.Errorf("bounded provider field identity is invalid")
	}
	if value.Complete {
		if value.OriginalBytes != len(value.Capture) || len(value.Capture) > limit ||
			policySHA256(string(value.Capture)) != value.OriginalSHA256 {
			return fmt.Errorf("complete provider field differs from its identity")
		}
		return nil
	}
	if value.OriginalBytes <= limit || len(value.Capture) != limit+1 {
		return fmt.Errorf("overflow provider field lacks its exact bounded prefix")
	}
	return nil
}

func (value providerGenerationWireBytes) exact(limit int) ([]byte, bool, error) {
	if err := value.validate(limit); err != nil {
		return nil, false, err
	}
	if !value.Complete {
		return nil, false, nil
	}
	return append([]byte{}, value.Capture...), true, nil
}
