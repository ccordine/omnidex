package workspace

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"
)

type StagedReconciliation struct {
	mu sync.Mutex

	source      Snapshot
	plan        ReconciliationPlan
	root        string
	desiredRoot string
	sourceRoot  string
	closed      bool
	applied     bool
}

func StageReconciliation(
	ctx context.Context,
	plan ReconciliationPlan,
) (_ *StagedReconciliation, resultErr error) {
	if ctx == nil {
		return nil, fmt.Errorf("stage workspace reconciliation requires a context")
	}
	if err := plan.Validate(); err != nil {
		return nil, err
	}
	source, err := Capture(ctx, plan.Root, plan.Paths)
	if err != nil {
		return nil, fmt.Errorf("capture workspace reconciliation stage source: %w", err)
	}
	if err := plan.validateSource(source); err != nil {
		return nil, err
	}
	if err := source.VerifyExact(ctx); err != nil {
		return nil, fmt.Errorf("stage workspace reconciliation source: %w", err)
	}
	stageRoot, err := os.MkdirTemp("", "omnidex-workspace-reconciliation-*")
	if err != nil {
		return nil, fmt.Errorf("create workspace reconciliation stage: %w", err)
	}
	keep := false
	defer func() {
		if !keep {
			resultErr = joinWorkspaceCleanupError(resultErr, os.RemoveAll(stageRoot))
		}
	}()
	desiredRoot := filepath.Join(stageRoot, "desired")
	sourceRoot := filepath.Join(stageRoot, "source")
	for _, directory := range []string{desiredRoot, sourceRoot} {
		if err := os.Mkdir(directory, 0o700); err != nil {
			return nil, fmt.Errorf("create workspace reconciliation inventory: %w", err)
		}
	}
	for _, transition := range plan.Files {
		if err := ctx.Err(); err != nil {
			return nil, fmt.Errorf("stage workspace reconciliation: %w", err)
		}
		if transition.Source.Present && transition.Source.Kind == EntryRegular {
			target := filepath.Join(sourceRoot, filepath.FromSlash(transition.Path))
			if err := copyExactSourceToStage(ctx, source.Root, transition, target); err != nil {
				return nil, err
			}
		}
		if transition.Expected.Present {
			target := filepath.Join(desiredRoot, filepath.FromSlash(transition.Path))
			if err := writeStageBytes(target, transition.Content); err != nil {
				return nil, fmt.Errorf("stage desired workspace bytes for %q: %w", transition.Path, err)
			}
		}
	}
	stage := &StagedReconciliation{
		source: cloneSnapshot(source), plan: cloneReconciliationPlan(plan),
		root: stageRoot, desiredRoot: desiredRoot, sourceRoot: sourceRoot,
	}
	if err := stage.verifyExactStage(ctx); err != nil {
		return nil, err
	}
	if err := source.VerifyExact(ctx); err != nil {
		return nil, fmt.Errorf("workspace source changed while reconciliation was staged: %w", err)
	}
	keep = true
	return stage, nil
}

func copyExactSourceToStage(
	ctx context.Context,
	root string,
	transition ReconciliationFileTransition,
	target string,
) error {
	if err := verifyReconciliationPathState(ctx, root, transition.Path, transition.Source); err != nil {
		return err
	}
	absolute := filepath.Join(root, filepath.FromSlash(transition.Path))
	before, err := os.Lstat(absolute)
	if err != nil {
		return fmt.Errorf("inspect workspace reconciliation source %q: %w", transition.Path, err)
	}
	input, err := os.Open(absolute)
	if err != nil {
		return fmt.Errorf("open workspace reconciliation source %q: %w", transition.Path, err)
	}
	opened, err := input.Stat()
	if err != nil || !os.SameFile(before, opened) {
		return fmt.Errorf("workspace reconciliation source %q changed while it was opened", transition.Path)
	}
	if err := os.MkdirAll(filepath.Dir(target), 0o700); err != nil {
		return fmt.Errorf("create staged source parent for %q: %w", transition.Path, err)
	}
	output, err := os.OpenFile(target, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return fmt.Errorf("create staged source for %q: %w", transition.Path, err)
	}
	hash := sha256.New()
	written, copyErr := io.Copy(io.MultiWriter(output, hash), &contextReader{ctx: ctx, reader: input})
	closeErr := closeSyncedFile(output)
	inputCloseErr := input.Close()
	if copyErr != nil {
		return fmt.Errorf("copy workspace reconciliation source %q: %w", transition.Path, copyErr)
	}
	if closeErr != nil {
		return fmt.Errorf("seal staged source %q: %w", transition.Path, closeErr)
	}
	if inputCloseErr != nil {
		return fmt.Errorf("close workspace reconciliation source %q: %w", transition.Path, inputCloseErr)
	}
	after, err := os.Lstat(absolute)
	if err != nil || !os.SameFile(before, after) || before.Mode() != after.Mode() ||
		before.Size() != after.Size() || !before.ModTime().Equal(after.ModTime()) ||
		written != transition.Source.Size || hex.EncodeToString(hash.Sum(nil)) != transition.Source.SHA256 {
		return fmt.Errorf("workspace reconciliation source %q changed while it was staged", transition.Path)
	}
	return nil
}

func writeStageBytes(target string, content []byte) error {
	if err := os.MkdirAll(filepath.Dir(target), 0o700); err != nil {
		return err
	}
	file, err := os.OpenFile(target, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return err
	}
	written, writeErr := io.Copy(file, bytes.NewReader(content))
	closeErr := closeSyncedFile(file)
	if writeErr != nil {
		return writeErr
	}
	if closeErr != nil {
		return closeErr
	}
	if written != int64(len(content)) {
		return fmt.Errorf("staged byte count differs from desired content")
	}
	return nil
}

func closeSyncedFile(file *os.File) error {
	if err := file.Sync(); err != nil {
		_ = file.Close()
		return err
	}
	return file.Close()
}

func (stage *StagedReconciliation) StageRoot() string {
	if stage == nil {
		return ""
	}
	return stage.root
}

func (stage *StagedReconciliation) SourceSnapshot() Snapshot {
	if stage == nil {
		return Snapshot{}
	}
	stage.mu.Lock()
	defer stage.mu.Unlock()
	return cloneSnapshot(stage.source)
}

func (stage *StagedReconciliation) Plan() ReconciliationPlan {
	if stage == nil {
		return ReconciliationPlan{}
	}
	stage.mu.Lock()
	defer stage.mu.Unlock()
	return cloneReconciliationPlan(stage.plan)
}

func (stage *StagedReconciliation) Cleanup() error {
	if stage == nil {
		return nil
	}
	stage.mu.Lock()
	defer stage.mu.Unlock()
	if stage.closed {
		return nil
	}
	if err := os.RemoveAll(stage.root); err != nil {
		return fmt.Errorf("clean workspace reconciliation stage %q: %w", stage.plan.ID, err)
	}
	stage.closed = true
	return nil
}

func joinWorkspaceCleanupError(primary, cleanup error) error {
	if primary == nil {
		return cleanup
	}
	if cleanup == nil {
		return primary
	}
	return fmt.Errorf("%v; cleanup: %w", primary, cleanup)
}
