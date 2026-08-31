package worker

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"

	workspacefacts "github.com/gryph/omnidex/internal/workspace"
)

func directCodingDigest(value string) string {
	digest := sha256.Sum256([]byte(value))
	return hex.EncodeToString(digest[:])
}

const directCodingExactStateAuthoritySchemaV1 = "omnidex.direct-coding-exact-state-authority.v1"
const directCodingExactStateReceiptSchemaV1 = "omnidex.direct-coding-exact-state-verification.v1"

type directCodingExactStateAuthority struct {
	Schema           string
	ID               string
	WorkspaceID      string
	WorkspaceStateID string
	OwnerID          string
}

type directCodingExactStateIdentity struct {
	Schema           string `json:"schema"`
	WorkspaceID      string `json:"workspace_id"`
	WorkspaceStateID string `json:"workspace_state_id"`
	OwnerID          string `json:"owner_id"`
}

func newDirectCodingExactStateAuthority(
	source workspacefacts.Snapshot,
	ownerID string,
) (directCodingExactStateAuthority, error) {
	authority := directCodingExactStateAuthority{
		Schema:      directCodingExactStateAuthoritySchemaV1,
		WorkspaceID: source.WorkspaceID, WorkspaceStateID: source.ID,
		OwnerID: ownerID,
	}
	id, err := directCodingExactStateAuthorityID(authority)
	if err != nil {
		return directCodingExactStateAuthority{}, err
	}
	authority.ID = id
	if err := authority.validate(source); err != nil {
		return directCodingExactStateAuthority{}, err
	}
	return authority, nil
}

func (authority directCodingExactStateAuthority) validate(
	source workspacefacts.Snapshot,
) error {
	if err := source.Validate(); err != nil {
		return fmt.Errorf("exact direct-coding workspace source: %w", err)
	}
	if authority.Schema != directCodingExactStateAuthoritySchemaV1 ||
		!validRepositoryVerificationOpaqueID(authority.ID, "workspace_exact_") ||
		!validRepositoryVerificationOpaqueID(authority.OwnerID, "coding_") ||
		authority.WorkspaceID != source.WorkspaceID || authority.WorkspaceStateID != source.ID {
		return fmt.Errorf("exact direct-coding workspace authority is invalid")
	}
	expected, err := directCodingExactStateAuthorityID(authority)
	if err != nil {
		return err
	}
	if authority.ID != expected {
		return fmt.Errorf("exact direct-coding workspace identity differs from its authority")
	}
	return nil
}

func directCodingExactStateAuthorityID(
	authority directCodingExactStateAuthority,
) (string, error) {
	raw, err := json.Marshal(directCodingExactStateIdentity{
		Schema:      authority.Schema,
		WorkspaceID: authority.WorkspaceID, WorkspaceStateID: authority.WorkspaceStateID,
		OwnerID: authority.OwnerID,
	})
	if err != nil {
		return "", fmt.Errorf("encode exact direct-coding workspace authority: %w", err)
	}
	return "workspace_exact_" + directCodingDigest(string(raw)), nil
}

func directCodingExactStateReceiptSHA256(
	authority directCodingExactStateAuthority,
	passed bool,
) (string, error) {
	if !validRepositoryVerificationOpaqueID(authority.ID, "workspace_exact_") {
		return "", fmt.Errorf("exact direct-coding verification receipt authority is incomplete")
	}
	raw, err := json.Marshal(struct {
		Schema      string `json:"schema"`
		AuthorityID string `json:"authority_id"`
		Passed      bool   `json:"passed"`
	}{
		Schema: directCodingExactStateReceiptSchemaV1, AuthorityID: authority.ID,
		Passed: passed,
	})
	if err != nil {
		return "", fmt.Errorf("encode exact direct-coding verification receipt: %w", err)
	}
	return directCodingDigest(string(raw)), nil
}
