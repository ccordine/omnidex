package changeapply

import (
	"bytes"
	"fmt"
	"strings"
)

type changedLineRange struct {
	start        int
	end          int
	replacements []targetReplacement
}

func buildUnifiedPatch(mutations []fileMutation) (string, error) {
	if len(mutations) == 0 {
		return "", fmt.Errorf("repository change staging requires at least one file mutation")
	}
	var output strings.Builder
	for _, mutation := range mutations {
		if !mutation.sourcePresent {
			writeCreatedFilePatch(&output, mutation)
			continue
		}
		if !mutation.desiredPresent {
			writeDeletedFilePatch(&output, mutation)
			continue
		}
		output.WriteString("diff --git a/")
		output.WriteString(mutation.file.Path)
		output.WriteString(" b/")
		output.WriteString(mutation.file.Path)
		output.WriteString("\n--- a/")
		output.WriteString(mutation.file.Path)
		output.WriteString("\n+++ b/")
		output.WriteString(mutation.file.Path)
		output.WriteByte('\n')
		ranges := changedLineRanges(mutation.original, mutation.replacements)
		lineDelta := 0
		for _, current := range ranges {
			oldSegment := mutation.original[current.start:current.end]
			newSegment := append([]byte(nil), oldSegment...)
			for index := len(current.replacements) - 1; index >= 0; index-- {
				target := current.replacements[index]
				newSegment = replaceExactBytes(
					newSegment,
					int(target.start)-current.start,
					int(target.end)-current.start,
					target.declaration,
				)
			}
			oldLines := splitLFSegment(oldSegment)
			newLines := splitLFSegment(newSegment)
			oldStart := bytes.Count(mutation.original[:current.start], []byte{'\n'}) + 1
			newStart := oldStart + lineDelta
			fmt.Fprintf(&output, "@@ -%d,%d +%d,%d @@\n", oldStart, len(oldLines), newStart, len(newLines))
			for _, line := range oldLines {
				output.WriteByte('-')
				output.WriteString(line)
				output.WriteByte('\n')
			}
			for _, line := range newLines {
				output.WriteByte('+')
				output.WriteString(line)
				output.WriteByte('\n')
			}
			lineDelta += len(newLines) - len(oldLines)
		}
	}
	return output.String(), nil
}

func writeCreatedFilePatch(output *strings.Builder, mutation fileMutation) {
	fmt.Fprintf(output, "diff --git a/%s b/%s\n--- /dev/null\n+++ b/%s\n", mutation.file.Path, mutation.file.Path, mutation.file.Path)
	lines := splitLFSegment(mutation.next)
	fmt.Fprintf(output, "@@ -0,0 +1,%d @@\n", len(lines))
	for _, line := range lines {
		output.WriteByte('+')
		output.WriteString(line)
		output.WriteByte('\n')
	}
}

func writeDeletedFilePatch(output *strings.Builder, mutation fileMutation) {
	fmt.Fprintf(output, "diff --git a/%s b/%s\n--- a/%s\n+++ /dev/null\n", mutation.file.Path, mutation.file.Path, mutation.file.Path)
	lines := splitLFSegment(mutation.original)
	fmt.Fprintf(output, "@@ -1,%d +0,0 @@\n", len(lines))
	for _, line := range lines {
		output.WriteByte('-')
		output.WriteString(line)
		output.WriteByte('\n')
	}
}

func changedLineRanges(content []byte, replacements []targetReplacement) []changedLineRange {
	ranges := make([]changedLineRange, 0, len(replacements))
	for _, replacement := range replacements {
		start := lineStart(content, int(replacement.start))
		end := lineEnd(content, int(replacement.end))
		if len(ranges) > 0 && start < ranges[len(ranges)-1].end {
			last := &ranges[len(ranges)-1]
			if end > last.end {
				last.end = end
			}
			last.replacements = append(last.replacements, replacement)
			continue
		}
		ranges = append(ranges, changedLineRange{
			start: start, end: end, replacements: []targetReplacement{replacement},
		})
	}
	return ranges
}

func lineStart(content []byte, offset int) int {
	if offset == 0 {
		return 0
	}
	if index := bytes.LastIndexByte(content[:offset], '\n'); index >= 0 {
		return index + 1
	}
	return 0
}

func lineEnd(content []byte, offset int) int {
	if offset > 0 && content[offset-1] == '\n' {
		return offset
	}
	if index := bytes.IndexByte(content[offset:], '\n'); index >= 0 {
		return offset + index + 1
	}
	return len(content)
}

func splitLFSegment(content []byte) []string {
	trimmed := bytes.TrimSuffix(content, []byte{'\n'})
	if len(trimmed) == 0 && len(content) == 0 {
		return nil
	}
	return strings.Split(string(trimmed), "\n")
}
