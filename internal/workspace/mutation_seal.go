package workspace

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"
	"strings"
)

func sealMutationDelta(source Snapshot, mutations []plannedMutation) (string, string, error) {
	entries := make(map[string]Entry, len(source.Entries)+len(mutations))
	for _, entry := range source.Entries {
		entries[entry.Path] = entry
	}
	exclusions := make(map[string]Exclusion, len(source.Exclusions)+len(mutations))
	for _, exclusion := range source.Exclusions {
		exclusions[exclusion.Path] = exclusion
	}
	var patch strings.Builder
	for _, mutation := range mutations {
		transition := mutation.transition
		part, err := BuildFullFileUnifiedPatch(
			transition.Path, transition.Source.Present, mutation.original,
			transition.Expected.Present, mutation.expected,
		)
		if err != nil {
			return "", "", err
		}
		patch.WriteString(part)
		if transition.Expected.Present {
			entry := entries[transition.Path]
			entry.Path = transition.Path
			entry.Kind = EntryRegular
			entry.SHA256 = transition.Expected.SHA256
			entry.Size = transition.Expected.Size
			entry.Mode = transition.Expected.Mode
			entry.LinkTarget = ""
			entries[transition.Path] = entry
			delete(exclusions, transition.Path)
		} else {
			delete(entries, transition.Path)
			delete(exclusions, transition.Path)
			if mutation.gitTracked {
				exclusions[transition.Path] = Exclusion{
					Path: transition.Path, Reason: ExclusionAbsent,
				}
			}
		}
	}
	if patch.Len() == 0 || patch.Len() > MaxMutationPatchBytes {
		return "", "", fmt.Errorf("workspace mutation patch must contain 1-%d bytes", MaxMutationPatchBytes)
	}
	expectedEntries := make([]Entry, 0, len(entries))
	for _, entry := range entries {
		expectedEntries = append(expectedEntries, entry)
	}
	sort.Slice(expectedEntries, func(left, right int) bool {
		return expectedEntries[left].Path < expectedEntries[right].Path
	})
	expectedExclusions := make([]Exclusion, 0, len(exclusions))
	for _, exclusion := range exclusions {
		expectedExclusions = append(expectedExclusions, exclusion)
	}
	sort.Slice(expectedExclusions, func(left, right int) bool {
		return expectedExclusions[left].Path < expectedExclusions[right].Path
	})
	expectedID, err := stateIDForEntries(
		source.Schema, source.WorkspaceID, expectedEntries, expectedExclusions,
	)
	if err != nil {
		return "", "", err
	}
	return patch.String(), expectedID, nil
}

func (plan MutationPlan) Validate() error {
	if plan.Schema != MutationPlanSchemaV1 || !validOpaqueID(plan.ID, "workspace_stage_") ||
		!validMutationOwnerID(plan.OwnerID) {
		return fmt.Errorf("workspace mutation plan identity is invalid")
	}
	root, err := exactRootPath(plan.WorkspaceRoot)
	if err != nil || root != plan.WorkspaceRoot || plan.WorkspaceID != workspaceID(root) ||
		!validOpaqueID(plan.SourceStateID, "workspace_state_") ||
		!validOpaqueID(plan.ExpectedStateID, "workspace_state_") ||
		plan.ExpectedStateID == plan.SourceStateID {
		return fmt.Errorf("workspace mutation plan state authority is invalid")
	}
	gitFields := 0
	for _, value := range []string{plan.GitSourceSnapshotID, plan.GitRepositoryID, plan.GitHeadCommit} {
		if value != "" {
			gitFields++
		}
	}
	if gitFields != 0 && gitFields != 3 ||
		gitFields == 3 && (!validOpaqueID(plan.GitSourceSnapshotID, "snapshot_") ||
			!validOpaqueID(plan.GitRepositoryID, "repository_") ||
			!validHexBytes(plan.GitHeadCommit, 20, 32)) {
		return fmt.Errorf("workspace mutation Git source authority is invalid")
	}
	if len(plan.Patch) == 0 || len(plan.Patch) > MaxMutationPatchBytes ||
		digestMutationBytes([]byte(plan.Patch)) != plan.PatchSHA256 {
		return fmt.Errorf("workspace mutation plan patch authority is invalid")
	}
	if len(plan.Files) == 0 || len(plan.Files) > MaxMutationFiles {
		return fmt.Errorf("workspace mutation plan requires 1-%d file transitions", MaxMutationFiles)
	}
	previous := ""
	for _, file := range plan.Files {
		if err := validateMutablePath(file.Path); err != nil {
			return err
		}
		if file.FileID != opaqueID("workspace_file_", plan.WorkspaceID, file.Path) {
			return fmt.Errorf("workspace mutation file %q has invalid stable identity", file.Path)
		}
		if previous != "" && file.Path <= previous {
			return fmt.Errorf("workspace mutation files must be uniquely sorted")
		}
		previous = file.Path
		if err := file.Source.validate("source", file.Path); err != nil {
			return err
		}
		if err := file.Expected.validate("expected", file.Path); err != nil {
			return err
		}
		if file.Source == file.Expected {
			return fmt.Errorf("workspace mutation transition %q has no state delta", file.Path)
		}
	}
	expectedPlanID := opaqueID(
		"workspace_stage_", plan.OwnerID, plan.WorkspaceID,
		plan.SourceStateID, plan.ExpectedStateID, plan.PatchSHA256,
	)
	if plan.ID != expectedPlanID {
		return fmt.Errorf("workspace mutation plan ID differs from its exact authority")
	}
	return nil
}

func (plan MutationPlan) ValidateSource(source Snapshot) error {
	if err := plan.Validate(); err != nil {
		return err
	}
	if err := source.Validate(); err != nil {
		return fmt.Errorf("workspace mutation plan source: %w", err)
	}
	if plan.WorkspaceID != source.WorkspaceID || plan.WorkspaceRoot != source.Root ||
		plan.SourceStateID != source.ID {
		return fmt.Errorf("workspace mutation plan differs from its exact source state")
	}
	if source.Git == nil {
		if plan.GitSourceSnapshotID != "" || plan.GitRepositoryID != "" || plan.GitHeadCommit != "" {
			return fmt.Errorf("plain workspace mutation contains Git-only authority")
		}
	} else if plan.GitSourceSnapshotID != source.Git.RepositorySnapshotID ||
		plan.GitRepositoryID != source.Git.RepositoryID || plan.GitHeadCommit != source.Git.HeadCommit {
		return fmt.Errorf("workspace mutation Git source authority is invalid")
	}
	return nil
}

func (plan MutationPlan) VerifyExpected(snapshot Snapshot) error {
	if err := snapshot.Validate(); err != nil {
		return fmt.Errorf("verify workspace mutation post-state: %w", err)
	}
	if snapshot.WorkspaceID != plan.WorkspaceID || snapshot.Root != plan.WorkspaceRoot ||
		snapshot.ID != plan.ExpectedStateID {
		return fmt.Errorf(
			"workspace mutation post-state differs from expected state %s; observed %s",
			plan.ExpectedStateID, snapshot.ID,
		)
	}
	if plan.GitSourceSnapshotID == "" {
		if snapshot.Git != nil {
			return fmt.Errorf("plain workspace mutation unexpectedly gained Git authority")
		}
	} else if snapshot.Git == nil || snapshot.Git.RepositoryID != plan.GitRepositoryID ||
		snapshot.Git.HeadCommit != plan.GitHeadCommit {
		return fmt.Errorf("workspace mutation post-state has a different Git repository or HEAD")
	}
	entries := make(map[string]Entry, len(snapshot.Entries))
	for _, entry := range snapshot.Entries {
		entries[entry.Path] = entry
	}
	for _, transition := range plan.Files {
		entry, present := entries[transition.Path]
		if present != transition.Expected.Present {
			return fmt.Errorf("workspace mutation post-state has wrong presence for %q", transition.Path)
		}
		if present && (entry.Kind != EntryRegular || entry.SHA256 != transition.Expected.SHA256 ||
			entry.Size != transition.Expected.Size || entry.Mode != transition.Expected.Mode) {
			return fmt.Errorf("workspace mutation post-state differs for %q", transition.Path)
		}
	}
	return nil
}

func stateIDForEntries(
	schema, workspaceID string,
	entries []Entry,
	exclusions []Exclusion,
) (string, error) {
	identities := make([]stateEntryIdentity, len(entries))
	for index, entry := range entries {
		identities[index] = stateEntryIdentity{
			Path: entry.Path, Kind: entry.Kind, SHA256: entry.SHA256,
			Size: entry.Size, Mode: entry.Mode, LinkTarget: entry.LinkTarget,
		}
	}
	raw, err := canonicalStateIdentityJSON(snapshotIdentity{
		Schema: schema, WorkspaceID: workspaceID, Entries: identities,
		Exclusions: append([]Exclusion(nil), exclusions...),
	})
	if err != nil {
		return "", err
	}
	digest := sha256.Sum256(raw)
	return "workspace_state_" + hex.EncodeToString(digest[:]), nil
}
