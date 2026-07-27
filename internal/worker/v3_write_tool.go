package worker

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/gryph/omnidex/internal/evidence"
	"github.com/gryph/omnidex/internal/omni"
	toolruntime "github.com/gryph/omnidex/internal/tools"
)

const maxV3WriteBytes = 128 * 1024

func workspaceWriteToolSpec() toolruntime.Spec {
	return toolruntime.Spec{
		Name:        "workspace.write",
		Description: "Atomically create, replace, or delete exactly one complete text file inside the server-authoritative job workspace. Supply complete file content, not a diff.",
		InputSchema: toolruntime.Schema{
			Type:     "object",
			Required: []string{"path", "operation", "content"},
			Properties: map[string]toolruntime.Schema{
				"path":      {Type: "string"},
				"operation": {Type: "string", Enum: []string{"create", "replace", "delete"}},
				"content":   {Type: "string", Description: "Complete UTF-8 file content. Must be empty for delete and end with a newline for create or replace."},
			},
		},
		OutputSchema: toolruntime.Schema{
			Type:     "object",
			Required: []string{"summary", "workspace", "path", "operation", "content_sha256"},
			Properties: map[string]toolruntime.Schema{
				"summary":        {Type: "string"},
				"workspace":      {Type: "string"},
				"path":           {Type: "string"},
				"operation":      {Type: "string"},
				"content_sha256": {Type: "string"},
			},
		},
		Examples: []toolruntime.Example{{
			When: "Create one complete source file in the assigned workspace.",
			Input: map[string]any{
				"path": "main.go", "operation": "create", "content": "package main\n\nfunc main() {}\n",
			},
		}},
		RequireEvidence: true,
	}
}

func executeV3WorkspaceWrite(ctx context.Context, call toolruntime.Call) (toolruntime.Result, error) {
	scope, err := v3WorkspaceScopeFromContext(ctx)
	if err != nil {
		return toolruntime.Result{}, err
	}
	path := strings.TrimSpace(toolInputString(call.Input, "path"))
	operation := strings.ToLower(strings.TrimSpace(toolInputString(call.Input, "operation")))
	content, ok := call.Input["content"].(string)
	if !ok {
		return toolruntime.Result{}, toolruntime.RejectCall(fmt.Errorf("workspace.write content must be a string"))
	}
	if len(content) > maxV3WriteBytes {
		return toolruntime.Result{}, toolruntime.RejectCall(fmt.Errorf("workspace.write content exceeds the %d-byte limit", maxV3WriteBytes))
	}
	target, err := resolveV3WorkspaceFile(scope.Root, path)
	if err != nil {
		return toolruntime.Result{}, toolruntime.RejectCall(err)
	}

	var original []byte
	switch operation {
	case "create":
		if content == "" || !strings.HasSuffix(content, "\n") {
			return toolruntime.Result{}, toolruntime.RejectCall(fmt.Errorf("workspace.write create content must be non-empty and end with a newline"))
		}
		if _, err := os.Lstat(target); err == nil {
			return toolruntime.Result{}, toolruntime.RejectCall(fmt.Errorf("workspace.write create target already exists: %s", path))
		} else if !os.IsNotExist(err) {
			return toolruntime.Result{}, toolruntime.RejectCall(fmt.Errorf("inspect workspace.write create target %s: %w", path, err))
		}
	case "replace", "delete":
		info, err := os.Lstat(target)
		if err != nil {
			return toolruntime.Result{}, toolruntime.RejectCall(fmt.Errorf("workspace.write %s target is unavailable: %s: %w", operation, path, err))
		}
		if !info.Mode().IsRegular() {
			return toolruntime.Result{}, toolruntime.RejectCall(fmt.Errorf("workspace.write %s target is not a regular file: %s", operation, path))
		}
		original, err = os.ReadFile(target)
		if err != nil {
			return toolruntime.Result{}, toolruntime.RejectCall(fmt.Errorf("read workspace.write target %s: %w", path, err))
		}
		if operation == "replace" {
			if content == "" || !strings.HasSuffix(content, "\n") {
				return toolruntime.Result{}, toolruntime.RejectCall(fmt.Errorf("workspace.write replace content must be non-empty and end with a newline"))
			}
			if bytes.Equal(original, []byte(content)) {
				return toolruntime.Result{}, toolruntime.RejectCall(fmt.Errorf("workspace.write replace does not change target %s", path))
			}
		} else if content != "" {
			return toolruntime.Result{}, toolruntime.RejectCall(fmt.Errorf("workspace.write delete content must be empty"))
		}
	default:
		return toolruntime.Result{}, toolruntime.RejectCall(fmt.Errorf("workspace.write operation must be create, replace, or delete; received %q", operation))
	}

	patch := fullFileUnifiedPatch(path, operation, string(original), content)
	preview, err := omni.ApplyUnifiedPatch(omni.PatchApplyOptions{Context: ctx, Workspace: scope.Root, Patch: patch, DryRun: true})
	if err != nil {
		return toolruntime.Result{}, toolruntime.RejectCall(fmt.Errorf("workspace.write validation rejected: %w", err))
	}
	if len(preview.Files) != 1 {
		return toolruntime.Result{}, fmt.Errorf("workspace.write internal validation produced %d file mutations; expected exactly one", len(preview.Files))
	}
	applied, err := omni.ApplyUnifiedPatch(omni.PatchApplyOptions{Context: ctx, Workspace: scope.Root, Patch: patch})
	if err != nil {
		return toolruntime.Result{}, fmt.Errorf("workspace.write apply failed after successful validation: %w", err)
	}

	digest := sha256.Sum256([]byte(content))
	digestText := hex.EncodeToString(digest[:])
	summary := fmt.Sprintf("%s complete file %s", pastTenseWriteOperation(operation), path)
	return toolruntime.Result{
		Summary: summary,
		Output: map[string]any{
			"summary": summary, "workspace": applied.Workspace, "path": path,
			"operation": operation, "content_sha256": digestText,
		},
		Evidence: []evidence.Record{{
			Kind: evidence.KindGeneratedDiff, SourceType: "workspace", SourceRef: "sha256:" + digestText,
			FilePaths: []string{path}, Excerpt: operation + " " + path, Summary: summary, Hash: digestText, Confidence: 1,
			Metadata: map[string]any{"mutation": true, "succeeded": true, "workspace": applied.Workspace, "operation": operation},
		}},
	}, nil
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

func fullFileUnifiedPatch(path, operation, original, content string) string {
	oldPath, newPath := "a/"+path, "b/"+path
	if operation == "create" {
		oldPath = "/dev/null"
	}
	if operation == "delete" {
		newPath = "/dev/null"
	}
	oldLines := completeFileLines(original)
	newLines := completeFileLines(content)
	var b strings.Builder
	fmt.Fprintf(&b, "diff --git a/%s b/%s\n--- %s\n+++ %s\n@@ -%d,%d +%d,%d @@\n", path, path, oldPath, newPath, patchStart(len(oldLines)), len(oldLines), patchStart(len(newLines)), len(newLines))
	for _, line := range oldLines {
		b.WriteByte('-')
		b.WriteString(line)
		b.WriteByte('\n')
	}
	for _, line := range newLines {
		b.WriteByte('+')
		b.WriteString(line)
		b.WriteByte('\n')
	}
	return b.String()
}

func completeFileLines(content string) []string {
	content = strings.TrimSuffix(strings.ReplaceAll(content, "\r\n", "\n"), "\n")
	if content == "" {
		return nil
	}
	return strings.Split(content, "\n")
}

func patchStart(lineCount int) int {
	if lineCount == 0 {
		return 0
	}
	return 1
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
