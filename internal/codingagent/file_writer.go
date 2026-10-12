package codingagent

import (
	"context"
	"fmt"
)

type taskFileWriter struct {
	environment Environment
	files       []string
}

func (writer taskFileWriter) WriteFile(ctx context.Context, edit FileWrite) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := ValidatePath(edit.Path); err != nil {
		return err
	}
	if !contains(writer.files, edit.Path) {
		return fmt.Errorf("file write is outside the frozen task scope")
	}
	if err := writer.environment.Attest(ctx); err != nil {
		return err
	}
	return writer.environment.WriteFile(ctx, edit)
}
