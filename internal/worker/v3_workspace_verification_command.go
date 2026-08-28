package worker

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/gryph/omnidex/internal/evidence"
	"github.com/gryph/omnidex/internal/queue"
)

const workspaceVerificationCommandSchemaV2 = "omnidex.workspace-verification-command.v2"

type workspaceVerificationCommandRole string

const (
	workspaceVerificationPrimary workspaceVerificationCommandRole = "primary"
	workspaceVerificationCleanup workspaceVerificationCommandRole = "cleanup"
)

type workspaceVerificationCommandAuthority struct {
	Schema             string                                  `json:"schema"`
	Family             string                                  `json:"family"`
	Program            string                                  `json:"program"`
	Arguments          []string                                `json:"arguments"`
	Purpose            string                                  `json:"purpose"`
	Role               string                                  `json:"role"`
	TimeoutNanoseconds int64                                   `json:"timeout_nanoseconds"`
	RepositoryProof    *repositoryGoTestProof                  `json:"repository_proof,omitempty"`
	Environment        *directCodingDockerEnvironmentAuthority `json:"environment,omitempty"`
}

func encodeWorkspaceVerificationCommand(command testCommand) (string, error) {
	if command.Family == plainTextWorkspaceVerificationFamily {
		if err := validatePlainTextWorkspaceVerificationCommand(command); err != nil {
			return "", err
		}
	} else {
		if command.Environment != nil {
			return "", fmt.Errorf("non-plain workspace verification command cannot carry project environment authority")
		}
		if err := validateV3Command(command.Name, command.Args); err != nil {
			return "", err
		}
	}
	if !validWorkspaceVerificationPurpose(command.Purpose) ||
		command.Family != strings.TrimSpace(command.Family) ||
		strings.ContainsAny(command.Family, "\x00\r\n") || len(command.Family) > 128 ||
		command.Timeout < 0 || command.Timeout > maxV3CommandLimit {
		return "", fmt.Errorf("workspace verification command has invalid timeout")
	}
	role := command.WorkspaceRole
	if role == "" {
		role = workspaceVerificationPrimary
	}
	if !validWorkspaceVerificationRole(role) {
		return "", fmt.Errorf("workspace verification command has invalid role %q", role)
	}
	authority := workspaceVerificationCommandAuthority{
		Schema: workspaceVerificationCommandSchemaV2,
		Family: command.Family, Program: command.Name,
		Arguments: append([]string(nil), command.Args...),
		Purpose:   string(command.Purpose), Role: string(role),
		TimeoutNanoseconds: int64(command.Timeout),
		RepositoryProof:    cloneRepositoryGoTestProof(command.RepositoryProof),
		Environment:        cloneDirectCodingDockerEnvironmentAuthority(command.Environment),
	}
	raw, err := json.Marshal(authority)
	if err != nil {
		return "", fmt.Errorf("encode workspace verification command: %w", err)
	}
	return string(raw), nil
}

func decodeWorkspaceVerificationCommand(raw string) (testCommand, error) {
	decoder := json.NewDecoder(bytes.NewBufferString(raw))
	decoder.DisallowUnknownFields()
	var authority workspaceVerificationCommandAuthority
	if err := decoder.Decode(&authority); err != nil {
		return testCommand{}, fmt.Errorf("decode workspace verification command: %w", err)
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		return testCommand{}, fmt.Errorf("workspace verification command contains trailing data")
	}
	if authority.Schema != workspaceVerificationCommandSchemaV2 ||
		authority.TimeoutNanoseconds < 0 ||
		authority.TimeoutNanoseconds > int64(maxV3CommandLimit) {
		return testCommand{}, fmt.Errorf("workspace verification command authority is invalid")
	}
	command := testCommand{
		Family: authority.Family, Name: authority.Program,
		Args:            append([]string(nil), authority.Arguments...),
		Purpose:         verificationCommandPurpose(authority.Purpose),
		Timeout:         time.Duration(authority.TimeoutNanoseconds),
		RepositoryProof: cloneRepositoryGoTestProof(authority.RepositoryProof),
		WorkspaceRole:   workspaceVerificationCommandRole(authority.Role),
		Environment:     cloneDirectCodingDockerEnvironmentAuthority(authority.Environment),
	}
	canonical, err := encodeWorkspaceVerificationCommand(command)
	if err != nil {
		return testCommand{}, err
	}
	if canonical != raw {
		return testCommand{}, fmt.Errorf("workspace verification command is not canonical")
	}
	return command, nil
}

func validWorkspaceVerificationPurpose(purpose verificationCommandPurpose) bool {
	switch purpose {
	case verificationSetup, verificationSyntax, verificationTest,
		verificationBuild, verificationConfig:
		return true
	default:
		return false
	}
}

func validWorkspaceVerificationRole(role workspaceVerificationCommandRole) bool {
	return role == workspaceVerificationPrimary || role == workspaceVerificationCleanup
}

func workspaceVerificationCommandsFromPlan(
	plan queue.WorkspaceMutationVerificationPlan,
) ([]testCommand, error) {
	if err := plan.Validate(); err != nil {
		return nil, err
	}
	commands := make([]testCommand, len(plan.Commands))
	cleanupStarted := false
	for index, planned := range plan.Commands {
		command, err := decodeWorkspaceVerificationCommand(planned.Command)
		if err != nil {
			return nil, fmt.Errorf("workspace verification command %d: %w", index+1, err)
		}
		if command.WorkspaceRole == workspaceVerificationCleanup {
			cleanupStarted = true
		} else if cleanupStarted {
			return nil, fmt.Errorf("workspace verification primary command %d follows cleanup authority", index+1)
		}
		expectedKind := workspaceVerificationEvidenceKind(command.Purpose)
		if planned.Kind != expectedKind {
			return nil, fmt.Errorf(
				"workspace verification command %d kind %q differs from purpose %q",
				index+1, planned.Kind, command.Purpose,
			)
		}
		commands[index] = command
	}
	if len(commands) == 0 || commands[0].WorkspaceRole != workspaceVerificationPrimary {
		return nil, fmt.Errorf("workspace verification plan requires at least one primary command")
	}
	return commands, nil
}

func workspaceVerificationEvidenceKind(purpose verificationCommandPurpose) string {
	if purpose == verificationTest {
		return evidence.KindTestResult
	}
	return evidence.KindCommandOutput
}

func cloneRepositoryGoTestProof(source *repositoryGoTestProof) *repositoryGoTestProof {
	if source == nil {
		return nil
	}
	clone := *source
	clone.Expected = append([]repositoryGoExpectedTest(nil), source.Expected...)
	for index := range clone.Expected {
		clone.Expected[index].TargetSymbolIDs = append(
			[]string(nil), source.Expected[index].TargetSymbolIDs...,
		)
	}
	return &clone
}
