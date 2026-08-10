package cognition

import "fmt"

type ContextProjectionID string
type WorkingSetID string

// ContextProjectionRef binds a model call to one immutable software-defined
// context projection without embedding the projected material itself.
type ContextProjectionRef struct {
	ID                ContextProjectionID `json:"id"`
	SHA256            string              `json:"sha256"`
	WorkingSetID      WorkingSetID        `json:"working_set_id"`
	WorkingSetVersion uint64              `json:"working_set_version"`
	RendererVersion   string              `json:"renderer_version"`
}

func (ref ContextProjectionRef) Validate() error {
	if err := validateIdentity(string(ref.ID), "context projection ID"); err != nil {
		return fmt.Errorf("%w: %w", ErrInvalidContextProjection, err)
	}
	if !validSHA256(ref.SHA256) {
		return fmt.Errorf("%w: hash must be 64 lowercase hex characters", ErrInvalidContextProjection)
	}
	if err := validateIdentity(string(ref.WorkingSetID), "projection working-set ID"); err != nil {
		return fmt.Errorf("%w: %w", ErrInvalidContextProjection, err)
	}
	if ref.WorkingSetVersion == 0 {
		return fmt.Errorf("%w: working-set version must be positive", ErrInvalidContextProjection)
	}
	if err := validateVersion(ref.RendererVersion, "projection renderer version"); err != nil {
		return fmt.Errorf("%w: %w", ErrInvalidContextProjection, err)
	}
	return nil
}
