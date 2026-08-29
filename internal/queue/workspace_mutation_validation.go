package queue

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/gryph/omnidex/internal/evidence"
	"github.com/gryph/omnidex/internal/model"
)

func NewWorkspaceMutationVerificationPlan(
	intents []WorkspaceMutationVerificationIntent,
) (WorkspaceMutationVerificationPlan, error) {
	plan := WorkspaceMutationVerificationPlan{
		Schema:   WorkspaceMutationVerificationPlanV1,
		Commands: make([]WorkspaceMutationVerificationCommand, len(intents)),
	}
	for index, intent := range intents {
		if intent.Command == "" || intent.Command != strings.TrimSpace(intent.Command) ||
			!utf8.ValidString(intent.Command) || strings.ContainsRune(intent.Command, '\x00') ||
			len(intent.Command) > 16*1024 {
			return WorkspaceMutationVerificationPlan{}, fmt.Errorf("workspace verification command %d is invalid", index+1)
		}
		digest := sha256.Sum256([]byte(intent.Command))
		plan.Commands[index] = WorkspaceMutationVerificationCommand{
			Ordinal: index + 1, Kind: intent.Kind, Command: intent.Command,
			CommandSHA256: hex.EncodeToString(digest[:]),
		}
	}
	if err := plan.Validate(); err != nil {
		return WorkspaceMutationVerificationPlan{}, err
	}
	return plan, nil
}

func (plan WorkspaceMutationVerificationPlan) Validate() error {
	if plan.Schema != WorkspaceMutationVerificationPlanV1 || len(plan.Commands) == 0 ||
		len(plan.Commands) > 32 {
		return fmt.Errorf("workspace mutation verification plan requires 1-32 commands")
	}
	for index, command := range plan.Commands {
		if command.Ordinal != index+1 ||
			(command.Kind != evidence.KindCommandOutput && command.Kind != evidence.KindTestResult) ||
			command.Command == "" || command.Command != strings.TrimSpace(command.Command) ||
			!utf8.ValidString(command.Command) || strings.ContainsRune(command.Command, '\x00') ||
			len(command.Command) > 16*1024 ||
			digestWorkspaceMutationText(command.Command) != command.CommandSHA256 ||
			!validSHA256Digest(command.CommandSHA256) {
			return fmt.Errorf("workspace mutation verification command %d is invalid", index+1)
		}
	}
	return nil
}

func validateWorkspaceMutationCommand(command WorkspaceMutationCommand) error {
	if err := validateStepAttemptAuthority(command.creatorAuthority()); err != nil {
		return fmt.Errorf("workspace mutation creator authority: %w", err)
	}
	if command.ProjectID <= 0 {
		return fmt.Errorf("workspace mutation requires one positive project identity")
	}
	if err := model.ValidateChannelWorkspaceRoot(command.ProjectLocation); err != nil ||
		command.ProjectLocation == "/" ||
		strings.ContainsAny(command.ProjectLocation, "\\\r\n") {
		return fmt.Errorf("workspace mutation requires one canonical non-root project location")
	}
	if err := command.Plan.Validate(); err != nil {
		return fmt.Errorf("workspace mutation plan: %w", err)
	}
	if err := command.Verification.Validate(); err != nil {
		return err
	}
	return nil
}

func validateWorkspaceMutationExecutionAuthority(
	authority model.StepAttemptAuthority,
	command WorkspaceMutationCommand,
) error {
	if err := validateStepAttemptAuthority(authority); err != nil {
		return err
	}
	if authority.JobID != command.JobID || authority.Generation != command.Generation ||
		authority.StepID != command.StepID {
		return staleStepAttemptError(authority, "workspace mutation owner disagrees with command", nil)
	}
	return nil
}

func validateWorkspaceMutationCallbacks(callbacks WorkspaceMutationCallbacks) error {
	if callbacks.Observe == nil || callbacks.Apply == nil || callbacks.Verify == nil {
		return fmt.Errorf("workspace mutation requires observe, apply, and verify callbacks")
	}
	return nil
}

func validateWorkspaceMutationVerificationResult(
	command WorkspaceMutationCommand,
	operationID string,
	result WorkspaceMutationVerificationResult,
) error {
	failure := strings.TrimSpace(result.Failure)
	if result.Succeeded && failure != "" || !result.Succeeded && failure == "" {
		return fmt.Errorf("workspace mutation verification result success and failure authority disagree")
	}
	if len(result.Failure) > 64*1024 || !utf8.ValidString(result.Failure) ||
		strings.ContainsRune(result.Failure, '\x00') {
		return fmt.Errorf("workspace mutation verification failure is not bounded exact text")
	}
	if len(result.CommandEvidence) != len(command.Verification.Commands) {
		return fmt.Errorf("workspace mutation verification evidence count differs from its plan")
	}
	failedCommand := false
	for index, record := range result.CommandEvidence {
		planned := command.Verification.Commands[index]
		if record.ID != 0 || record.JobID != command.JobID || record.StepID != command.StepID ||
			record.Kind != planned.Kind || record.Command != planned.Command ||
			digestWorkspaceMutationText(record.Command) != planned.CommandSHA256 {
			return fmt.Errorf("workspace mutation verification evidence %d differs from its plan", index+1)
		}
		succeeded, ok := record.Metadata["succeeded"].(bool)
		if !ok || result.Succeeded && !succeeded {
			return fmt.Errorf("workspace mutation verification evidence %d has invalid outcome", index+1)
		}
		if !succeeded {
			failedCommand = true
		}
		if record.SourceType != "" && record.SourceType != "workspace_verification" ||
			record.SourceRef != "" && record.SourceRef != operationID {
			return fmt.Errorf("workspace mutation verification evidence %d has foreign source authority", index+1)
		}
	}
	if !result.Succeeded && !failedCommand {
		return fmt.Errorf("failed workspace mutation verification has no failed command evidence")
	}
	if command.Plan.GitSourceSnapshotID == "" && result.VerifiedRepositorySnapshotID != "" ||
		command.Plan.GitSourceSnapshotID != "" && !validSHA256ID(
			result.VerifiedRepositorySnapshotID, "snapshot_",
		) {
		return fmt.Errorf("workspace mutation verification result has invalid optional Git binding")
	}
	return nil
}

func digestWorkspaceMutationText(value string) string {
	digest := sha256.Sum256([]byte(value))
	return hex.EncodeToString(digest[:])
}
