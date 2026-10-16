package codingagent

import (
	"context"
	"fmt"
)

type taskFileWriter struct {
	environment Environment
	reads       []string
	files       []string
	deletes     []string
}

func (writer taskFileWriter) ReadFile(ctx context.Context, file string) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	if err := ValidatePath(file); err != nil {
		return "", err
	}
	if !contains(writer.reads, file) {
		return "", fmt.Errorf("task packet does not authorize reading %s", file)
	}
	if err := writer.environment.Attest(ctx); err != nil {
		return "", err
	}
	return writer.environment.ReadTaskFile(ctx, file)
}

func (writer taskFileWriter) DeleteFile(ctx context.Context, deletion FileDelete) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := ValidatePath(deletion.Path); err != nil {
		return err
	}
	if !contains(writer.deletes, deletion.Path) {
		return fmt.Errorf("task packet does not authorize deleting %s", deletion.Path)
	}
	if err := writer.environment.Attest(ctx); err != nil {
		return err
	}
	return writer.environment.DeleteFile(ctx, deletion)
}

func (writer taskFileWriter) WriteFile(ctx context.Context, edit FileWrite) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := ValidatePath(edit.Path); err != nil {
		return err
	}
	if !contains(writer.files, edit.Path) {
		return fmt.Errorf("task packet does not authorize writing %s", edit.Path)
	}
	if err := writer.environment.Attest(ctx); err != nil {
		return err
	}
	return writer.environment.WriteFile(ctx, edit)
}
