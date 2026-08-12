package objectiveworkload

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"unicode/utf8"
)

const (
	maxAuthorityBytes         = 8 * 1024
	maxIdentityBytes          = 128
	maxRequirementBytes       = 1024
	maxRequirements           = 96
	maxObjectives             = 1 + 3*maxRequirements
	maxArtifactBytes          = 16 * 1024
	maxResponseJobIDBytes     = 128
	maxResponseCandidateBytes = 12 * 1024
	maxStationCalls           = 128
	maxTransitions            = 512
	maxObjectiveDepth         = 32
)

var (
	ErrInvalidAuthority = errors.New("invalid objective workload authority")
	ErrInvalidCompile   = errors.New("invalid objective workload compilation")
	ErrCompileBound     = errors.New("objective workload compilation bound exceeded")
	ErrInvalidGraph     = errors.New("invalid objective workload graph")
	ErrInvalidRun       = errors.New("invalid objective workload run")
	ErrRunBound         = errors.New("objective workload transition bound exceeded")
	ErrDeadlock         = errors.New("objective workload deterministic deadlock")
	ErrOperation        = errors.New("objective workload code operation failed")
	ErrArtifact         = errors.New("invalid objective workload artifact")
)

type EvidenceClass string

const EvidencePrimitiveContaminatedNonAutonomy EvidenceClass = "primitive_contaminated_non_autonomy"

type CompilationID string
type WorkloadID string
type RequirementID string
type ObjectiveID string
type ArtifactID string
type GapID string

type Authority struct {
	Text   string
	SHA256 string
}

func newAuthority(text string) (Authority, error) {
	if len(text) == 0 || len(text) > maxAuthorityBytes || !utf8.ValidString(text) ||
		strings.ContainsRune(text, 0) || strings.TrimSpace(text) == "" {
		return Authority{}, fmt.Errorf(
			"%w: exact text must be nonempty bounded UTF-8 without NUL",
			ErrInvalidAuthority,
		)
	}
	return Authority{Text: text, SHA256: digestBytes([]byte(text))}, nil
}

func validateAuthority(authority Authority) error {
	want, err := newAuthority(authority.Text)
	if err != nil {
		return err
	}
	if authority.SHA256 != want.SHA256 {
		return fmt.Errorf("%w: digest does not match exact text", ErrInvalidAuthority)
	}
	return nil
}

func validIdentity(value string) bool {
	return value != "" && len(value) <= maxIdentityBytes && utf8.ValidString(value) &&
		!strings.ContainsAny(value, "\x00 \t\r\n")
}

func digestBytes(value []byte) string {
	sum := sha256.Sum256(value)
	return hex.EncodeToString(sum[:])
}

func digestFields(values ...string) string {
	hash := sha256.New()
	for _, value := range values {
		_, _ = hash.Write([]byte(value))
		_, _ = hash.Write([]byte{0})
	}
	return hex.EncodeToString(hash.Sum(nil))
}

func nilInterface(value any) bool {
	if value == nil {
		return true
	}
	reflected := reflect.ValueOf(value)
	switch reflected.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return reflected.IsNil()
	default:
		return false
	}
}
