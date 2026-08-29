package assemblyline

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
)

const (
	// PortableJobSchemaV2 identifies work whose model result uses only its
	// registered raw response transport.
	PortableJobSchemaV2      = "omnidex.portable-job.v2"
	maxPortableResourceBytes = 128 * 1024
	// MaxPortableSemanticCandidateBytes is the shared gross ceiling for raw
	// semantic and bounded-source candidates.
	MaxPortableSemanticCandidateBytes = maxPortableResourceBytes
	// Raw provider output is captured and bounded before it becomes a portable
	// result. This duplicate ceiling protects test and alternate in-process
	// executors at the same coarse 16 MiB provider-response boundary.
	MaxPortableRawCandidateBytes = 16 * 1024 * 1024
	// Portable byte limits are coarse resource ceilings. Station field bounds
	// and the exact provider token budget remain the semantic/context authority.
	maxPortablePayloadBytes      = maxPortableResourceBytes
	maxPortableCandidateBytes    = MaxPortableSemanticCandidateBytes
	maxPortableRawCandidateBytes = MaxPortableRawCandidateBytes
)

type WorkKind string

type PortableJob struct {
	Schema           string          `json:"schema"`
	ID               string          `json:"id"`
	Kind             WorkKind        `json:"kind"`
	Payload          json.RawMessage `json:"payload"`
	SourceProjection string          `json:"source_projection,omitempty"`
}

func newPortableJob(kind WorkKind, input any) (PortableJob, error) {
	payload, err := json.Marshal(input)
	if err != nil {
		return PortableJob{}, fmt.Errorf("encode portable %s input: %w", kind, err)
	}
	job := PortableJob{Schema: PortableJobSchemaV2, Kind: kind, Payload: payload}
	job.ID = portableJobDigest(job.Schema, job.Kind, job.Payload)
	if err := job.Validate(); err != nil {
		return PortableJob{}, err
	}
	return job, nil
}

func (job PortableJob) Validate() error {
	if err := job.validateIdentity(); err != nil {
		return err
	}
	return validatePortableJobPayload(job.Kind, job.Payload)
}

// ValidatePortableJobForRenderer binds payload shape to the renderer that
// owns it. Current code cannot reinterpret a frozen historical input through
// a newer prompt contract, and historical replay cannot accept a current-only
// input as if those bytes had existed under an older renderer.
func ValidatePortableJobForRenderer(job PortableJob, renderer string) error {
	if !IsReplayablePortableRenderer(renderer) {
		return fmt.Errorf("portable renderer %q is not registered", renderer)
	}
	if err := job.validateIdentity(); err != nil {
		return err
	}
	return validatePortableJobPayloadForRenderer(job.Kind, job.Payload, renderer)
}

func (job PortableJob) validateIdentity() error {
	if job.Schema != PortableJobSchemaV2 {
		return fmt.Errorf("portable job schema must be %q", PortableJobSchemaV2)
	}
	if !validWorkKind(job.Kind) {
		return fmt.Errorf("portable job kind %q is unsupported", job.Kind)
	}
	if len(job.Payload) == 0 {
		return fmt.Errorf("portable job payload is empty")
	}
	if len(job.Payload) > maxPortablePayloadBytes {
		return fmt.Errorf(
			"portable job payload exceeds gross resource ceiling of %d bytes",
			maxPortablePayloadBytes,
		)
	}
	if err := validatePortableJobSourceProjection(job); err != nil {
		return err
	}
	expectedID := portableJobProjectionDigest(
		job.Schema, job.Kind, job.Payload, job.SourceProjection,
	)
	if job.ID != expectedID {
		return fmt.Errorf("portable job id does not match its immutable content")
	}

	return nil
}

func portableJobDigest(schema string, kind WorkKind, payload []byte) string {
	return portableJobProjectionDigest(schema, kind, payload, "")
}

func portableJobProjectionDigest(
	schema string,
	kind WorkKind,
	payload []byte,
	sourceProjection string,
) string {
	hash := sha256.New()
	_, _ = hash.Write([]byte(schema))
	_, _ = hash.Write([]byte{0})
	_, _ = hash.Write([]byte(kind))
	_, _ = hash.Write([]byte{0})
	_, _ = hash.Write(payload)
	if sourceProjection != "" {
		_, _ = hash.Write([]byte{0})
		_, _ = hash.Write([]byte(sourceProjection))
	}
	return hex.EncodeToString(hash.Sum(nil))
}
