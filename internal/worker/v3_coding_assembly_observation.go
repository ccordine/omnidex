package worker

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"

	"github.com/gryph/omnidex/internal/workspace"
)

func validateDirectCodingAssembly(
	ctx context.Context,
	reader workspace.Reader,
	assembly directCodingAssembly,
) error {
	if reader == nil {
		return fmt.Errorf("assembly observation requires acquired workspace access")
	}
	rootInfo, err := reader.Stat(ctx, ".")
	if err != nil || rootInfo.Kind != workspace.EntryDirectory {
		return fmt.Errorf("authoritative workspace root is not one exact directory")
	}
	declared := make(map[string]struct{}, len(assembly.Files))
	for _, file := range assembly.Files {
		declared[file.Path] = struct{}{}
		actual, err := reader.ReadFile(ctx, file.Path)
		if err != nil {
			return fmt.Errorf("read authoritative assembly file %s: %w", file.Path, err)
		}
		if !bytes.Equal(actual.Content, file.Content) || actual.Entry.Mode != file.Mode {
			return fmt.Errorf("authoritative assembly file %s differs from in-memory authority", file.Path)
		}
	}
	for _, required := range assembly.RequiredPaths {
		if _, generated := declared[required]; generated {
			continue
		}
		info, err := reader.Stat(ctx, required)
		if err != nil || info.Kind != workspace.EntryFile {
			return fmt.Errorf("required authoritative file %s is absent or non-regular", required)
		}
	}
	for _, deleted := range assembly.DeletePaths {
		_, err := reader.Stat(ctx, deleted)
		if !errors.Is(err, os.ErrNotExist) {
			if err != nil {
				return fmt.Errorf("inspect authoritative deletion %s: %w", deleted, err)
			}
			return fmt.Errorf("authoritative deletion %s is still present", deleted)
		}
	}
	for _, file := range assembly.Files {
		if file.MoveFrom == "" {
			continue
		}
		if _, retained := declared[file.MoveFrom]; retained {
			continue
		}
		_, err := reader.Stat(ctx, file.MoveFrom)
		if !errors.Is(err, os.ErrNotExist) {
			if err != nil {
				return fmt.Errorf("inspect authoritative move source %s: %w", file.MoveFrom, err)
			}
			return fmt.Errorf("authoritative move source %s is still present", file.MoveFrom)
		}
	}
	return nil
}
