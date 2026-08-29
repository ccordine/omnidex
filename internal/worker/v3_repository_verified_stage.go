package worker

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/gryph/omnidex/internal/omni"
	"github.com/gryph/omnidex/internal/repository/changeapply"
)

type verifiedRepositoryChangeStage struct {
	contractID         string
	verificationPlanID string
	stage              *changeapply.StagedChange
}

const repositoryGoVerificationPlanSchemaV1 = "omnidex.repository-go-verification-plan.v1"

func newVerifiedRepositoryChangeStage(
	contractID string,
	commands []testCommand,
	stage *changeapply.StagedChange,
) (*verifiedRepositoryChangeStage, error) {
	contractID = strings.TrimSpace(contractID)
	if contractID == "" || stage == nil || stage.ID() == "" {
		return nil, fmt.Errorf("verified repository change stage requires exact contract and stage authority")
	}
	planID, err := repositoryVerificationPlanID(commands)
	if err != nil {
		return nil, err
	}
	return &verifiedRepositoryChangeStage{
		contractID: contractID, verificationPlanID: planID, stage: stage,
	}, nil
}

func (prepared *verifiedRepositoryChangeStage) RequireAuthority(
	contractID string,
	commands []testCommand,
) error {
	if prepared == nil || prepared.stage == nil || prepared.stage.ID() == "" {
		return fmt.Errorf("verified repository change stage is absent")
	}
	if strings.TrimSpace(contractID) == "" || contractID != prepared.contractID {
		return fmt.Errorf("verified repository change stage belongs to a different contract")
	}
	planID, err := repositoryVerificationPlanID(commands)
	if err != nil {
		return err
	}
	if planID != prepared.verificationPlanID {
		return fmt.Errorf("verified repository change stage belongs to a different verification plan")
	}
	return nil
}

func (prepared *verifiedRepositoryChangeStage) ID() string {
	return prepared.stage.ID()
}

func (prepared *verifiedRepositoryChangeStage) Patch() string {
	return prepared.stage.Patch()
}

func (prepared *verifiedRepositoryChangeStage) PatchSHA256() string {
	return prepared.stage.PatchSHA256()
}

func (prepared *verifiedRepositoryChangeStage) ChangedFileIDs() []string {
	return prepared.stage.ChangedFileIDs()
}

func (prepared *verifiedRepositoryChangeStage) ExpectedFiles() []changeapply.ExpectedFileState {
	return prepared.stage.ExpectedFiles()
}

func (prepared *verifiedRepositoryChangeStage) ApplyVerified(
	ctx context.Context,
) (omni.PatchApplyResult, error) {
	return prepared.stage.ApplyVerified(ctx)
}

func (prepared *verifiedRepositoryChangeStage) Cleanup() error {
	if prepared == nil || prepared.stage == nil {
		return nil
	}
	return prepared.stage.Cleanup()
}

func (prepared *verifiedRepositoryChangeStage) verificationAuthority(
	sourceSnapshotID string,
	contractID string,
	commands []testCommand,
) (repositoryVerificationAuthority, error) {
	if err := prepared.RequireAuthority(contractID, commands); err != nil {
		return repositoryVerificationAuthority{}, err
	}
	return newRepositoryVerificationAuthority(
		sourceSnapshotID, contractID, commands, prepared.stage,
	)
}

func repositoryVerificationPlanID(commands []testCommand) (string, error) {
	if err := validateRepositoryGoVerificationPlan(repositoryVerificationStaged, commands); err != nil {
		return "", fmt.Errorf("bind repository verification plan: %w", err)
	}
	raw, err := json.Marshal(commands)
	if err != nil {
		return "", fmt.Errorf("encode repository verification plan authority: %w", err)
	}
	digest := sha256.Sum256(append(
		[]byte(repositoryGoVerificationPlanSchemaV1+"\x00"), raw...,
	))
	return hex.EncodeToString(digest[:]), nil
}
