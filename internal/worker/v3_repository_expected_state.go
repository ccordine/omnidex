package worker

import (
	"fmt"
	"path"
	"reflect"
	"sort"
	"strings"

	repositoryfacts "github.com/gryph/omnidex/internal/repository"
	"github.com/gryph/omnidex/internal/repository/changeapply"
)

func validateExpectedRepositoryFileState(state changeapply.ExpectedFileState) error {
	if !validRepositoryVerificationOpaqueID(state.FileID, "file_") ||
		state.Path == "" || state.Path != strings.TrimSpace(state.Path) ||
		strings.ContainsAny(state.Path, "\\\x00\r\n") || path.Clean(state.Path) != state.Path ||
		state.Path == "." || state.Path == ".." || strings.HasPrefix(state.Path, "../") {
		return fmt.Errorf("expected repository file state has invalid identity or path")
	}
	if !state.Present {
		if state.SHA256 != "" || state.Size != 0 || state.Mode != 0 {
			return fmt.Errorf("expected absent repository file state contains source authority")
		}
		return nil
	}
	if !validRepositoryVerificationSHA256(state.SHA256) || state.Size < 0 || state.Mode > 0o777 {
		return fmt.Errorf("expected present repository file state has invalid content authority")
	}
	return nil
}

func canonicalExpectedRepositoryFileStates(
	changedFileIDs []string,
	expected []changeapply.ExpectedFileState,
) ([]changeapply.ExpectedFileState, error) {
	if len(changedFileIDs) == 0 || len(changedFileIDs) != len(expected) {
		return nil, fmt.Errorf("expected repository post authority is incomplete")
	}
	files := append([]changeapply.ExpectedFileState(nil), expected...)
	sort.Slice(files, func(left, right int) bool { return files[left].FileID < files[right].FileID })
	changed := append([]string(nil), changedFileIDs...)
	sort.Strings(changed)
	for index, file := range files {
		if err := validateExpectedRepositoryFileState(file); err != nil {
			return nil, err
		}
		if file.FileID != changed[index] || index > 0 && file.FileID == files[index-1].FileID {
			return nil, fmt.Errorf("expected repository post authority is invalid or duplicated")
		}
	}
	return files, nil
}

func validateExactRepositoryPostInventory(
	before repositoryfacts.Snapshot,
	after repositoryfacts.Snapshot,
	expectedFiles []changeapply.ExpectedFileState,
) error {
	expected := make(map[string]changeapply.ExpectedFileState, len(expectedFiles))
	for _, state := range expectedFiles {
		if err := validateExpectedRepositoryFileState(state); err != nil {
			return err
		}
		if _, duplicate := expected[state.FileID]; duplicate {
			return fmt.Errorf("repository expected post authority is duplicated")
		}
		expected[state.FileID] = state
	}
	if len(expected) == 0 {
		return fmt.Errorf("repository expected post authority is empty")
	}
	current := make(map[string]repositoryfacts.File, len(after.Files))
	for _, file := range after.Files {
		current[file.ID] = file
	}
	minimumInventory := len(before.Files)
	for _, state := range expectedFiles {
		if state.Present {
			if _, existed := currentRepositoryFileByID(before, state.FileID); !existed {
				minimumInventory++
			}
		} else {
			minimumInventory--
		}
	}
	if len(after.Files) != minimumInventory {
		return fmt.Errorf("repository verification changed the indexed file inventory outside the contract")
	}
	creates, deletes := 0, 0
	for _, prior := range before.Files {
		state, changed := expected[prior.ID]
		if !changed {
			next, exists := current[prior.ID]
			if !exists || !reflect.DeepEqual(prior, next) {
				return fmt.Errorf("repository verification changed file %q outside the exact contract", prior.ID)
			}
			continue
		}
		if state.Path != prior.Path {
			return fmt.Errorf("repository target %q path differs from source authority", prior.ID)
		}
		delete(expected, prior.ID)
		if !state.Present {
			deletes++
			if _, exists := current[prior.ID]; exists {
				return fmt.Errorf("repository verification retained expected-absent file %q", prior.ID)
			}
			continue
		}
		next, exists := current[prior.ID]
		if !exists {
			return fmt.Errorf("repository verification removed expected-present file %q", prior.ID)
		}
		want := prior
		want.SHA256, want.Size, want.Mode = state.SHA256, state.Size, state.Mode
		if reflect.DeepEqual(prior, want) {
			return fmt.Errorf("repository target file %q has unchanged expected authority", prior.ID)
		}
		if !reflect.DeepEqual(want, next) {
			return fmt.Errorf("repository verification changed file %q outside the exact contract", prior.ID)
		}
	}
	for fileID, state := range expected {
		if !state.Present {
			return fmt.Errorf("repository expected absent file %q is absent from source authority", fileID)
		}
		derivedID, err := repositoryfacts.FileIDForAbsentPath(before, state.Path)
		if err != nil || derivedID != fileID {
			return fmt.Errorf("repository expected created file %q has invalid absent-source authority", fileID)
		}
		next, exists := current[fileID]
		if !exists || next.Path != state.Path || next.Kind != repositoryfacts.EntryRegular ||
			next.SHA256 != state.SHA256 || next.Size != state.Size || next.Mode != state.Mode {
			return fmt.Errorf("repository expected created file %q differs from exact post authority", fileID)
		}
		creates++
	}
	if len(after.Files) != len(before.Files)+creates-deletes {
		return fmt.Errorf("repository verification changed the indexed file inventory outside the contract")
	}
	wantExclusions := make([]repositoryfacts.Exclusion, len(before.Exclusions))
	copy(wantExclusions, before.Exclusions)
	for _, state := range expectedFiles {
		if !state.Present {
			wantExclusions = append(wantExclusions, repositoryfacts.Exclusion{
				Path: state.Path, Reason: repositoryfacts.ExclusionAbsent,
			})
		}
	}
	sort.Slice(wantExclusions, func(left, right int) bool {
		return wantExclusions[left].Path < wantExclusions[right].Path
	})
	if !reflect.DeepEqual(wantExclusions, after.Exclusions) {
		return fmt.Errorf(
			"repository verification changed excluded inventory: expected=%v actual=%v",
			wantExclusions, after.Exclusions,
		)
	}
	return nil
}

func currentRepositoryFileByID(snapshot repositoryfacts.Snapshot, fileID string) (repositoryfacts.File, bool) {
	for _, file := range snapshot.Files {
		if file.ID == fileID {
			return file, true
		}
	}
	return repositoryfacts.File{}, false
}
