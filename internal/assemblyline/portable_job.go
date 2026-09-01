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
	Schema  string          `json:"schema"`
	ID      string          `json:"id"`
	Kind    WorkKind        `json:"kind"`
	Payload json.RawMessage `json:"payload"`
}

func (job PortableJob) Validate() error {
	if job.Schema != PortableJobSchemaV2 || !validWorkKind(job.Kind) {
		return fmt.Errorf("portable job schema or work kind is not registered")
	}
	if len(job.Payload) == 0 || len(job.Payload) > maxPortablePayloadBytes ||
		!json.Valid(job.Payload) {
		return fmt.Errorf("portable job payload is not one bounded JSON value")
	}
	if job.ID != portableJobDigest(job.Schema, job.Kind, job.Payload) {
		return fmt.Errorf("portable job ID differs from its code-owned content")
	}
	return nil
}

func newPortableJob(kind WorkKind, input any) (PortableJob, error) {
	payload, err := json.Marshal(input)
	if err != nil {
		return PortableJob{}, fmt.Errorf("encode portable %s input: %w", kind, err)
	}
	job := PortableJob{Schema: PortableJobSchemaV2, Kind: kind, Payload: payload}
	job.ID = portableJobDigest(job.Schema, job.Kind, job.Payload)
	return job, nil
}

func portableJobDigest(schema string, kind WorkKind, payload []byte) string {
	hash := sha256.New()
	_, _ = hash.Write([]byte(schema))
	_, _ = hash.Write([]byte{0})
	_, _ = hash.Write([]byte(kind))
	_, _ = hash.Write([]byte{0})
	_, _ = hash.Write(payload)
	return hex.EncodeToString(hash.Sum(nil))
}
