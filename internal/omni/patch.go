package omni

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

type PatchApplyOptions struct {
	Context   context.Context
	Workspace string
	Patch     string
	DryRun    bool
}

type PatchApplyResult struct {
	Workspace string            `json:"workspace"`
	DryRun    bool              `json:"dry_run"`
	Files     []PatchFileResult `json:"files"`
}

type PatchFileResult struct {
	Path   string `json:"path"`
	Action string `json:"action"`
}

type parsedPatchFile struct {
	oldPath string
	newPath string
	hunks   []parsedPatchHunk
}

type parsedPatchHunk struct {
	oldStart int
	lines    []string
}

var unifiedHunkHeaderPattern = regexp.MustCompile(`^@@ -([0-9]+)(?:,[0-9]+)? \+([0-9]+)(?:,[0-9]+)? @@`)

func ApplyUnifiedPatch(options PatchApplyOptions) (PatchApplyResult, error) {
	ctx := options.Context
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return PatchApplyResult{}, fmt.Errorf("apply patch: %w", err)
	}
	workspace := strings.TrimSpace(options.Workspace)
	if workspace == "" {
		var err error
		workspace, err = os.Getwd()
		if err != nil {
			return PatchApplyResult{}, fmt.Errorf("resolve workspace: %w", err)
		}
	}
	absWorkspace, err := filepath.Abs(workspace)
	if err != nil {
		return PatchApplyResult{}, fmt.Errorf("resolve workspace: %w", err)
	}
	files, err := parseUnifiedPatch(options.Patch)
	if err != nil {
		return PatchApplyResult{}, err
	}
	mutations, result, err := preparePatchMutations(ctx, absWorkspace, files, options.DryRun)
	if err != nil || options.DryRun {
		return result, err
	}
	if err := commitPatchMutations(ctx, absWorkspace, mutations); err != nil {
		return PatchApplyResult{}, err
	}
	return result, nil
}

func FormatPatchApplyResult(result PatchApplyResult) string {
	status := "Patch applied"
	if result.DryRun {
		status = "Patch dry-run passed"
	}
	lines := []string{fmt.Sprintf("%s: %s", status, result.Workspace)}
	for _, file := range result.Files {
		lines = append(lines, fmt.Sprintf("%s %s", file.Action, file.Path))
	}
	return strings.Join(lines, "\n") + "\n"
}

func parseUnifiedPatch(patch string) ([]parsedPatchFile, error) {
	lines := strings.Split(strings.ReplaceAll(patch, "\r\n", "\n"), "\n")
	var files []parsedPatchFile
	var current *parsedPatchFile
	var currentHunk *parsedPatchHunk
	for _, line := range lines {
		if strings.HasPrefix(line, "diff --git ") {
			if current != nil {
				files = append(files, *current)
			}
			current = &parsedPatchFile{}
			currentHunk = nil
			continue
		}
		if current == nil {
			continue
		}
		switch {
		case strings.HasPrefix(line, "--- "):
			current.oldPath = cleanPatchPath(strings.TrimSpace(strings.TrimPrefix(line, "--- ")))
		case strings.HasPrefix(line, "+++ "):
			current.newPath = cleanPatchPath(strings.TrimSpace(strings.TrimPrefix(line, "+++ ")))
		case strings.HasPrefix(line, "@@ "):
			matches := unifiedHunkHeaderPattern.FindStringSubmatch(line)
			if len(matches) == 0 {
				return nil, fmt.Errorf("invalid hunk header: %s", line)
			}
			oldStart, err := strconv.Atoi(matches[1])
			if err != nil {
				return nil, fmt.Errorf("invalid hunk old start: %w", err)
			}
			current.hunks = append(current.hunks, parsedPatchHunk{oldStart: oldStart})
			currentHunk = &current.hunks[len(current.hunks)-1]
		case currentHunk != nil && len(line) > 0 && strings.ContainsRune(" +-", rune(line[0])):
			currentHunk.lines = append(currentHunk.lines, line)
		case strings.HasPrefix(line, `\ No newline at end of file`):
			continue
		case currentHunk != nil && line != "":
			return nil, fmt.Errorf("invalid hunk line: expected space, +, or - prefix, received %q", trimPatchDiagnostic(line))
		}
	}
	if current != nil {
		files = append(files, *current)
	}
	if len(files) == 0 {
		return nil, fmt.Errorf("patch contains no file diffs")
	}
	for _, file := range files {
		if file.targetPath() == "" || len(file.hunks) == 0 {
			return nil, fmt.Errorf("patch file is missing target path or hunks")
		}
		if !file.hasChangeLine() {
			return nil, fmt.Errorf("patch file %s contains no change lines", file.targetPath())
		}
	}
	return files, nil
}

func (file parsedPatchFile) hasChangeLine() bool {
	for _, hunk := range file.hunks {
		for _, line := range hunk.lines {
			if line != "" && (line[0] == '+' || line[0] == '-') {
				return true
			}
		}
	}
	return false
}

func trimPatchDiagnostic(line string) string {
	const limit = 160
	if len(line) <= limit {
		return line
	}
	return line[:limit] + "..."
}

func applyParsedPatchContent(originalContent []byte, file parsedPatchFile) (string, error) {
	var original []string
	if file.oldPath != "/dev/null" {
		original = splitPatchText(string(originalContent))
	}
	next := make([]string, 0, len(original))
	cursor := 0
	for _, hunk := range file.hunks {
		hunkStart := hunk.oldStart - 1
		if file.oldPath == "/dev/null" && hunk.oldStart == 0 {
			hunkStart = 0
		}
		if hunkStart < cursor || hunkStart > len(original) {
			return "", fmt.Errorf("hunk starts outside target file: %s", file.targetPath())
		}
		next = append(next, original[cursor:hunkStart]...)
		cursor = hunkStart
		for _, line := range hunk.lines {
			if line == "" {
				continue
			}
			prefix := line[0]
			content := line[1:]
			switch prefix {
			case ' ':
				if cursor >= len(original) || original[cursor] != content {
					return "", fmt.Errorf("patch context mismatch in %s", file.targetPath())
				}
				next = append(next, content)
				cursor++
			case '-':
				if cursor >= len(original) || original[cursor] != content {
					return "", fmt.Errorf("patch removal mismatch in %s", file.targetPath())
				}
				cursor++
			case '+':
				next = append(next, content)
			default:
				return "", fmt.Errorf("unsupported patch line in %s", file.targetPath())
			}
		}
	}
	next = append(next, original[cursor:]...)
	return strings.Join(next, "\n") + "\n", nil
}

func (file parsedPatchFile) targetPath() string {
	if file.newPath != "" && file.newPath != "/dev/null" {
		return file.newPath
	}
	if file.oldPath != "" && file.oldPath != "/dev/null" {
		return file.oldPath
	}
	return ""
}

func (file parsedPatchFile) action() string {
	switch {
	case file.oldPath == "/dev/null":
		return "create"
	case file.newPath == "/dev/null":
		return "delete"
	default:
		return "update"
	}
}

func cleanPatchPath(path string) string {
	path = strings.TrimSpace(path)
	path = strings.TrimPrefix(path, "a/")
	path = strings.TrimPrefix(path, "b/")
	return path
}

func splitPatchText(text string) []string {
	text = strings.ReplaceAll(text, "\r\n", "\n")
	text = strings.TrimSuffix(text, "\n")
	if text == "" {
		return nil
	}
	return strings.Split(text, "\n")
}

func safeWorkspacePath(absWorkspace, relPath string) (string, error) {
	if filepath.IsAbs(relPath) {
		return "", fmt.Errorf("patch path must be relative: %s", relPath)
	}
	cleaned := filepath.Clean(relPath)
	if cleaned == "." || strings.HasPrefix(cleaned, ".."+string(filepath.Separator)) || cleaned == ".." {
		return "", fmt.Errorf("patch path escapes workspace: %s", relPath)
	}
	absTarget := filepath.Join(absWorkspace, cleaned)
	rel, err := filepath.Rel(absWorkspace, absTarget)
	if err != nil {
		return "", fmt.Errorf("resolve patch target: %w", err)
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("patch path escapes workspace: %s", relPath)
	}
	if err := rejectPatchSymlinkEscape(absWorkspace, absTarget, relPath); err != nil {
		return "", err
	}
	return absTarget, nil
}

func rejectPatchSymlinkEscape(absWorkspace, absTarget, relPath string) error {
	resolvedWorkspace, err := filepath.EvalSymlinks(absWorkspace)
	if err != nil {
		return fmt.Errorf("resolve workspace symlinks: %w", err)
	}
	probe := absTarget
	remainder := make([]string, 0, 4)
	for {
		if _, err := os.Lstat(probe); err == nil {
			break
		} else if !os.IsNotExist(err) {
			return fmt.Errorf("inspect patch target %s: %w", relPath, err)
		}
		parent := filepath.Dir(probe)
		if parent == probe {
			return fmt.Errorf("resolve patch target %s: no existing parent", relPath)
		}
		remainder = append([]string{filepath.Base(probe)}, remainder...)
		probe = parent
	}
	resolvedProbe, err := filepath.EvalSymlinks(probe)
	if err != nil {
		return fmt.Errorf("resolve patch target symlinks %s: %w", relPath, err)
	}
	resolvedTarget := resolvedProbe
	if len(remainder) > 0 {
		resolvedTarget = filepath.Join(append([]string{resolvedProbe}, remainder...)...)
	}
	if !isWithinWorkspace(resolvedWorkspace, resolvedTarget) {
		return fmt.Errorf("patch path escapes workspace through symlink: %s", relPath)
	}
	return nil
}
