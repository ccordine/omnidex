package workspace

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
)

func ReadEntry(ctx context.Context, root RootReader, relative string) (Entry, error) {
	if err := validateRead(ctx, root, relative); err != nil {
		return Entry{}, err
	}
	info, err := root.Lstat(relative)
	if err != nil {
		return Entry{}, err
	}
	entry := entryFromInfo(relative, info)
	return entry, entry.Validate(relative)
}

func validateRead(ctx context.Context, root RootReader, relative string) error {
	if ctx == nil || root == nil {
		return fmt.Errorf("workspace read requires context and root authority")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := validateReadPath(relative); err != nil {
		return err
	}
	parts := strings.Split(relative, "/")
	for index := 1; index < len(parts); index++ {
		parent := strings.Join(parts[:index], "/")
		info, err := root.Lstat(parent)
		if err != nil {
			return err
		}
		if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("workspace read parent %q is not an exact directory", parent)
		}
	}
	return nil
}

func ReadFile(ctx context.Context, root RootReader, relative string) (result File, resultErr error) {
	if err := validateRead(ctx, root, relative); err != nil {
		return result, err
	}
	before, err := root.Lstat(relative)
	if err != nil {
		return result, err
	}
	if !before.Mode().IsRegular() {
		return result, fmt.Errorf("workspace read path %q is not a regular file", relative)
	}
	if before.Size() < 0 || before.Size() > MaxReconciliationFileBytes {
		return result, fmt.Errorf("workspace file %q exceeds the %d-byte read limit", relative, MaxReconciliationFileBytes)
	}
	if before.Size() == 0 {
		// A regular file of exactly zero bytes has no unread semantic or byte
		// content. Recheck that exact observation without requiring read access.
		if err := validateRead(ctx, root, relative); err != nil {
			return result, err
		}
		after, err := root.Lstat(relative)
		if err != nil {
			return result, err
		}
		if !sameReadFileObservation(before, after) {
			return result, fmt.Errorf("empty workspace file %q changed while observed", relative)
		}
		return File{Entry: entryFromInfo(relative, after), Content: []byte{}}, nil
	}
	file, err := root.Open(relative)
	if err != nil {
		return result, err
	}
	defer func() { resultErr = errors.Join(resultErr, file.Close()) }()
	opened, err := file.Stat()
	if err != nil || !opened.Mode().IsRegular() || !os.SameFile(before, opened) {
		return result, errors.Join(fmt.Errorf("workspace file changed before reading"), err)
	}
	content, err := io.ReadAll(io.LimitReader(file, MaxReconciliationFileBytes+1))
	if err != nil {
		return result, err
	}
	if err := validateRead(ctx, root, relative); err != nil {
		return result, err
	}
	after, err := root.Lstat(relative)
	if err != nil {
		return result, err
	}
	current, err := file.Stat()
	if err != nil {
		return result, err
	}
	if !sameReadFileObservation(before, after) || !sameReadFileObservation(before, current) || before.Size() != int64(len(content)) {
		return result, fmt.Errorf("workspace file %q changed while its bytes were read", relative)
	}
	return File{Entry: entryFromInfo(relative, after), Content: content}, nil
}

func sameReadFileObservation(before, after os.FileInfo) bool {
	return os.SameFile(before, after) && before.Mode() == after.Mode() && before.Size() == after.Size() && before.ModTime().Equal(after.ModTime())
}
