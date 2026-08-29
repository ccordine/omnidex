package worker

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/gryph/omnidex/internal/evidence"
	"github.com/gryph/omnidex/internal/omni"
	"github.com/gryph/omnidex/internal/operation"
	workspacefacts "github.com/gryph/omnidex/internal/workspace"
)

const (
	maxV3WriteBytes       = 128 * 1024
	maxV3DiffPreviewBytes = 24 * 1024
)

type workspaceFileOperation string

const (
	workspaceDirectoryEnsure workspaceFileOperation = "ensure_directory"
	workspaceFileCreate      workspaceFileOperation = "create"
	workspaceFileReplace     workspaceFileOperation = "replace"
	workspaceFileDelete      workspaceFileOperation = "delete"
)

type workspaceFileMutation struct {
	Path      string
	Operation workspaceFileOperation
	Content   string
}

func applyWorkspaceFileMutation(ctx context.Context, root string, command workspaceFileMutation) (operation.Result, error) {
	if ctx == nil {
		return operation.Result{}, fmt.Errorf("workspace mutation requires a context")
	}
	root = strings.TrimSpace(root)
	if root == "" || !filepath.IsAbs(root) {
		return operation.Result{}, fmt.Errorf("workspace mutation requires one absolute server-authoritative root")
	}
	path := strings.TrimSpace(command.Path)
	fileOperation := command.Operation
	content := command.Content
	if len(content) > maxV3WriteBytes {
		return operation.Result{}, operation.Reject(fmt.Errorf("workspace.write content exceeds the %d-byte limit", maxV3WriteBytes))
	}
	target, err := resolveV3WorkspaceFile(root, path)
	if err != nil {
		return operation.Result{}, operation.Reject(err)
	}

	var original []byte
	switch fileOperation {
	case workspaceFileCreate:
		if content == "" || !strings.HasSuffix(content, "\n") {
			return operation.Result{}, operation.Reject(fmt.Errorf("workspace.write create content must be non-empty and end with a newline"))
		}
		if _, err := os.Lstat(target); err == nil {
			return operation.Result{}, operation.Reject(fmt.Errorf("workspace.write create target already exists: %s", path))
		} else if !os.IsNotExist(err) {
			return operation.Result{}, operation.Reject(fmt.Errorf("inspect workspace.write create target %s: %w", path, err))
		}
	case workspaceFileReplace, workspaceFileDelete:
		info, err := os.Lstat(target)
		if err != nil {
			return operation.Result{}, operation.Reject(fmt.Errorf("workspace.write %s target is unavailable: %s: %w", fileOperation, path, err))
		}
		if !info.Mode().IsRegular() {
			return operation.Result{}, operation.Reject(fmt.Errorf("workspace.write %s target is not a regular file: %s", fileOperation, path))
		}
		if info.Size() > maxV3WriteBytes {
			return operation.Result{}, operation.Reject(fmt.Errorf(
				"workspace.write %s target exceeds the %d-byte read bound: %s",
				fileOperation, maxV3WriteBytes, path,
			))
		}
		original, err = os.ReadFile(target)
		if err != nil {
			return operation.Result{}, operation.Reject(fmt.Errorf("read workspace.write target %s: %w", path, err))
		}
		if fileOperation == workspaceFileReplace {
			if content == "" || !strings.HasSuffix(content, "\n") {
				return operation.Result{}, operation.Reject(fmt.Errorf("workspace.write replace content must be non-empty and end with a newline"))
			}
			if bytes.Equal(original, []byte(content)) {
				return operation.Result{}, operation.Reject(fmt.Errorf("workspace.write replace does not change target %s", path))
			}
		} else if content != "" {
			return operation.Result{}, operation.Reject(fmt.Errorf("workspace.write delete content must be empty"))
		}
	default:
		return operation.Result{}, operation.Reject(fmt.Errorf("workspace.write operation must be create, replace, or delete; received %q", fileOperation))
	}

	operationText := string(fileOperation)
	patch, err := workspacefacts.BuildFullFileUnifiedPatch(
		path,
		fileOperation != workspaceFileCreate,
		original,
		fileOperation != workspaceFileDelete,
		[]byte(content),
	)
	if err != nil {
		return operation.Result{}, operation.Reject(fmt.Errorf("workspace.write patch construction rejected: %w", err))
	}
	diff, diffTruncated := boundedV3DiffPreview(patch)
	preview, err := omni.ApplyUnifiedPatch(omni.PatchApplyOptions{Context: ctx, Workspace: root, Patch: patch, DryRun: true})
	if err != nil {
		return operation.Result{}, operation.Reject(fmt.Errorf("workspace.write validation rejected: %w", err))
	}
	if len(preview.Files) != 1 {
		return operation.Result{}, fmt.Errorf("workspace.write internal validation produced %d file mutations; expected exactly one", len(preview.Files))
	}
	applied, err := omni.ApplyUnifiedPatch(omni.PatchApplyOptions{Context: ctx, Workspace: root, Patch: patch})
	if err != nil {
		return operation.Result{}, fmt.Errorf("workspace.write apply failed after successful validation: %w", err)
	}

	summary := fmt.Sprintf("%s complete file %s", pastTenseWriteOperation(operationText), path)
	return operation.Result{
		Summary: summary,
		Output: map[string]any{
			"summary": summary, "workspace": applied.Workspace, "path": path,
			"operation": operationText, "diff": diff, "diff_truncated": diffTruncated,
		},
		Evidence: []evidence.Record{{
			Kind: evidence.KindGeneratedDiff, SourceType: "workspace", SourceRef: path,
			FilePaths: []string{path}, Excerpt: operationText + " " + path, Summary: summary, Confidence: 1,
			Metadata: map[string]any{"mutation": true, "succeeded": true, "workspace": applied.Workspace, "operation": operationText},
		}},
	}, nil
}

func boundedV3DiffPreview(diff string) (string, bool) {
	if len(diff) <= maxV3DiffPreviewBytes {
		return diff, false
	}
	marker := fmt.Sprintf("\n[diff truncated: original=%d bytes; inspect the authoritative file for complete content]\n", len(diff))
	budget := maxV3DiffPreviewBytes - len(marker)
	if budget < 0 {
		return marker, true
	}
	return diff[:budget] + marker, true
}

func resolveV3WorkspaceFile(root, path string) (string, error) {
	if path == "" || strings.ContainsAny(path, "\x00\r\n") || filepath.IsAbs(path) {
		return "", fmt.Errorf("workspace.write path must be a non-empty relative path")
	}
	clean := filepath.Clean(path)
	if clean == "." || clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("workspace.write path escapes the workspace: %s", path)
	}
	if err := validateV3WritePath(filepath.ToSlash(clean)); err != nil {
		return "", err
	}
	target := filepath.Join(root, clean)
	current := root
	parts := strings.Split(clean, string(filepath.Separator))
	for index, part := range parts {
		current = filepath.Join(current, part)
		info, err := os.Lstat(current)
		if os.IsNotExist(err) {
			break
		}
		if err != nil {
			return "", fmt.Errorf("inspect workspace.write path %s: %w", path, err)
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return "", fmt.Errorf("workspace.write path traverses a symlink: %s", path)
		}
		if index < len(parts)-1 && !info.IsDir() {
			return "", fmt.Errorf("workspace.write parent is not a directory: %s", path)
		}
	}
	return target, nil
}

func validateV3WritePath(path string) error {
	first, _, _ := strings.Cut(strings.ToLower(path), "/")
	switch first {
	case ".git", ".omni", "node_modules", "vendor":
		return fmt.Errorf("workspace.write target %q is protected", path)
	}
	if strings.EqualFold(path, ".env") || strings.HasPrefix(strings.ToLower(path), ".env.") && !strings.EqualFold(path, ".env.example") {
		return fmt.Errorf("workspace.write target %q may contain secrets and is protected", path)
	}
	return nil
}

func pastTenseWriteOperation(operation string) string {
	switch operation {
	case "create":
		return "Created"
	case "replace":
		return "Replaced"
	case "delete":
		return "Deleted"
	default:
		return "Changed"
	}
}
