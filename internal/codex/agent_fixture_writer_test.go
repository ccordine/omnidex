package codex

import (
	"context"
	"fmt"
	"github.com/gryph/omnidex/internal/codingagent"
	"os"
	"path/filepath"
)

type agentFixtureWriter struct {
	scope  codingagent.Assignment
	writes int
}

func (writer *agentFixtureWriter) WriteFile(ctx context.Context, write codingagent.FileWrite) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := codingagent.ValidatePath(write.Path); err != nil {
		return err
	}
	allowed := false
	for _, file := range writer.scope.WriteFiles {
		if file == write.Path {
			allowed = true
		}
	}
	if !allowed || writer.scope.Role != codingagent.Execute {
		return fmt.Errorf("fixture writer denied an ungranted source file")
	}
	path := filepath.Join(writer.scope.Root, filepath.FromSlash(write.Path))
	resolved, err := filepath.EvalSymlinks(path)
	if err != nil || resolved != path {
		return fmt.Errorf("fixture writer requires exact physical source")
	}
	current, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	if string(current) != write.ExpectedContent {
		return fmt.Errorf("fixture writer rejected stale source bytes")
	}
	if err := os.WriteFile(path, []byte(write.Content), 0o600); err != nil {
		return err
	}
	writer.writes++
	return nil
}
