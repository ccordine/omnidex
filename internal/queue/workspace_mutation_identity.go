package queue

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
)

type workspaceMutationIdentityEnvelope struct {
	Schema  string                   `json:"schema"`
	Command WorkspaceMutationCommand `json:"command"`
}

func workspaceMutationOperation(
	command WorkspaceMutationCommand,
) (workspaceMutationOperationIdentity, error) {
	if err := validateWorkspaceMutationCommand(command); err != nil {
		return workspaceMutationOperationIdentity{}, err
	}
	planRaw, err := json.Marshal(command.Verification)
	if err != nil {
		return workspaceMutationOperationIdentity{}, fmt.Errorf("encode workspace verification plan: %w", err)
	}
	planSHA := sha256.Sum256(planRaw)
	encoded, err := json.Marshal(workspaceMutationIdentityEnvelope{
		Schema: workspaceMutationCommandSchema, Command: command,
	})
	if err != nil {
		return workspaceMutationOperationIdentity{}, fmt.Errorf("encode workspace mutation command identity: %w", err)
	}
	digest := sha256.Sum256(encoded)
	commandSHA := hex.EncodeToString(digest[:])
	return workspaceMutationOperationIdentity{
		ID: "workspace_mutation_" + commandSHA, CommandSHA256: commandSHA,
		PlanJSON: string(planRaw), PlanSHA256: hex.EncodeToString(planSHA[:]),
	}, nil
}
