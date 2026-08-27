package workspace

import (
	"fmt"
	"strings"
	"unicode/utf8"
)

// BuildFullFileUnifiedPatch encodes one complete text-file state transition.
// Presence determines the operation mechanically; callers cannot supply an
// operation label that disagrees with the source and expected states.
func BuildFullFileUnifiedPatch(
	path string,
	sourcePresent bool,
	original []byte,
	expectedPresent bool,
	expected []byte,
) (string, error) {
	if err := validateMutablePath(path); err != nil {
		return "", err
	}
	if !sourcePresent && !expectedPresent {
		return "", fmt.Errorf("workspace patch path %q is absent in both source and expected state", path)
	}
	if sourcePresent {
		if len(original) > MaxMutationFileBytes || !utf8.Valid(original) || strings.ContainsRune(string(original), '\x00') {
			return "", fmt.Errorf("workspace patch source %q exceeds the byte limit or is not bounded UTF-8 text", path)
		}
	} else if len(original) != 0 {
		return "", fmt.Errorf("workspace patch absent source %q contains bytes", path)
	}
	if expectedPresent {
		if len(expected) == 0 || len(expected) > MaxMutationFileBytes || !utf8.Valid(expected) ||
			strings.ContainsRune(string(expected), '\x00') || !strings.HasSuffix(string(expected), "\n") ||
			strings.Contains(string(expected), "\r") {
			return "", fmt.Errorf("workspace patch expected %q must be bounded newline-terminated UTF-8 text within the byte limit", path)
		}
	} else if len(expected) != 0 {
		return "", fmt.Errorf("workspace patch absent expected state %q contains bytes", path)
	}
	if sourcePresent && expectedPresent && string(original) == string(expected) {
		return "", fmt.Errorf("workspace patch update does not change %q", path)
	}

	oldPath, newPath := "a/"+path, "b/"+path
	if !sourcePresent {
		oldPath = "/dev/null"
	}
	if !expectedPresent {
		newPath = "/dev/null"
	}
	oldLines := completeMutationFileLines(string(original))
	newLines := completeMutationFileLines(string(expected))
	var builder strings.Builder
	fmt.Fprintf(
		&builder,
		"diff --git a/%s b/%s\n--- %s\n+++ %s\n@@ -%d,%d +%d,%d @@\n",
		path, path, oldPath, newPath,
		mutationPatchStart(len(oldLines)), len(oldLines),
		mutationPatchStart(len(newLines)), len(newLines),
	)
	for _, line := range oldLines {
		builder.WriteByte('-')
		builder.WriteString(line)
		builder.WriteByte('\n')
	}
	for _, line := range newLines {
		builder.WriteByte('+')
		builder.WriteString(line)
		builder.WriteByte('\n')
	}
	return builder.String(), nil
}

func completeMutationFileLines(content string) []string {
	content = strings.TrimSuffix(strings.ReplaceAll(content, "\r\n", "\n"), "\n")
	if content == "" {
		return nil
	}
	return strings.Split(content, "\n")
}

func mutationPatchStart(lineCount int) int {
	if lineCount == 0 {
		return 0
	}
	return 1
}

func validateMutablePath(value string) error {
	if err := validateRelativePath(value); err != nil {
		return fmt.Errorf("workspace mutation path: %w", err)
	}
	for _, component := range strings.Split(strings.ToLower(value), "/") {
		switch component {
		case ".git", ".omni", "node_modules", "vendor":
			return fmt.Errorf("workspace mutation path %q enters protected authority", value)
		}
	}
	if sensitivePath(value) {
		return fmt.Errorf("workspace mutation path %q enters protected sensitive authority", value)
	}
	return nil
}
