package retrieval

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
)

const maxRetrievalQueryBytes = 4 * 1024

// NewQueryBinding returns an opaque, operation-bound identity for one exact
// retrieval query. The plaintext query remains code authority and is never a
// field of EvidencePack.
func NewQueryBinding(operation Operation, query string) (string, error) {
	if err := operation.Validate(); err != nil {
		return "", err
	}
	if err := validateRetrievalQuery(query); err != nil {
		return "", err
	}
	hash := sha256.New()
	_, _ = hash.Write([]byte(operation))
	_, _ = hash.Write([]byte{0})
	_, _ = hash.Write([]byte(query))
	return "query_binding_" + hex.EncodeToString(hash.Sum(nil)), nil
}

func validateRetrievalQuery(query string) error {
	if query == "" || query != strings.TrimSpace(query) || len([]byte(query)) > maxRetrievalQueryBytes {
		return fmt.Errorf(
			"repository evidence retrieval query requires 1-%d trimmed bytes",
			maxRetrievalQueryBytes,
		)
	}
	return nil
}

func validQueryBinding(value string) bool {
	if !strings.HasPrefix(value, "query_binding_") {
		return false
	}
	decoded, err := hex.DecodeString(strings.TrimPrefix(value, "query_binding_"))
	return err == nil && len(decoded) == sha256.Size
}
