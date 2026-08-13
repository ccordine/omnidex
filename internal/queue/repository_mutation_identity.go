package queue

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
)

const repositoryMutationIdentitySchema = "omnidex.repository-mutation-command.v2"

type repositoryMutationIdentityEnvelope struct {
	Schema  string                    `json:"schema"`
	Command RepositoryMutationCommand `json:"command"`
}

func repositoryMutationOperation(
	command RepositoryMutationCommand,
) (repositoryMutationOperationIdentity, error) {
	if err := validateRepositoryMutationCommand(command); err != nil {
		return repositoryMutationOperationIdentity{}, err
	}
	encoded, err := json.Marshal(repositoryMutationIdentityEnvelope{
		Schema: repositoryMutationIdentitySchema, Command: command,
	})
	if err != nil {
		return repositoryMutationOperationIdentity{}, fmt.Errorf(
			"encode repository mutation command identity: %w", err,
		)
	}
	digest := sha256.Sum256(encoded)
	commandSHA := hex.EncodeToString(digest[:])
	return repositoryMutationOperationIdentity{
		ID: "repository_mutation_" + commandSHA, CommandSHA256: commandSHA,
	}, nil
}
