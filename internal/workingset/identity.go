package workingset

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"regexp"
	"strconv"
)

var (
	setIDPattern    = regexp.MustCompile(`^working_set_[0-9a-f]{64}$`)
	ledgerIDPattern = regexp.MustCompile(`^ledger_[0-9a-f]{64}$`)
)

func NewSetID(owner Owner) (SetID, error) {
	if err := validateOwner(owner); err != nil {
		return "", err
	}
	digest := workingSetIdentityDigest(
		WorkingSetSchemaV1,
		string(owner.LedgerID),
		strconv.FormatInt(owner.JobID, 10),
		strconv.FormatInt(owner.Generation, 10),
	)
	return SetID("working_set_" + digest), nil
}

func validateOwner(owner Owner) error {
	if !ledgerIDPattern.MatchString(string(owner.LedgerID)) {
		return fmt.Errorf("%w: owner ledger ID must match the canonical task-ledger identity", ErrInvalidSet)
	}
	if owner.JobID <= 0 {
		return fmt.Errorf("%w: owner job ID must be positive", ErrInvalidSet)
	}
	if owner.Generation <= 0 {
		return fmt.Errorf("%w: owner generation must be a positive PostgreSQL BIGINT", ErrInvalidSet)
	}
	return nil
}

func validateSetIdentity(id SetID, owner Owner) error {
	if !setIDPattern.MatchString(string(id)) {
		return fmt.Errorf("%w: ID must match working_set_ plus 64 lowercase hex characters", ErrInvalidSet)
	}
	expected, err := NewSetID(owner)
	if err != nil {
		return err
	}
	if id != expected {
		return fmt.Errorf("%w: ID does not match its ledger, job, and generation owner", ErrInvalidSet)
	}
	return nil
}

func rootScope(owner Owner) Scope {
	return Scope{Kind: ScopeJob, ID: ScopeID("job-" + strconv.FormatInt(owner.JobID, 10))}
}

func workingSetIdentityDigest(parts ...string) string {
	var canonical bytes.Buffer
	for _, part := range parts {
		_ = binary.Write(&canonical, binary.BigEndian, uint64(len(part)))
		_, _ = canonical.WriteString(part)
	}
	sum := sha256.Sum256(canonical.Bytes())
	return hex.EncodeToString(sum[:])
}
