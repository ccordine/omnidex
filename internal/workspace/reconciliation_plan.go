package workspace

import (
	"context"
	"fmt"
	"path/filepath"
	"sort"
	"strings"
)

func PlanReconciliation(
	ctx context.Context,
	root string,
	ownerID string,
	desired []DesiredFile,
) (ReconciliationPlan, error) {
	if ctx == nil {
		return ReconciliationPlan{}, fmt.Errorf("workspace reconciliation planning requires a context")
	}
	if err := ctx.Err(); err != nil {
		return ReconciliationPlan{}, fmt.Errorf("workspace reconciliation planning: %w", err)
	}
	if !validReconciliationOwnerID(ownerID) {
		return ReconciliationPlan{}, fmt.Errorf("workspace reconciliation requires one exact opaque owner ID")
	}
	if len(desired) > MaxReconciliationFiles {
		return ReconciliationPlan{}, fmt.Errorf("workspace reconciliation exceeds %d desired files", MaxReconciliationFiles)
	}
	paths := make([]string, 0, len(desired))
	seenPaths := make(map[string]struct{}, len(desired))
	totalBytes := 0
	for _, state := range desired {
		if err := validateMutablePath(state.Path); err != nil {
			return ReconciliationPlan{}, err
		}
		if _, duplicate := seenPaths[state.Path]; duplicate {
			return ReconciliationPlan{}, fmt.Errorf("workspace desired state repeats path %q", state.Path)
		}
		seenPaths[state.Path] = struct{}{}
		paths = append(paths, state.Path)
		if !state.Present && (state.Content != nil || state.Mode != 0) {
			return ReconciliationPlan{}, fmt.Errorf("absent workspace desired state %q contains file authority", state.Path)
		}
		if state.Present && state.Mode&^uint32(0o777) != 0 {
			return ReconciliationPlan{}, fmt.Errorf("workspace desired state %q has invalid permission bits", state.Path)
		}
		if len(state.Content) > MaxReconciliationFileBytes {
			return ReconciliationPlan{}, fmt.Errorf(
				"workspace desired file %q exceeds %d bytes", state.Path, MaxReconciliationFileBytes,
			)
		}
		if totalBytes > MaxReconciliationTotalBytes-len(state.Content) {
			return ReconciliationPlan{}, fmt.Errorf(
				"workspace desired content exceeds %d total bytes", MaxReconciliationTotalBytes,
			)
		}
		totalBytes += len(state.Content)
	}
	sort.Strings(paths)
	source, err := Capture(ctx, root, paths)
	if err != nil {
		return ReconciliationPlan{}, fmt.Errorf("capture workspace reconciliation source: %w", err)
	}

	transitions, err := deriveReconciliationFiles(ctx, source, desired)
	if err != nil {
		return ReconciliationPlan{}, err
	}
	sort.Slice(transitions, func(left, right int) bool {
		return transitions[left].Path < transitions[right].Path
	})
	expectedStateID, err := expectedReconciliationStateID(source, transitions)
	if err != nil {
		return ReconciliationPlan{}, err
	}
	plan := ReconciliationPlan{
		Schema: ReconciliationPlanSchemaV1, OwnerID: ownerID,
		WorkspaceID: source.WorkspaceID, Root: source.Root,
		SourceStateID: source.ID, ExpectedStateID: expectedStateID,
		Paths: paths, Files: transitions, Moves: deriveReconciliationMoves(transitions),
	}
	plan.ID = reconciliationPlanID(plan)
	if err := plan.validateSource(source); err != nil {
		return ReconciliationPlan{}, err
	}
	return plan, nil
}

func deriveReconciliationMoves(
	transitions []ReconciliationFileTransition,
) []ReconciliationMove {
	type contentIdentity struct {
		digest string
		size   int64
	}
	sources := make(map[contentIdentity][]string)
	destinations := make(map[contentIdentity][]string)
	for _, transition := range transitions {
		if transition.Source.Present && transition.Source.Kind == EntryRegular &&
			!transition.Expected.Present {
			identity := contentIdentity{digest: transition.Source.SHA256, size: transition.Source.Size}
			sources[identity] = append(sources[identity], transition.Path)
		}
		if !transition.Source.Present && transition.Expected.Present {
			identity := contentIdentity{digest: transition.Expected.SHA256, size: transition.Expected.Size}
			destinations[identity] = append(destinations[identity], transition.Path)
		}
	}
	moves := make([]ReconciliationMove, 0)
	for identity, sourcePaths := range sources {
		destinationPaths := destinations[identity]
		sort.Strings(sourcePaths)
		sort.Strings(destinationPaths)
		count := min(len(sourcePaths), len(destinationPaths))
		for index := 0; index < count; index++ {
			moves = append(moves, ReconciliationMove{
				SourcePath: sourcePaths[index], DestinationPath: destinationPaths[index],
			})
		}
	}
	sort.Slice(moves, func(left, right int) bool {
		if moves[left].SourcePath != moves[right].SourcePath {
			return moves[left].SourcePath < moves[right].SourcePath
		}
		return moves[left].DestinationPath < moves[right].DestinationPath
	})
	return moves
}

func deriveReconciliationFiles(
	ctx context.Context,
	source Snapshot,
	desired []DesiredFile,
) ([]ReconciliationFileTransition, error) {
	entries := make(map[string]Entry, len(source.Entries))
	for _, entry := range source.Entries {
		entries[entry.Path] = entry
	}
	seen := make(map[string]struct{}, len(desired))
	transitions := make([]ReconciliationFileTransition, 0, len(desired))
	for _, state := range desired {
		if err := ctx.Err(); err != nil {
			return nil, fmt.Errorf("derive workspace reconciliation: %w", err)
		}
		if err := validateMutablePath(state.Path); err != nil {
			return nil, err
		}
		if _, duplicate := seen[state.Path]; duplicate {
			return nil, fmt.Errorf("workspace desired state repeats path %q", state.Path)
		}
		seen[state.Path] = struct{}{}
		transition, changed, err := deriveReconciliationFile(entries[state.Path], state)
		if err != nil {
			return nil, err
		}
		if changed {
			transitions = append(transitions, transition)
		}
	}
	return transitions, nil
}

func deriveReconciliationFile(
	entry Entry,
	desired DesiredFile,
) (ReconciliationFileTransition, bool, error) {
	present := entry.Path != ""
	if !desired.Present {
		if desired.Content != nil || desired.Mode != 0 {
			return ReconciliationFileTransition{}, false, fmt.Errorf(
				"absent workspace desired state %q contains file authority", desired.Path,
			)
		}
		if !present {
			return ReconciliationFileTransition{}, false, nil
		}
		return ReconciliationFileTransition{
			Path: desired.Path, Source: reconciliationState(entry),
		}, true, nil
	}
	if desired.Mode&^uint32(0o777) != 0 {
		return ReconciliationFileTransition{}, false, fmt.Errorf(
			"workspace desired state %q has invalid permission bits", desired.Path,
		)
	}
	expected := ReconciliationFileState{
		Present: true, Kind: EntryRegular, SHA256: digestBytes(desired.Content),
		Size: int64(len(desired.Content)), Mode: desired.Mode,
	}
	if present && entry.Kind == EntryRegular && entry.SHA256 == expected.SHA256 &&
		entry.Size == expected.Size && entry.Mode == expected.Mode {
		return ReconciliationFileTransition{}, false, nil
	}
	return ReconciliationFileTransition{
		Path: desired.Path, Source: reconciliationState(entry), Expected: expected,
		Content: append([]byte(nil), desired.Content...),
	}, true, nil
}

func validateMutablePath(value string) error {
	if err := validateRelativePath(value); err != nil {
		return fmt.Errorf("workspace reconciliation path: %w", err)
	}
	return nil
}

func reconciliationPathDepth(relative string) int {
	return strings.Count(filepath.ToSlash(relative), "/")
}
