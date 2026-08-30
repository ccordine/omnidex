package worker

import (
	"encoding/json"
	"fmt"

	"github.com/gryph/omnidex/internal/queue"
	workspacefacts "github.com/gryph/omnidex/internal/workspace"
)

const directCodingExactStateAuthoritySchemaV1 = "omnidex.direct-coding-exact-state-authority.v1"
const directCodingExactStateReceiptSchemaV1 = "omnidex.direct-coding-exact-state-verification.v1"

// directCodingPreparedExactState is the read-only counterpart to a staged
// mutation. Its snapshot was mechanically proved to equal the desired states
// by the strict mutation planner before this authority can be constructed.
type directCodingPreparedExactState struct {
	source    workspacefacts.Snapshot
	authority directCodingExactStateAuthority
}

type directCodingExactStateAuthority struct {
	Schema           string
	ID               string
	WorkspaceID      string
	WorkspaceStateID string
	OwnerID          string
	Verification     queue.WorkspaceMutationVerificationPlan
}

type directCodingExactStateIdentity struct {
	Schema           string                                  `json:"schema"`
	WorkspaceID      string                                  `json:"workspace_id"`
	WorkspaceStateID string                                  `json:"workspace_state_id"`
	OwnerID          string                                  `json:"owner_id"`
	Verification     queue.WorkspaceMutationVerificationPlan `json:"verification"`
}

func newDirectCodingExactStateAuthority(
	source workspacefacts.Snapshot,
	ownerID string,
	commands []testCommand,
) (directCodingExactStateAuthority, error) {
	verification, err := workspaceVerificationPlanForCommands(commands)
	if err != nil {
		return directCodingExactStateAuthority{}, err
	}
	authority := directCodingExactStateAuthority{
		Schema:      directCodingExactStateAuthoritySchemaV1,
		WorkspaceID: source.WorkspaceID, WorkspaceStateID: source.ID,
		OwnerID: ownerID, Verification: verification,
	}
	id, err := directCodingExactStateAuthorityID(authority)
	if err != nil {
		return directCodingExactStateAuthority{}, err
	}
	authority.ID = id
	if err := authority.validate(source, commands); err != nil {
		return directCodingExactStateAuthority{}, err
	}
	return authority, nil
}

func (authority directCodingExactStateAuthority) validate(
	source workspacefacts.Snapshot,
	commands []testCommand,
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
	if err := validateDirectCodingExactStateJournal(authority.Verification, commands); err != nil {
		return err
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
		OwnerID: authority.OwnerID, Verification: authority.Verification,
	})
	if err != nil {
		return "", fmt.Errorf("encode exact direct-coding workspace authority: %w", err)
	}
	return "workspace_exact_" + directCodingDigest(string(raw)), nil
}

func workspaceVerificationPlanForCommands(
	commands []testCommand,
) (queue.WorkspaceMutationVerificationPlan, error) {
	intents := make([]queue.WorkspaceMutationVerificationIntent, len(commands))
	for index, command := range commands {
		encoded, err := encodeWorkspaceVerificationCommand(command)
		if err != nil {
			return queue.WorkspaceMutationVerificationPlan{}, fmt.Errorf(
				"encode workspace verification command %d: %w", index+1, err,
			)
		}
		intents[index] = queue.WorkspaceMutationVerificationIntent{
			Kind: workspaceVerificationEvidenceKind(command.Purpose), Command: encoded,
		}
	}
	return queue.NewWorkspaceMutationVerificationPlan(intents)
}

func validateDirectCodingExactStateJournal(
	plan queue.WorkspaceMutationVerificationPlan,
	commands []testCommand,
) error {
	recovered, err := workspaceVerificationCommandsFromPlan(plan)
	if err != nil {
		return err
	}
	if len(commands) != len(recovered) {
		return fmt.Errorf("exact direct-coding commands differ from journal authority")
	}
	for index, command := range commands {
		encoded, err := encodeWorkspaceVerificationCommand(command)
		if err != nil || encoded != plan.Commands[index].Command {
			return fmt.Errorf("exact direct-coding command %d differs from journal authority", index+1)
		}
	}
	return nil
}

func directCodingExactStateReceiptSHA256(
	authority directCodingExactStateAuthority,
	commandEvidenceIDs []int64,
	passed bool,
) (string, error) {
	if !validRepositoryVerificationOpaqueID(authority.ID, "workspace_exact_") ||
		len(commandEvidenceIDs) != len(authority.Verification.Commands) {
		return "", fmt.Errorf("exact direct-coding verification receipt authority is incomplete")
	}
	for index, evidenceID := range commandEvidenceIDs {
		if evidenceID <= 0 || index > 0 && evidenceID <= commandEvidenceIDs[index-1] {
			return "", fmt.Errorf("exact direct-coding verification evidence identities are not ordered")
		}
	}
	raw, err := json.Marshal(struct {
		Schema       string `json:"schema"`
		AuthorityID  string `json:"authority_id"`
		CommandCount int    `json:"command_count"`
		Passed       bool   `json:"passed"`
	}{
		Schema: directCodingExactStateReceiptSchemaV1, AuthorityID: authority.ID,
		CommandCount: len(commandEvidenceIDs), Passed: passed,
	})
	if err != nil {
		return "", fmt.Errorf("encode exact direct-coding verification receipt: %w", err)
	}
	return directCodingDigest(string(raw)), nil
}
