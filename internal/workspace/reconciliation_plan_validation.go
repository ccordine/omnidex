package workspace

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"path/filepath"
	"slices"
	"sort"
	"strconv"
)

func (plan ReconciliationPlan) Validate() error {
	if plan.Schema != ReconciliationPlanSchemaV1 ||
		!validOpaqueID(plan.ID, "workspace_reconciliation_") ||
		!validReconciliationOwnerID(plan.OwnerID) {
		return fmt.Errorf("workspace reconciliation plan identity is invalid")
	}
	root, err := exactRootPath(plan.Root)
	if err != nil || root != plan.Root || plan.WorkspaceID != workspaceID(root) ||
		!validOpaqueID(plan.SourceStateID, "workspace_state_") ||
		!validOpaqueID(plan.ExpectedStateID, "workspace_state_") {
		return fmt.Errorf("workspace reconciliation plan state authority is invalid")
	}
	if plan.Files == nil || len(plan.Files) > MaxReconciliationFiles {
		return fmt.Errorf("workspace reconciliation plan files are absent or exceed %d", MaxReconciliationFiles)
	}
	if plan.Paths == nil || len(plan.Paths) > MaxReconciliationFiles {
		return fmt.Errorf("workspace reconciliation plan paths are absent or exceed %d", MaxReconciliationFiles)
	}
	selected := make(map[string]struct{}, len(plan.Paths))
	previous := ""
	for _, relative := range plan.Paths {
		if err := validateMutablePath(relative); err != nil {
			return err
		}
		if previous != "" && relative <= previous {
			return fmt.Errorf("workspace reconciliation paths must be uniquely sorted")
		}
		previous = relative
		selected[relative] = struct{}{}
	}
	if plan.Moves == nil || !slices.Equal(plan.Moves, deriveReconciliationMoves(plan.Files)) {
		return fmt.Errorf("workspace reconciliation move records differ from exact transitions")
	}
	totalBytes := 0
	if len(plan.Files) == 0 && plan.ExpectedStateID != plan.SourceStateID {
		return fmt.Errorf("zero-delta workspace reconciliation changes expected state identity")
	}
	if len(plan.Files) > 0 && plan.ExpectedStateID == plan.SourceStateID {
		return fmt.Errorf("changed workspace reconciliation retains source state identity")
	}
	previous = ""
	for _, file := range plan.Files {
		if err := validateMutablePath(file.Path); err != nil {
			return err
		}
		if previous != "" && file.Path <= previous {
			return fmt.Errorf("workspace reconciliation files must be uniquely sorted")
		}
		previous = file.Path
		if _, ok := selected[file.Path]; !ok {
			return fmt.Errorf("workspace reconciliation transition %q is outside selected path authority", file.Path)
		}
		if err := file.Source.validate("source", file.Path); err != nil {
			return err
		}
		if err := file.Expected.validate("expected", file.Path); err != nil {
			return err
		}
		if !file.Source.Present && !file.Expected.Present {
			return fmt.Errorf("workspace reconciliation transition %q is absent on both sides", file.Path)
		}
		if file.Source == file.Expected {
			return fmt.Errorf("workspace reconciliation transition %q has no state delta", file.Path)
		}
		if file.Expected.Present {
			if len(file.Content) > MaxReconciliationFileBytes ||
				file.Expected.Kind != EntryRegular || int64(len(file.Content)) != file.Expected.Size ||
				digestBytes(file.Content) != file.Expected.SHA256 {
				return fmt.Errorf("workspace reconciliation expected bytes for %q are invalid", file.Path)
			}
		} else if file.Content != nil {
			return fmt.Errorf("workspace reconciliation absent state %q contains bytes", file.Path)
		}
		if totalBytes > MaxReconciliationTotalBytes-len(file.Content) {
			return fmt.Errorf("workspace reconciliation exceeds %d total desired bytes", MaxReconciliationTotalBytes)
		}
		totalBytes += len(file.Content)
	}
	if plan.ID != reconciliationPlanID(plan) {
		return fmt.Errorf("workspace reconciliation plan ID differs from its exact authority")
	}
	return nil
}

func (plan ReconciliationPlan) validateSource(source Snapshot) error {
	if err := plan.Validate(); err != nil {
		return err
	}
	if err := source.Validate(); err != nil {
		return fmt.Errorf("workspace reconciliation plan source: %w", err)
	}
	if plan.WorkspaceID != source.WorkspaceID || plan.Root != source.Root ||
		plan.SourceStateID != source.ID || !slices.Equal(plan.Paths, source.Paths) {
		return fmt.Errorf("workspace reconciliation plan differs from its exact source state")
	}
	entries := make(map[string]Entry, len(source.Entries))
	for _, entry := range source.Entries {
		entries[entry.Path] = entry
	}
	for _, transition := range plan.Files {
		if reconciliationState(entries[transition.Path]) != transition.Source {
			return fmt.Errorf(
				"workspace reconciliation source state for %q differs from its captured root",
				transition.Path,
			)
		}
	}
	expectedStateID, err := expectedReconciliationStateID(source, plan.Files)
	if err != nil {
		return err
	}
	if expectedStateID != plan.ExpectedStateID {
		return fmt.Errorf("workspace reconciliation expected state differs from its exact source transitions")
	}
	return nil
}

func (plan ReconciliationPlan) VerifyExpected(snapshot Snapshot) error {
	if err := plan.Validate(); err != nil {
		return err
	}
	if err := snapshot.Validate(); err != nil {
		return fmt.Errorf("verify workspace reconciliation post-state: %w", err)
	}
	if snapshot.WorkspaceID != plan.WorkspaceID || snapshot.Root != plan.Root ||
		snapshot.ID != plan.ExpectedStateID || !slices.Equal(snapshot.Paths, plan.Paths) {
		return fmt.Errorf(
			"workspace reconciliation post-state differs from expected state %s; observed %s",
			plan.ExpectedStateID, snapshot.ID,
		)
	}
	entries := make(map[string]Entry, len(snapshot.Entries))
	for _, entry := range snapshot.Entries {
		entries[entry.Path] = entry
	}
	for _, transition := range plan.Files {
		entry, present := entries[transition.Path]
		if present != transition.Expected.Present {
			return fmt.Errorf("workspace reconciliation post-state has wrong presence for %q", transition.Path)
		}
		if present && reconciliationState(entry) != transition.Expected {
			return fmt.Errorf("workspace reconciliation post-state differs for %q", transition.Path)
		}
	}
	return nil
}

func reconciliationPlanID(plan ReconciliationPlan) string {
	values := []string{
		plan.OwnerID, plan.WorkspaceID, plan.Root, plan.SourceStateID, plan.ExpectedStateID,
		strconv.Itoa(len(plan.Paths)),
	}
	values = append(values, plan.Paths...)
	values = append(values, strconv.Itoa(len(plan.Files)))
	for _, file := range plan.Files {
		values = append(values, file.Path)
		values = appendReconciliationStateIdentity(values, file.Source)
		values = appendReconciliationStateIdentity(values, file.Expected)
	}
	for _, move := range plan.Moves {
		values = append(values, move.SourcePath, move.DestinationPath)
	}
	return opaqueID("workspace_reconciliation_", values...)
}

func appendReconciliationStateIdentity(
	values []string,
	state ReconciliationFileState,
) []string {
	return append(
		values,
		strconv.FormatBool(state.Present), string(state.Kind), state.SHA256,
		strconv.FormatInt(state.Size, 10), strconv.FormatUint(uint64(state.Mode), 10),
		state.LinkTarget,
	)
}

func expectedReconciliationStateID(
	source Snapshot,
	transitions []ReconciliationFileTransition,
) (string, error) {
	entries := make(map[string]Entry, len(source.Entries)+len(transitions))
	for _, entry := range source.Entries {
		entries[entry.Path] = entry
	}
	for _, transition := range transitions {
		if transition.Expected.Present {
			entries[transition.Path] = Entry{
				ID: opaqueID("workspace_file_", source.WorkspaceID, transition.Path),
				Path: transition.Path, Kind: EntryRegular,
				SHA256: transition.Expected.SHA256, Size: transition.Expected.Size,
				Mode: transition.Expected.Mode,
			}
		} else {
			delete(entries, transition.Path)
		}
	}
	expectedEntries := make([]Entry, 0, len(entries))
	for _, entry := range entries {
		expectedEntries = append(expectedEntries, entry)
	}
	sort.Slice(expectedEntries, func(left, right int) bool {
		return expectedEntries[left].Path < expectedEntries[right].Path
	})
	if err := validateReconciliationEntryTopology(expectedEntries); err != nil {
		return "", err
	}
	return stateIDForEntries(source.Schema, source.WorkspaceID, source.Paths, expectedEntries)
}

func validateReconciliationEntryTopology(entries []Entry) error {
	byPath := make(map[string]EntryKind, len(entries))
	for _, entry := range entries {
		byPath[entry.Path] = entry.Kind
	}
	for _, entry := range entries {
		for parent := filepath.ToSlash(filepath.Dir(filepath.FromSlash(entry.Path)));
			parent != ".";
			parent = filepath.ToSlash(filepath.Dir(filepath.FromSlash(parent))) {
			if kind, conflict := byPath[parent]; conflict && kind != EntryDirectory {
				return fmt.Errorf(
					"workspace reconciliation expected file %q conflicts with parent file %q",
					entry.Path, parent,
				)
			}
		}
	}
	return nil
}

func stateIDForEntries(
	schema, workspaceID string,
	paths []string,
	entries []Entry,
) (string, error) {
	identities := make([]stateEntryIdentity, len(entries))
	for index, entry := range entries {
		identities[index] = stateEntryIdentity{
			Path: entry.Path, Kind: entry.Kind, SHA256: entry.SHA256,
			Size: entry.Size, Mode: entry.Mode, LinkTarget: entry.LinkTarget,
		}
	}
	raw, err := canonicalStateIdentityJSON(snapshotIdentity{
		Schema: schema, WorkspaceID: workspaceID,
		Paths: append([]string(nil), paths...), Entries: identities,
	})
	if err != nil {
		return "", err
	}
	digest := sha256.Sum256(raw)
	return "workspace_state_" + hex.EncodeToString(digest[:]), nil
}
