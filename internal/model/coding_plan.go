package model

import (
	"crypto/rand"
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

	CodingPlanDecisionPending  CodingPlanDecision = "pending"
	CodingPlanDecisionApproved CodingPlanDecision = "approved"
	CodingPlanDecisionRejected CodingPlanDecision = "rejected"
)

var (
	codingPlanLeafIDPattern = regexp.MustCompile(`^coding_plan_leaf_[0-9a-f]{32}$`)
)

type CodingPlanState string
type CodingPlanDecision string
type CodingPlanLeafID string

// CodingPlan is the user-visible, code-owned authorization ledger for one
// exact job generation. Semantic receipts used by execution remain in the
// repository record and are intentionally absent from this projection.
type CodingPlan struct {
	JobID      int64            `json:"job_id"`
	Generation int64            `json:"generation"`
	Revision   int64            `json:"revision"`
	State      CodingPlanState  `json:"state"`
	Leaves     []CodingPlanLeaf `json:"leaves"`
	CreatedAt  time.Time        `json:"created_at"`
	UpdatedAt  time.Time        `json:"updated_at"`
	FrozenAt   *time.Time       `json:"frozen_at,omitempty"`
}

type CodingPlanLeaf struct {
	ID        CodingPlanLeafID   `json:"id"`
	Statement string             `json:"statement"`
	Decision  CodingPlanDecision `json:"decision"`
}

func NewCodingPlanLeafID() (CodingPlanLeafID, error) {
	var identity [16]byte
	if _, err := rand.Read(identity[:]); err != nil {
		return "", fmt.Errorf("allocate coding plan leaf identity: %w", err)
	}
	return CodingPlanLeafID("coding_plan_leaf_" + hex.EncodeToString(identity[:])), nil
}

func ParseCodingPlanLeafID(value string) (CodingPlanLeafID, error) {
	if !codingPlanLeafIDPattern.MatchString(value) {
		return "", fmt.Errorf("coding plan leaf ID must match coding_plan_leaf_ plus 32 lowercase hex characters")
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
	if plan.Leaves == nil || len(plan.Leaves) > MaxCodingPlanLeaves {
		return fmt.Errorf("coding plan leaves must be an array of at most %d entries", MaxCodingPlanLeaves)
	}
	seen := make(map[CodingPlanLeafID]struct{}, len(plan.Leaves))
	statements := make(map[string]struct{}, len(plan.Leaves))
	approved := 0
	for index, leaf := range plan.Leaves {
		if err := leaf.Validate(); err != nil {
			return fmt.Errorf("coding plan leaf %d: %w", index, err)
		}
		if _, duplicate := seen[leaf.ID]; duplicate {
			return fmt.Errorf("coding plan contains duplicate leaf %q", leaf.ID)
		}
		seen[leaf.ID] = struct{}{}
		if _, duplicate := statements[leaf.Statement]; duplicate {
			return fmt.Errorf("coding plan repeats the statement of leaf %q", leaf.ID)
		}
		statements[leaf.Statement] = struct{}{}
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
