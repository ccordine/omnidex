package llm

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"encoding/hex"
	"fmt"
)

const (
	ProviderContentEncodingEvidenceSchemaV1 = "omnidex.provider-content-encoding-evidence.v1"
	MaxProviderContentEncodingCaptureBytes  = 64*1024 + 1
	MaxProviderContentEncodingBytes         = MaxProviderContentEncodingCaptureBytes + 1
	MaxProviderContentEncodingValues        = MaxProviderContentEncodingBytes / 8
	MaxProviderContentEncodingBase64Bytes   = ((MaxProviderContentEncodingCaptureBytes + 2) / 3) * 4
)

// ProviderContentEncodingEvidence byte-preserves every Content-Encoding value.
// Each value is framed by an unsigned 64-bit big-endian byte length before
// hashing/capture, so multiple values and arbitrary string bytes remain exact.
type ProviderContentEncodingEvidence struct {
	Schema         string `json:"schema"`
	Values         int    `json:"values"`
	Complete       bool   `json:"complete"`
	SHA256         string `json:"sha256"`
	Bytes          int64  `json:"bytes"`
	CapturedBase64 string `json:"captured_base64"`
	CapturedBytes  int    `json:"captured_bytes"`
	Uncompressed   bool   `json:"uncompressed"`
}

func NewProviderContentEncodingEvidence(
	values []string,
	uncompressed bool,
) ProviderContentEncodingEvidence {
	hash := sha256.New()
	capture := make([]byte, 0, MaxProviderContentEncodingCaptureBytes)
	total := int64(0)
	appendPart := func(part []byte) {
		_, _ = hash.Write(part)
		total += int64(len(part))
		remaining := MaxProviderContentEncodingCaptureBytes - len(capture)
		if remaining > len(part) {
			remaining = len(part)
		}
		if remaining > 0 {
			capture = append(capture, part[:remaining]...)
		}
	}
	for _, value := range values {
		var length [8]byte
		binary.BigEndian.PutUint64(length[:], uint64(len(value)))
		appendPart(length[:])
		appendPart([]byte(value))
	}
	return ProviderContentEncodingEvidence{
		Schema: ProviderContentEncodingEvidenceSchemaV1, Values: len(values),
		Complete: total <= MaxProviderContentEncodingCaptureBytes,
		SHA256:   hex.EncodeToString(hash.Sum(nil)), Bytes: total,
		CapturedBase64: base64.StdEncoding.EncodeToString(capture),
		CapturedBytes:  len(capture), Uncompressed: uncompressed,
	}
}

func (evidence ProviderContentEncodingEvidence) Validate() error {
	if evidence.Schema != ProviderContentEncodingEvidenceSchemaV1 || evidence.Values < 0 ||
		evidence.Values > MaxProviderContentEncodingValues || evidence.Bytes < 0 ||
		evidence.Bytes > MaxProviderContentEncodingBytes ||
		!providerIdentityDigest.MatchString(evidence.SHA256) ||
		evidence.CapturedBytes < 0 || evidence.CapturedBytes > MaxProviderContentEncodingCaptureBytes {
		return fmt.Errorf("provider content-encoding evidence identity is invalid")
	}
	if len(evidence.CapturedBase64) > MaxProviderContentEncodingBase64Bytes {
		return fmt.Errorf("provider content-encoding capture is outside its encoded bound")
	}
	captured, err := base64.StdEncoding.Strict().DecodeString(evidence.CapturedBase64)
	if err != nil || len(captured) != evidence.CapturedBytes {
		return fmt.Errorf("provider content-encoding capture is invalid")
	}
	if evidence.Complete {
		if evidence.Bytes != int64(len(captured)) || providerBodySHA256(captured) != evidence.SHA256 {
			return fmt.Errorf("complete provider content-encoding evidence changed")
		}
		values, err := decodeProviderContentEncodingValues(captured)
		if err != nil || len(values) != evidence.Values {
			return fmt.Errorf("provider content-encoding framing is invalid")
		}
	} else if evidence.Values < 1 || evidence.Bytes <= int64(evidence.CapturedBytes) ||
		evidence.CapturedBytes != MaxProviderContentEncodingCaptureBytes {
		return fmt.Errorf("partial provider content-encoding evidence lacks its exact prefix")
	}
	return nil
}

func (evidence ProviderContentEncodingEvidence) IsIdentity() bool {
	if evidence.Validate() != nil || evidence.Uncompressed || !evidence.Complete {
		return false
	}
	captured, _ := base64.StdEncoding.Strict().DecodeString(evidence.CapturedBase64)
	values, _ := decodeProviderContentEncodingValues(captured)
	return len(values) == 0 || (len(values) == 1 && string(values[0]) == "identity")
}

func decodeProviderContentEncodingValues(raw []byte) ([][]byte, error) {
	values := make([][]byte, 0)
	for len(raw) > 0 {
		if len(raw) < 8 {
			return nil, fmt.Errorf("content-encoding length is truncated")
		}
		length := binary.BigEndian.Uint64(raw[:8])
		raw = raw[8:]
		if length > uint64(len(raw)) {
			return nil, fmt.Errorf("content-encoding value is truncated")
		}
		values = append(values, append([]byte(nil), raw[:int(length)]...))
		raw = raw[int(length):]
	}
	return values, nil
}
