package model

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"
)

const (
	MaxCodingPlanLeaves         = 30
	MaxCodingPlanStatementBytes = 1024

	CodingPlanStateReview     CodingPlanState = "review"
	CodingPlanStateFrozen     CodingPlanState = "frozen"
	CodingPlanStateSuperseded CodingPlanState = "superseded"
	CodingPlanStateCanceled   CodingPlanState = "canceled"

	CodingPlanAnnotationGrounded             CodingPlanAnnotation = "grounded"
	CodingPlanAnnotationReasonableDerivation CodingPlanAnnotation = "reasonable_derivation"
	CodingPlanAnnotationSpeculativeReview    CodingPlanAnnotation = "speculative_review"
	CodingPlanAnnotationConcreteConflict     CodingPlanAnnotation = "concrete_scope_conflict"

	CodingPlanDecisionPending  CodingPlanDecision = "pending"
	CodingPlanDecisionApproved CodingPlanDecision = "approved"
	CodingPlanDecisionRejected CodingPlanDecision = "rejected"
)

var (
	codingPlanSHA256Pattern = regexp.MustCompile(`^[0-9a-f]{64}$`)
	codingPlanLeafIDPattern = regexp.MustCompile(`^coding_plan_leaf_[0-9a-f]{64}$`)
)

type CodingPlanState string
type CodingPlanAnnotation string
type CodingPlanDecision string
type CodingPlanLeafID string

// CodingPlan is the user-visible, code-owned authorization ledger for one
// exact job generation. Semantic receipts used by execution remain in the
// repository record and are intentionally absent from this projection.
type CodingPlan struct {
	JobID         int64            `json:"job_id"`
	Generation    int64            `json:"generation"`
	Revision      int64            `json:"revision"`
	State         CodingPlanState  `json:"state"`
	ScopeMode     CodingScopeMode  `json:"scope_mode"`
	RequestSHA256 string           `json:"request_sha256"`
	Leaves        []CodingPlanLeaf `json:"leaves"`
	CreatedAt     time.Time        `json:"created_at"`
	UpdatedAt     time.Time        `json:"updated_at"`
	FrozenAt      *time.Time       `json:"frozen_at,omitempty"`
}

type CodingPlanLeaf struct {
	ID         CodingPlanLeafID     `json:"id"`
	Statement  string               `json:"statement"`
	Annotation CodingPlanAnnotation `json:"annotation"`
	Decision   CodingPlanDecision   `json:"decision"`
}

func NewCodingPlanLeafID(statement string) (CodingPlanLeafID, error) {
	if err := validateCodingPlanStatement(statement); err != nil {
		return "", err
	}
	digest := sha256.Sum256([]byte(statement))
	return CodingPlanLeafID("coding_plan_leaf_" + hex.EncodeToString(digest[:])), nil
}

func ParseCodingPlanLeafID(value string) (CodingPlanLeafID, error) {
	if !codingPlanLeafIDPattern.MatchString(value) {
		return "", fmt.Errorf("coding plan leaf ID must match coding_plan_leaf_ plus 64 lowercase hex characters")
	}
	return CodingPlanLeafID(value), nil
}

func (state CodingPlanState) Validate() error {
	switch state {
	case CodingPlanStateReview, CodingPlanStateFrozen, CodingPlanStateSuperseded,
		CodingPlanStateCanceled:
		return nil
	default:
		return fmt.Errorf("coding plan state %q is unsupported", state)
	}
}

func (annotation CodingPlanAnnotation) Validate() error {
	switch annotation {
	case CodingPlanAnnotationGrounded, CodingPlanAnnotationReasonableDerivation,
		CodingPlanAnnotationSpeculativeReview, CodingPlanAnnotationConcreteConflict:
		return nil
	default:
		return fmt.Errorf("coding plan annotation %q is unsupported", annotation)
	}
}

func (decision CodingPlanDecision) Validate() error {
	switch decision {
	case CodingPlanDecisionPending, CodingPlanDecisionApproved, CodingPlanDecisionRejected:
		return nil
	default:
		return fmt.Errorf("coding plan decision %q is unsupported", decision)
	}
}

func (leaf CodingPlanLeaf) Validate() error {
	if _, err := ParseCodingPlanLeafID(string(leaf.ID)); err != nil {
		return err
	}
	if err := validateCodingPlanStatement(leaf.Statement); err != nil {
		return err
	}
	expected, _ := NewCodingPlanLeafID(leaf.Statement)
	if leaf.ID != expected {
		return fmt.Errorf("coding plan leaf ID does not match its exact statement")
	}
	if err := leaf.Annotation.Validate(); err != nil {
		return err
	}
	if err := leaf.Decision.Validate(); err != nil {
		return err
	}
	return nil
}

func (plan CodingPlan) Validate() error {
	if plan.JobID <= 0 || plan.Generation <= 0 || plan.Revision <= 0 {
		return fmt.Errorf("coding plan requires positive job, generation, and revision identities")
	}
	if err := plan.State.Validate(); err != nil {
		return err
	}
	if err := plan.ScopeMode.Validate(); err != nil {
		return err
	}
	if !codingPlanSHA256Pattern.MatchString(plan.RequestSHA256) {
		return fmt.Errorf("coding plan request SHA-256 must be 64 lowercase hex characters")
	}
	if plan.Leaves == nil || len(plan.Leaves) > MaxCodingPlanLeaves {
		return fmt.Errorf("coding plan leaves must be an array of at most %d entries", MaxCodingPlanLeaves)
	}
	seen := make(map[CodingPlanLeafID]struct{}, len(plan.Leaves))
	approved := 0
	for index, leaf := range plan.Leaves {
		if err := leaf.Validate(); err != nil {
			return fmt.Errorf("coding plan leaf %d: %w", index, err)
		}
		if _, duplicate := seen[leaf.ID]; duplicate {
			return fmt.Errorf("coding plan contains duplicate leaf %q", leaf.ID)
		}
		seen[leaf.ID] = struct{}{}
		if leaf.Decision == CodingPlanDecisionApproved {
			approved++
		}
		if plan.State == CodingPlanStateFrozen && leaf.Decision == CodingPlanDecisionPending {
			return fmt.Errorf("frozen coding plan contains pending leaf %q", leaf.ID)
		}
	}
	if plan.State == CodingPlanStateFrozen {
		if plan.FrozenAt == nil || plan.FrozenAt.IsZero() {
			return fmt.Errorf("frozen coding plan requires a frozen timestamp")
		}
		if approved == 0 {
			return fmt.Errorf("frozen coding plan requires at least one approved leaf")
		}
	} else if plan.State == CodingPlanStateReview && plan.FrozenAt != nil {
		return fmt.Errorf("non-frozen coding plan must not carry a frozen timestamp")
	}
	if plan.CreatedAt.IsZero() || plan.UpdatedAt.IsZero() || plan.UpdatedAt.Before(plan.CreatedAt) {
		return fmt.Errorf("coding plan timestamps are invalid")
	}
	return nil
}

func validateCodingPlanStatement(statement string) error {
	if statement == "" || statement != strings.TrimSpace(statement) ||
		!utf8.ValidString(statement) || strings.ContainsRune(statement, '\x00') ||
		len(statement) > MaxCodingPlanStatementBytes {
		return fmt.Errorf(
			"coding plan statement must be trimmed UTF-8 text of at most %d bytes",
			MaxCodingPlanStatementBytes,
		)
	}
	return nil
}
