package assemblyline

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
)

type PortableResult struct {
	JobID      string                    `json:"job_id"`
	Candidate  string                    `json:"candidate"`
	Projection *PortableResultProjection `json:"-"`
}

type PortableResultProjectionKind string

const (
	PortableResultProjectionExactResponse      PortableResultProjectionKind = "exact_response"
	PortableResultProjectionSourceDeclaration  PortableResultProjectionKind = "source_declaration"
	PortableResultProjectionTypeScriptFunction PortableResultProjectionKind = "typescript_function"
)

// PortableResultProjection is code-owned metadata binding the accepted leaf
// to an exact byte span in Candidate. Candidate remains the complete untrusted
// final response and is preserved separately as call evidence.
type PortableResultProjection struct {
	Kind                 PortableResultProjectionKind
	Source               string
	SourceResponseSHA256 string
	SourceSHA256         string
	StartByte            int
	EndByte              int
	RawBytes             int
	DiscardedBytes       int
}

func (result PortableResult) ValidateFor(job PortableJob) error {
	if result.JobID != job.ID {
		return fmt.Errorf("portable result job id does not match the claimed job")
	}
	if strings.TrimSpace(result.Candidate) == "" {
		return fmt.Errorf("portable result candidate is empty")
	}
	if len(result.Candidate) > maxPortableRawCandidateBytes {
		return fmt.Errorf(
			"portable result candidate exceeds gross resource ceiling of %d bytes",
			maxPortableRawCandidateBytes,
		)
	}
	if result.Projection != nil {
		if err := result.Projection.ValidateFor(result.Candidate); err != nil {
			return fmt.Errorf("portable result projection: %w", err)
		}
	}
	return nil
}

func NewExactPortableResultProjection(raw string) (PortableResultProjection, error) {
	projection := PortableResultProjection{
		Kind: PortableResultProjectionExactResponse, Source: raw,
		SourceResponseSHA256: portableProjectionSHA256(raw),
		SourceSHA256:         portableProjectionSHA256(raw),
		StartByte:            0, EndByte: len(raw), RawBytes: len(raw),
	}
	if err := projection.ValidateFor(raw); err != nil {
		return PortableResultProjection{}, err
	}
	return projection, nil
}

func (projection TypeScriptFunctionProjection) PortableResultProjection() (PortableResultProjection, error) {
	result := PortableResultProjection{
		Kind: PortableResultProjectionTypeScriptFunction, Source: projection.Source,
		SourceResponseSHA256: projection.RawSHA256, SourceSHA256: projection.SourceSHA256,
		StartByte: projection.StartByte, EndByte: projection.EndByte,
		RawBytes: projection.RawBytes, DiscardedBytes: projection.DiscardedBytes,
	}
	if projection.SourceBytes != len(projection.Source) ||
		projection.RawBytes != len(projection.Source) ||
		projection.RawSHA256 != projection.SourceSHA256 {
		return PortableResultProjection{}, fmt.Errorf("TypeScript projection metadata is not exact")
	}
	if err := result.ValidateFor(projection.Source); err != nil {
		return PortableResultProjection{}, err
	}
	return result, nil
}

func (projection PortableResultProjection) ValidateFor(raw string) error {
	if projection.Kind != PortableResultProjectionExactResponse &&
		projection.Kind != PortableResultProjectionSourceDeclaration &&
		projection.Kind != PortableResultProjectionTypeScriptFunction {
		return fmt.Errorf("projection kind %q is not registered", projection.Kind)
	}
	if projection.RawBytes != len(raw) || projection.SourceResponseSHA256 != portableProjectionSHA256(raw) {
		return fmt.Errorf("projection raw response identity is invalid")
	}
	if projection.StartByte < 0 || projection.EndByte <= projection.StartByte ||
		projection.EndByte > len(raw) {
		return fmt.Errorf("projection source span is invalid")
	}
	if projection.Source != raw[projection.StartByte:projection.EndByte] ||
		projection.SourceSHA256 != portableProjectionSHA256(projection.Source) {
		return fmt.Errorf("projection source differs from its exact response span")
	}
	if projection.DiscardedBytes != len(raw)-len(projection.Source) {
		return fmt.Errorf("projection discarded byte count is invalid")
	}
	if projection.StartByte != 0 || projection.EndByte != len(raw) ||
		projection.DiscardedBytes != 0 || projection.Source != raw {
		return fmt.Errorf("projection is not the complete exact response")
	}
	return nil
}

func portableProjectionSHA256(value string) string {
	digest := sha256.Sum256([]byte(value))
	return hex.EncodeToString(digest[:])
}
