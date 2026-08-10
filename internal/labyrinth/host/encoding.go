package host

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"

	"github.com/gryph/omnidex/internal/cognition"
)

const maxReceiptJSONBytes = 256 * 1024

func encodeExact(value any) ([]byte, string, error) {
	raw, err := json.Marshal(value)
	if err != nil {
		return nil, "", err
	}
	if len(raw) == 0 || len(raw) > maxReceiptJSONBytes {
		return nil, "", fmt.Errorf("%w: encoded receipt has %d bytes", ErrReceiptCorrupt, len(raw))
	}
	digest := sha256.Sum256(raw)
	return raw, hex.EncodeToString(digest[:]), nil
}

func decodeExact(raw []byte, digest string, target any) error {
	if len(raw) == 0 || len(raw) > maxReceiptJSONBytes {
		return fmt.Errorf("%w: encoded receipt has %d bytes", ErrReceiptCorrupt, len(raw))
	}
	actual := sha256.Sum256(raw)
	if hex.EncodeToString(actual[:]) != digest {
		return fmt.Errorf("%w: receipt digest does not bind exact bytes", ErrReceiptCorrupt)
	}
	if err := json.Unmarshal(raw, target); err != nil {
		return fmt.Errorf("%w: decode receipt: %v", ErrReceiptCorrupt, err)
	}
	return nil
}

// actionRequestSHA256 matches the kernel's semantic idempotency identity. The
// actor is fenced separately, allowing a replacement attempt to replay an
// already committed action without changing its meaning.
func actionRequestSHA256(action cognition.RegisteredAction) (string, error) {
	request := action.Request.Clone()
	sort.Slice(request.Arguments, func(left, right int) bool {
		return request.Arguments[left].Name < request.Arguments[right].Name
	})
	evidence := append([]cognition.EvidenceRef(nil), action.EvidenceRefs...)
	sort.Slice(evidence, func(left, right int) bool {
		if evidence[left].ObservationID != evidence[right].ObservationID {
			return evidence[left].ObservationID < evidence[right].ObservationID
		}
		if evidence[left].Revision.Number != evidence[right].Revision.Number {
			return evidence[left].Revision.Number < evidence[right].Revision.Number
		}
		return evidence[left].SHA256 < evidence[right].SHA256
	})
	payload := struct {
		Schema   cognition.ActionSchemaRef `json:"schema"`
		Request  cognition.ActionRequest   `json:"request"`
		Evidence []cognition.EvidenceRef   `json:"evidence_refs"`
	}{action.Schema, request, evidence}
	_, digest, err := encodeExact(payload)
	return digest, err
}
