package worker

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/gryph/omnidex/internal/repository/changeapply"
)

const repositoryExpectedPostSchemaV1 = "omnidex.repository-expected-post.v1"

type repositoryVerificationEvidenceAuthority interface {
	validate([]testCommand) error
	metadata() map[string]any
	planIdentity() string
	allowsScope(repositoryVerificationScope) bool
}

type repositoryVerificationAuthority struct {
	contractID       string
	sourceSnapshotID string
	stageID          string
	patchSHA256      string
	planID           string
	expectedPostID   string
}

type repositoryExpectedPostIdentity struct {
	Schema           string                          `json:"schema"`
	SourceSnapshotID string                          `json:"source_snapshot_id"`
	Files            []changeapply.ExpectedFileState `json:"files"`
}

func newRepositoryVerificationAuthority(
	sourceSnapshotID string,
	contractID string,
	commands []testCommand,
	stage *changeapply.StagedChange,
) (repositoryVerificationAuthority, error) {
	if stage == nil || strings.TrimSpace(sourceSnapshotID) == "" || strings.TrimSpace(contractID) == "" {
		return repositoryVerificationAuthority{}, fmt.Errorf("repository verification authority is incomplete")
	}
	patch := []byte(stage.Patch())
	patchDigest := sha256.Sum256(patch)
	patchSHA256 := hex.EncodeToString(patchDigest[:])
	if len(patch) == 0 || len(patch) > maxRepositoryPatchEvidenceBytes ||
		stage.ID() == "" || stage.PatchSHA256() != patchSHA256 {
		return repositoryVerificationAuthority{}, fmt.Errorf("repository verification stage has invalid patch authority")
	}
	planID, err := repositoryVerificationPlanID(commands)
	if err != nil {
		return repositoryVerificationAuthority{}, err
	}
	expectedPostID, err := repositoryExpectedPostID(
		sourceSnapshotID, stage.ChangedFileIDs(), stage.ExpectedFiles(),
	)
	if err != nil {
		return repositoryVerificationAuthority{}, err
	}
	authority := repositoryVerificationAuthority{
		contractID: contractID, sourceSnapshotID: sourceSnapshotID,
		stageID: stage.ID(), patchSHA256: patchSHA256,
		planID: planID, expectedPostID: expectedPostID,
	}
	if err := authority.validate(commands); err != nil {
		return repositoryVerificationAuthority{}, err
	}
	return authority, nil
}

func (authority repositoryVerificationAuthority) validate(commands []testCommand) error {
	if !validRepositoryVerificationOpaqueID(authority.contractID, "change_contract_") ||
		!validRepositoryVerificationOpaqueID(authority.sourceSnapshotID, "snapshot_") ||
		!validRepositoryVerificationOpaqueID(authority.stageID, "repository_change_stage_") ||
		!validRepositoryVerificationSHA256(authority.patchSHA256) ||
		!validRepositoryVerificationSHA256(authority.planID) ||
		!validRepositoryVerificationSHA256(authority.expectedPostID) {
		return fmt.Errorf("repository verification authority contains a malformed identity")
	}
	planID, err := repositoryVerificationPlanID(commands)
	if err != nil {
		return err
	}
	if planID != authority.planID {
		return fmt.Errorf("repository verification authority belongs to a different ordered plan")
	}
	return nil
}

func (authority repositoryVerificationAuthority) metadata() map[string]any {
	return map[string]any{
		"repository_change_contract_id":   authority.contractID,
		"repository_source_snapshot_id":   authority.sourceSnapshotID,
		"repository_change_stage_id":      authority.stageID,
		"repository_change_patch_sha256":  authority.patchSHA256,
		"repository_verification_plan_id": authority.planID,
		"repository_expected_post_id":     authority.expectedPostID,
	}
}

func (authority repositoryVerificationAuthority) planIdentity() string {
	return authority.planID
}

func (authority repositoryVerificationAuthority) allowsScope(scope repositoryVerificationScope) bool {
	return scope == repositoryVerificationStaged
}

func repositoryExpectedPostID(
	sourceSnapshotID string,
	changedFileIDs []string,
	expected []changeapply.ExpectedFileState,
) (string, error) {
	if !validRepositoryVerificationOpaqueID(sourceSnapshotID, "snapshot_") ||
		len(changedFileIDs) == 0 || len(changedFileIDs) != len(expected) {
		return "", fmt.Errorf("repository verification expected post authority is incomplete")
	}
	files := append([]changeapply.ExpectedFileState(nil), expected...)
	sort.Slice(files, func(left, right int) bool { return files[left].FileID < files[right].FileID })
	changed := append([]string(nil), changedFileIDs...)
	sort.Strings(changed)
	for index, file := range files {
		if file.FileID == "" || file.FileID != changed[index] ||
			!validRepositoryVerificationSHA256(file.SHA256) || file.Size < 0 ||
			index > 0 && file.FileID == files[index-1].FileID {
			return "", fmt.Errorf("repository verification expected post authority is invalid")
		}
	}
	raw, err := json.Marshal(repositoryExpectedPostIdentity{
		Schema: repositoryExpectedPostSchemaV1, SourceSnapshotID: sourceSnapshotID, Files: files,
	})
	if err != nil {
		return "", fmt.Errorf("encode repository expected post authority: %w", err)
	}
	digest := sha256.Sum256(append([]byte(repositoryExpectedPostSchemaV1+"\x00"), raw...))
	return hex.EncodeToString(digest[:]), nil
}

func validRepositoryVerificationOpaqueID(value, prefix string) bool {
	return strings.HasPrefix(value, prefix) &&
		validRepositoryVerificationSHA256(strings.TrimPrefix(value, prefix))
}

func validRepositoryVerificationSHA256(value string) bool {
	if len(value) != sha256.Size*2 || value != strings.ToLower(value) {
		return false
	}
	decoded, err := hex.DecodeString(value)
	return err == nil && len(decoded) == sha256.Size
}
