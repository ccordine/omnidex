package changeapply

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"unicode/utf8"

	repositoryfacts "github.com/gryph/omnidex/internal/repository"
)

func planMutations(
	workspace string,
	snapshot repositoryfacts.Snapshot,
	replacements []targetReplacement,
) ([]fileMutation, error) {
	files := make(map[string]repositoryfacts.File, len(snapshot.Files))
	for _, file := range snapshot.Files {
		files[file.ID] = file
	}
	grouped := make(map[string][]targetReplacement)
	for _, replacement := range replacements {
		grouped[replacement.fileID] = append(grouped[replacement.fileID], replacement)
	}
	mutations := make([]fileMutation, 0, len(grouped))
	for fileID, targets := range grouped {
		file := files[fileID]
		if file.Size > maxTargetFileBytes {
			return nil, fmt.Errorf("repository change target file exceeds %d bytes", maxTargetFileBytes)
		}
		absolute := filepath.Join(workspace, filepath.FromSlash(file.Path))
		original, err := os.ReadFile(absolute)
		if err != nil {
			return nil, fmt.Errorf("read staged repository target %q: %w", file.ID, err)
		}
		if int64(len(original)) != file.Size || digest(original) != file.SHA256 {
			return nil, fmt.Errorf("staged repository target %q differs from its exact file authority", file.ID)
		}
		if err := validatePatchableSource(file.ID, original); err != nil {
			return nil, err
		}
		sort.Slice(targets, func(left, right int) bool {
			if targets[left].start == targets[right].start {
				return targets[left].end < targets[right].end
			}
			return targets[left].start < targets[right].start
		})
		for _, target := range targets {
			current := original[target.start:target.end]
			if digest(current) != target.expected {
				return nil, fmt.Errorf("repository declaration %q hash differs from its change contract", target.symbolID)
			}
			if !utf8.Valid(current) || bytes.IndexByte(current, 0) >= 0 {
				return nil, fmt.Errorf("repository declaration %q is not valid UTF-8, NUL-free source", target.symbolID)
			}
			if bytes.Equal(current, target.declaration) {
				return nil, fmt.Errorf("repository change candidate for symbol %q is unchanged", target.symbolID)
			}
		}
		next := append([]byte(nil), original...)
		for index := len(targets) - 1; index >= 0; index-- {
			target := targets[index]
			next = replaceExactBytes(next, int(target.start), int(target.end), target.declaration)
		}
		mutations = append(mutations, fileMutation{
			file: file, original: original, next: next, replacements: targets,
		})
	}
	sort.Slice(mutations, func(left, right int) bool {
		return mutations[left].file.Path < mutations[right].file.Path
	})
	return mutations, nil
}

func validatePatchableSource(fileID string, content []byte) error {
	if len(content) == 0 || !utf8.Valid(content) || bytes.IndexByte(content, 0) >= 0 {
		return fmt.Errorf("repository change target file %q is not supported UTF-8, NUL-free source", fileID)
	}
	if bytes.IndexByte(content, '\r') >= 0 {
		return fmt.Errorf("repository change target file %q contains unsupported carriage-return bytes", fileID)
	}
	if !strings.HasSuffix(string(content), "\n") {
		return fmt.Errorf("repository change target file %q must end with one LF for exact patch staging", fileID)
	}
	return nil
}

func replaceExactBytes(content []byte, start, end int, replacement []byte) []byte {
	next := make([]byte, 0, len(content)-(end-start)+len(replacement))
	next = append(next, content[:start]...)
	next = append(next, replacement...)
	next = append(next, content[end:]...)
	return next
}

func verifyStagedMutations(workspace string, mutations []fileMutation) error {
	for _, mutation := range mutations {
		absolute := filepath.Join(workspace, filepath.FromSlash(mutation.file.Path))
		actual, err := os.ReadFile(absolute)
		if err != nil {
			return fmt.Errorf("read applied staged target %q: %w", mutation.file.ID, err)
		}
		if !bytes.Equal(actual, mutation.next) {
			return fmt.Errorf("staged patch result for target file %q differs from the planned exact bytes", mutation.file.ID)
		}
		info, err := os.Stat(absolute)
		if err != nil || !info.Mode().IsRegular() || info.Mode().Perm() != os.FileMode(mutation.file.Mode) {
			return fmt.Errorf("staged patch changed target file %q kind or mode", mutation.file.ID)
		}
	}
	return nil
}
