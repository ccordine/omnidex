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
	PortableResultProjectionExactResponse PortableResultProjectionKind = "exact_response"
)

// PortableResultProjection is code-owned evidence that Candidate is the exact
// ordinary response captured from the provider. It is not model-visible and
// carries no source, schema, or workflow responsibility.
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
	if err := job.Validate(); err != nil {
		return err
	}
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
	maximum, err := PortableResponseMaximumBytesForJob(job)
	if err != nil {
		return err
	}
	if len(result.Candidate) > maximum {
		return fmt.Errorf(
			"portable result candidate exceeds %s response ceiling of %d bytes",
			job.Kind, maximum,
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

func (projection PortableResultProjection) ValidateFor(raw string) error {
	if projection.Kind != PortableResultProjectionExactResponse {
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
