package llm

import (
	"bytes"
	"encoding/base64"
	"encoding/json"

	"github.com/gryph/omnidex/internal/exactjson"
)

const (
	MaxProviderContentEncodingValues        = 64 * 1024
	MaxProviderContentEncodingRawBytes      = 64 * 1024
	MaxProviderContentEncodingEvidenceBytes = 192 * 1024
)

func EncodeProviderContentEncodingEvidence(values []string) (int, string, bool) {
	if len(values) == 0 {
		return 0, "", true
	}
	if len(values) > MaxProviderContentEncodingValues {
		return 0, "", false
	}
	encoded := make([]string, len(values))
	total := 0
	for index, value := range values {
		total += len(value)
		if total > MaxProviderContentEncodingRawBytes {
			return 0, "", false
		}
		encoded[index] = base64.StdEncoding.EncodeToString([]byte(value))
	}
	raw, err := exactjson.Canonical(encoded)
	if err != nil || len(raw) > MaxProviderContentEncodingEvidenceBytes {
		return 0, "", false
	}
	return len(values), string(raw), true
}

func validProviderContentEncoding(count int, evidence string) bool {
	if count == 0 {
		return evidence == ""
	}
	if count < 0 || count > MaxProviderContentEncodingValues ||
		len(evidence) > MaxProviderContentEncodingEvidenceBytes {
		return false
	}
	var encoded []string
	if err := json.Unmarshal([]byte(evidence), &encoded); err != nil || len(encoded) != count {
		return false
	}
	canonical, err := exactjson.Canonical(encoded)
	if err != nil || !bytes.Equal(canonical, []byte(evidence)) {
		return false
	}
	total := 0
	for _, value := range encoded {
		raw, err := base64.StdEncoding.Strict().DecodeString(value)
		if err != nil {
			return false
		}
		total += len(raw)
		if total > MaxProviderContentEncodingRawBytes {
			return false
		}
	}
	return true
}

func ProviderContentEncodingIsIdentity(count int, evidence string) bool {
	if count == 0 {
		return evidence == ""
	}
	if count != 1 || !validProviderContentEncoding(count, evidence) {
		return false
	}
	var values []string
	if json.Unmarshal([]byte(evidence), &values) != nil {
		return false
	}
	raw, err := base64.StdEncoding.Strict().DecodeString(values[0])
	return err == nil && string(raw) == "identity"
}
