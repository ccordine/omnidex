package worker

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"path"
	"strings"

	"github.com/gryph/omnidex/internal/codingagent"
	"github.com/gryph/omnidex/internal/projectroot"
	"github.com/gryph/omnidex/internal/workspace"
)

type codingAgentEnvironment struct {
	runtime        *nativeRuntimeV3
	root, identity string
}

func (environment *codingAgentEnvironment) Attest(ctx context.Context) error {
	if err := environment.runtime.svc.repo.RequireActiveStepAttempt(ctx, environment.runtime.claim.Authority); err != nil {
		return err
	}
	identity, err := projectroot.DirectoryIdentity(environment.root)
	if err != nil || identity != environment.identity {
		return fmt.Errorf("native agent workspace identity changed: %w", err)
	}
	return environment.runtime.workspaceFence.Reattest(environment.root)
}

func (environment *codingAgentEnvironment) ReadFiles(ctx context.Context) ([]string, error) {
	files := []string{}
	directories := []string{"."}
	for len(directories) > 0 {
		directory := directories[0]
		directories = directories[1:]
		after := ""
		for {
			page, err := environment.runtime.workspaceFence.ReadDirectory(ctx, directory, after, workspace.MaxDirectoryPageEntries)
			if err != nil {
				return nil, err
			}
			for _, entry := range page.Entries {
				name := path.Base(entry.Path)
				if name == ".git" || name == "node_modules" || name == ".env" || strings.HasPrefix(name, ".env.") || name == ".codex" || name == ".agent-cache" || name == "bin" {
					continue
				}
				switch entry.Kind {
				case workspace.EntryDirectory:
					directories = append(directories, entry.Path)
				case workspace.EntryFile:
					files = append(files, entry.Path)
				}
			}
			if !page.HasMore {
				environment.runtime.observeWorkspaceRead("workspace_directory_read", "list", directory)
				break
			}
			if len(page.Entries) == 0 {
				return nil, fmt.Errorf("workspace directory pagination made no progress")
			}
			after = path.Base(page.Entries[len(page.Entries)-1].Path)
		}
	}
	return files, nil
}

func (environment *codingAgentEnvironment) ReadTaskFile(ctx context.Context, file string) (string, error) {
	if err := environment.Attest(ctx); err != nil {
		return "", err
	}
	if err := codingagent.ValidatePath(file); err != nil {
		return "", err
	}
	observed, err := environment.runtime.readObservedWorkspaceFile(ctx, file)
	if err != nil {
		return "", err
	}
	return string(observed.Content), environment.Attest(ctx)
}

func (environment *codingAgentEnvironment) PrepareFiles(ctx context.Context, task codingagent.Task) error {
	if err := environment.Attest(ctx); err != nil {
		return err
	}
	for _, file := range append(append(append([]string{}, task.ReadFiles...), task.WriteFiles...), task.DeleteFiles...) {
		if err := codingagent.ValidatePath(file); err != nil {
			return err
		}
	}
	for _, file := range task.WriteFiles {
		entry, err := environment.runtime.workspaceFence.Stat(ctx, file)
		if err == nil {
			if entry.Kind != workspace.EntryFile {
				return fmt.Errorf("agent write scope %s is not a file", file)
			}
			continue
		}
		if !errors.Is(err, fs.ErrNotExist) {
			return err
		}
		prepared, err := environment.runtime.workspaceFence.Prepare(ctx, []workspace.DesiredFile{{Path: file, Present: true, Content: []byte{}, Mode: 0o644, CreateOnly: true}}, nil)
		if err != nil {
			return err
		}
		result, err := prepared.ApplyVerified(ctx, nil)
		if err != nil {
			return err
		}
		if err := environment.recordWrittenFiles(ctx, result); err != nil {
			return err
		}
	}
	for _, file := range task.ReadFiles {
		entry, err := environment.runtime.workspaceFence.Stat(ctx, file)
		if err != nil {
			return err
		}
		if entry.Kind != workspace.EntryFile {
			return fmt.Errorf("agent read scope %s is not a file", file)
		}
	}
	for _, file := range task.DeleteFiles {
		entry, err := environment.runtime.workspaceFence.Stat(ctx, file)
		if err != nil {
			return err
		}
		if entry.Kind != workspace.EntryFile {
			return fmt.Errorf("agent delete scope %s is not a file", file)
		}
	}
	return environment.Attest(ctx)
}

func (environment *codingAgentEnvironment) WriteFile(ctx context.Context, edit codingagent.FileWrite) error {
	if err := environment.Attest(ctx); err != nil {
		return err
	}
	entry, err := environment.runtime.workspaceFence.Stat(ctx, edit.Path)
	if errors.Is(err, fs.ErrNotExist) {
		if edit.ExpectedContent != "" {
			return fmt.Errorf("missing source does not match the agent's expected content")
		}
		prepared, err := environment.runtime.workspaceFence.Prepare(ctx, []workspace.DesiredFile{{Path: edit.Path, Present: true, Content: []byte(edit.Content), Mode: 0o644, CreateOnly: true}}, nil)
		if err != nil {
			return err
		}
		result, err := prepared.ApplyVerified(ctx, nil)
		if err != nil {
			return err
		}
		return environment.recordWrittenFiles(ctx, result)
	}
	if err != nil {
		return err
	}
	if entry.Kind != workspace.EntryFile {
		return fmt.Errorf("agent edit requires a pre-attested regular file")
	}
	file, err := environment.runtime.readObservedWorkspaceFile(ctx, edit.Path)
	if err != nil {
		return err
	}
	if string(file.Content) != edit.ExpectedContent {
		return fmt.Errorf("source changed since the agent read it; inspect the file before editing")
	}
	prepared, err := environment.runtime.workspaceFence.Prepare(ctx, []workspace.DesiredFile{{Path: edit.Path, Present: true, Content: []byte(edit.Content), Mode: entry.Mode}}, []workspace.File{file})
	if err != nil {
		return err
	}
	result, err := prepared.ApplyVerified(ctx, nil)
	if err != nil {
		return err
	}
	return environment.recordWrittenFiles(ctx, result)
}

func (environment *codingAgentEnvironment) DeleteFile(ctx context.Context, deletion codingagent.FileDelete) error {
	if err := environment.Attest(ctx); err != nil {
		return err
	}
	entry, err := environment.runtime.workspaceFence.Stat(ctx, deletion.Path)
	if err != nil {
		return err
	}
	if entry.Kind != workspace.EntryFile {
		return fmt.Errorf("agent deletion requires a pre-attested regular file")
	}
	file, err := environment.runtime.readObservedWorkspaceFile(ctx, deletion.Path)
	if err != nil {
		return err
	}
	if string(file.Content) != deletion.ExpectedContent {
		return fmt.Errorf("source changed since the agent read it; inspect the file before deleting")
	}
	prepared, err := environment.runtime.workspaceFence.Prepare(ctx, []workspace.DesiredFile{{Path: deletion.Path, Present: false}}, []workspace.File{file})
	if err != nil {
		return err
	}
	result, err := prepared.ApplyVerified(ctx, nil)
	if err != nil {
		return err
	}
	return environment.recordWrittenFiles(ctx, result)
}

func (environment *codingAgentEnvironment) recordWrittenFiles(ctx context.Context, result workspace.ReconciliationResult) error {
	if len(result.Changes) == 0 {
		return nil
	}
	files := make([]string, len(result.Changes))
	for index, change := range result.Changes {
		files[index] = change.Path
	}
	_, err := environment.runtime.svc.repo.CodingAgentEvidence(environment.runtime.claim.Authority).Append(ctx,
		codingagent.Entry{Generation: environment.runtime.claim.Authority.Generation, Phase: "files_written", WrittenFiles: files})
	return err
}
