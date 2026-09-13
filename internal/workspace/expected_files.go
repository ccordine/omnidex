package workspace

import (
	"bytes"
	"context"
	"fmt"
)

func (file File) Validate() error {
	if err := file.Entry.Validate(file.Entry.Path); err != nil {
		return err
	}
	if file.Entry.Kind != EntryFile || file.Entry.Path == "." ||
		file.Entry.Size != int64(len(file.Content)) || len(file.Content) > MaxReconciliationFileBytes {
		return fmt.Errorf("workspace file differs from its exact regular-file data")
	}
	return nil
}

func cloneExpectedFiles(files []File) ([]File, error) {
	if len(files) > MaxReconciliationFiles {
		return nil, fmt.Errorf("too many expected workspace files")
	}
	result := make([]File, len(files))
	seen := make(map[string]bool, len(files))
	total := 0
	for index, file := range files {
		if err := file.Validate(); err != nil {
			return nil, err
		}
		if seen[file.Entry.Path] {
			return nil, fmt.Errorf("expected workspace file %q is repeated", file.Entry.Path)
		}
		seen[file.Entry.Path] = true
		total += len(file.Content)
		if total > MaxReconciliationTotalBytes {
			return nil, fmt.Errorf("expected workspace content exceeds its byte bound")
		}
		result[index] = File{Entry: file.Entry, Content: bytes.Clone(file.Content)}
	}
	return result, nil
}

func requireExpectedFiles(ctx context.Context, root RootReader, expected []File) error {
	for _, file := range expected {
		actual, err := ReadFile(ctx, root, file.Entry.Path)
		if err != nil {
			return fmt.Errorf("read expected workspace file %q: %w", file.Entry.Path, err)
		}
		if actual.Entry != file.Entry || !bytes.Equal(actual.Content, file.Content) {
			return fmt.Errorf("previously observed workspace file %q changed before reconciliation", file.Entry.Path)
		}
	}
	return nil
}
