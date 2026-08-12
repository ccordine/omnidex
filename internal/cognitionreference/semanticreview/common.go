package semanticreview

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"unicode/utf8"
)

const (
	maxIdentityBytes     = 128
	maxArtifactBytes     = 2 * 1024
	maxSpecificationText = 2 * 1024
	maxReviewRounds      = 8
	maxReviewEvidence    = 16
	maxReviewCandidates  = 8
	maxCorrectionRules   = 7
)

var (
	ErrInvalidObjective        = errors.New("invalid semantic review objective")
	ErrInvalidArtifact         = errors.New("invalid semantic review artifact")
	ErrInvalidSpecification    = errors.New("invalid semantic review specification")
	ErrInvalidRuleRegistry     = errors.New("invalid semantic review correction rule registry")
	ErrInvalidExecutorRegistry = errors.New("invalid semantic review correction executor registry")
	ErrInvalidMachine          = errors.New("invalid semantic review machine")
	ErrVerification            = errors.New("semantic review deterministic verification failed")
	ErrCorrection              = errors.New("semantic review code correction failed")
	ErrReviewRoundBound        = errors.New("semantic review round bound exceeded")
	ErrCompletion              = errors.New("semantic review completion rejected")
)

type EvidenceClass string

const EvidencePrimitiveContaminatedNonAutonomy EvidenceClass = "primitive_contaminated_non_autonomy"

func validIdentity(value string) bool {
	return value != "" && len(value) <= maxIdentityBytes && utf8.ValidString(value) &&
		!strings.ContainsAny(value, "\x00 \t\r\n")
}

func validText(value string, maxBytes int) bool {
	return value != "" && len(value) <= maxBytes && utf8.ValidString(value) &&
		!strings.ContainsRune(value, 0) && strings.TrimSpace(value) == value
}

func validExactBytes(value []byte, maxBytes int) bool {
	return len(value) > 0 && len(value) <= maxBytes && utf8.Valid(value) &&
		bytes.IndexByte(value, 0) < 0 && bytes.Equal(bytes.TrimSpace(value), value)
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

func requireExact[T comparable](got, want []T, label string) error {
	if !reflect.DeepEqual(got, want) {
		return fmt.Errorf("%s differs from exact registered values", label)
	}
	return nil
}
