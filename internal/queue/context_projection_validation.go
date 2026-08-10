package queue

import (
	"context"
	"fmt"
	"math"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/gryph/omnidex/internal/contextbuilder"
)

func validateContextProjectionStore(
	r *Repository,
	ctx context.Context,
	authority ContextProjectionAuthority,
	projection contextbuilder.Projection,
) error {
	if err := validateStepAttemptAuthority(authority.StepAttemptAuthority); err != nil {
		return fmt.Errorf("%w: %v", ErrInvalidContextProjection, err)
	}
	if err := validateContextProjectionExact(authority.WorkKind, "work kind", 256); err != nil {
		return err
	}
	if !validContextProjectionMode(authority.Mode) {
		return fmt.Errorf("%w: context projection mode %q is not registered", ErrInvalidContextProjection, authority.Mode)
	}
	if err := projection.Validate(); err != nil {
		return fmt.Errorf("%w: %v", ErrInvalidContextProjection, err)
	}
	if len(projection.WorkID) != 64 || !llmEvidenceLowerHex(projection.WorkID) {
		return fmt.Errorf("%w: work ID must be one lowercase SHA-256 digest", ErrInvalidContextProjection)
	}
	if len(projection.Selected) > maxContextProjectionSelected ||
		len(projection.Selected)+len(projection.Omitted) > maxContextProjectionRecords {
		return fmt.Errorf("%w: reference count exceeds the durable projection budget", ErrInvalidContextProjection)
	}
	if projection.WorkingSetVersion > math.MaxInt64 {
		return fmt.Errorf("%w: working-set version exceeds PostgreSQL BIGINT", ErrInvalidContextProjection)
	}
	return validateContextProjectionRepository(r, ctx)
}

func validContextProjectionMode(mode ContextProjectionMode) bool {
	return mode == ContextProjectionModeShadow || mode == ContextProjectionModeLive
}

func validateContextProjectionRepository(r *Repository, ctx context.Context) error {
	if ctx == nil {
		return fmt.Errorf("%w: context is required", ErrInvalidContextProjection)
	}
	if r == nil || r.pool == nil {
		return fmt.Errorf("%w: PostgreSQL repository is required", ErrInvalidContextProjection)
	}
	return nil
}

func validateContextProjectionExact(value, field string, maxBytes int) error {
	if value == "" || !utf8.ValidString(value) || strings.ContainsRune(value, '\x00') ||
		len([]byte(value)) > maxBytes || strings.TrimSpace(value) != value ||
		strings.IndexFunc(value, unicode.IsSpace) >= 0 {
		return fmt.Errorf("%w: %s must be one bounded whitespace-free exact value", ErrInvalidContextProjection, field)
	}
	return nil
}

func validContextProjectionID(id string) bool {
	const prefix = "context_projection_"
	return strings.HasPrefix(id, prefix) && len(id) == len(prefix)+64 &&
		llmEvidenceLowerHex(strings.TrimPrefix(id, prefix))
}
