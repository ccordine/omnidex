package changeapply

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"go/format"
	"go/parser"
	"go/token"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"strings"
	"unicode/utf8"

	repositoryfacts "github.com/gryph/omnidex/internal/repository"
)

const maxCreatedGoSourceBytes = 1024 * 1024

func deriveAbsentToSourceMutation(
	ctx context.Context,
	input FileStateInput,
	files map[string]repositoryfacts.File,
	desired DesiredFileState,
) (fileMutation, error) {
	if !exactSourceFileEmpty(desired.Source) {
		return fileMutation{}, fmt.Errorf("repository desired present state for %q requires exact absent source authority", desired.Path)
	}
	if _, exists := files[desired.Path]; exists {
		return fileMutation{}, fmt.Errorf("repository desired present path %q already exists and requires ordinary bounded modification", desired.Path)
	}
	if len(desired.RemovedSymbolIDs) != 0 {
		return fileMutation{}, fmt.Errorf("repository desired present state cannot remove source symbols")
	}
	if err := validateNewGoSource(input, desired); err != nil {
		return fileMutation{}, err
	}
	if err := requirePhysicalAbsence(input.Snapshot.Root, desired.Path); err != nil {
		return fileMutation{}, err
	}
	if err := rejectIgnoredTarget(ctx, input.Snapshot.Root, desired.Path, false); err != nil {
		return fileMutation{}, err
	}
	fileID, err := repositoryfacts.FileIDForAbsentPath(input.Snapshot, desired.Path)
	if err != nil {
		return fileMutation{}, err
	}
	file := repositoryfacts.File{
		ID: fileID, Path: desired.Path, Kind: repositoryfacts.EntryRegular,
		SHA256: digest(desired.Content), Size: int64(len(desired.Content)), Mode: desired.Mode,
		Language: "go",
	}
	return fileMutation{
		file: file, next: append([]byte(nil), desired.Content...),
		sourcePresent: false, desiredPresent: true,
	}, nil
}

func validateNewGoSource(input FileStateInput, desired DesiredFileState) error {
	if err := validateDesiredGoPath(desired.Path); err != nil {
		return err
	}
	if desired.Mode != 0o644 {
		return fmt.Errorf("repository created Go source %q requires code-owned mode 0644", desired.Path)
	}
	if len(desired.Content) == 0 || len(desired.Content) > maxCreatedGoSourceBytes ||
		!utf8.Valid(desired.Content) || bytes.IndexByte(desired.Content, 0) >= 0 {
		return fmt.Errorf("repository created Go source %q has invalid or oversized exact bytes", desired.Path)
	}
	if bytes.IndexByte(desired.Content, '\r') >= 0 || desired.Content[len(desired.Content)-1] != '\n' {
		return fmt.Errorf("repository created Go source %q requires canonical LF-only bytes", desired.Path)
	}
	canonical, err := format.Source(desired.Content)
	if err != nil {
		return fmt.Errorf("repository created Go source %q does not parse: %w", desired.Path, err)
	}
	if !bytes.Equal(canonical, desired.Content) {
		return fmt.Errorf("repository created Go source %q is not canonically formatted", desired.Path)
	}
	parsed, err := parser.ParseFile(token.NewFileSet(), desired.Path, desired.Content, parser.PackageClauseOnly)
	if err != nil || parsed.Name == nil {
		return fmt.Errorf("repository created Go source %q has no exact package declaration", desired.Path)
	}
	packageName, directory, err := exactGoPackagePlacement(input.Snapshot, input.Analysis, desired.PackageArtifactID)
	if err != nil {
		return err
	}
	if parsed.Name.Name != packageName || path.Dir(desired.Path) != directory {
		return fmt.Errorf("repository created Go source %q differs from its code-owned package placement", desired.Path)
	}
	return nil
}

func deriveSourceToAbsentMutation(
	ctx context.Context,
	input FileStateInput,
	files map[string]repositoryfacts.File,
	desired DesiredFileState,
) (fileMutation, error) {
	if len(desired.Content) != 0 || desired.Mode != 0 || desired.PackageArtifactID != "" {
		return fileMutation{}, fmt.Errorf("repository desired absent state for %q cannot contain post-state source authority", desired.Path)
	}
	file, exists := files[desired.Path]
	if !exists {
		return fileMutation{}, fmt.Errorf("repository desired absent path %q is not an exact indexed source member", desired.Path)
	}
	if exactSourceFileEmpty(desired.Source) || desired.Source.FileID != file.ID ||
		desired.Source.SHA256 != file.SHA256 || desired.Source.Size != file.Size || desired.Source.Mode != file.Mode {
		return fileMutation{}, fmt.Errorf("repository deletion for %q differs from its exact source authority", desired.Path)
	}
	if file.Kind != repositoryfacts.EntryRegular || file.Language != "go" || file.Generated || excludedFileStatePath(file.Path) {
		return fileMutation{}, fmt.Errorf("repository deletion target %q is generated, protected, vendored, or unsupported", desired.Path)
	}
	if err := requireTrackedSource(ctx, input.Snapshot.Root, desired.Path); err != nil {
		return fileMutation{}, err
	}
	if err := rejectIgnoredTarget(ctx, input.Snapshot.Root, desired.Path, true); err != nil {
		return fileMutation{}, err
	}
	if err := rejectCanonicalGeneratedDeletion(input.Snapshot.Root, file); err != nil {
		return fileMutation{}, err
	}
	if err := validateRemovedFileSymbols(input.Analysis, file, desired.RemovedSymbolIDs); err != nil {
		return fileMutation{}, err
	}
	return fileMutation{file: file, sourcePresent: true, desiredPresent: false}, nil
}

func rejectCanonicalGeneratedDeletion(root string, file repositoryfacts.File) error {
	generated, err := canonicalGeneratedDeletion(root, file)
	if err != nil {
		return err
	}
	if generated {
		return fmt.Errorf("repository deletion target %q is generated and cannot be mutated", file.ID)
	}
	return nil
}

func canonicalGeneratedDeletion(root string, file repositoryfacts.File) (bool, error) {
	raw, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(file.Path)))
	if err != nil {
		return false, fmt.Errorf("read repository deletion target %q: %w", file.ID, err)
	}
	if int64(len(raw)) != file.Size || digest(raw) != file.SHA256 {
		return false, fmt.Errorf("repository deletion target %q changed after exact snapshot validation", file.ID)
	}
	return repositoryfacts.GeneratedGoSource(raw), nil
}

func requireTrackedSource(ctx context.Context, root, relative string) error {
	tracked, err := trackedRepositorySource(ctx, root, relative)
	if err != nil {
		return err
	}
	if tracked {
		return nil
	}
	return fmt.Errorf(
		"repository deletion target %q is untracked and has no durable absence authority",
		relative,
	)
}

func trackedRepositorySource(ctx context.Context, root, relative string) (bool, error) {
	command := exec.CommandContext(
		ctx, "git", "-C", root, "ls-files", "--error-unmatch", "--", relative,
	)
	err := command.Run()
	if err == nil {
		return true, nil
	}
	var exit *exec.ExitError
	if errors.As(err, &exit) && exit.ExitCode() == 1 {
		return false, nil
	}
	return false, fmt.Errorf("prove repository deletion target %q is tracked: %w", relative, err)
}

func validateDesiredGoPath(value string) error {
	if value == "" || value != strings.TrimSpace(value) || filepath.IsAbs(value) ||
		strings.ContainsAny(value, "\\\x00\r\n") || path.Clean(value) != value ||
		value == "." || value == ".." || strings.HasPrefix(value, "../") || path.Ext(value) != ".go" {
		return fmt.Errorf("repository desired path %q is not one normalized Go source path", value)
	}
	if excludedFileStatePath(value) {
		return fmt.Errorf("repository desired path %q is generated, vendored, or protected", value)
	}
	return nil
}

func excludedFileStatePath(value string) bool {
	if protectedRepositoryPath(value) {
		return true
	}
	lower := strings.ToLower(value)
	for _, part := range strings.Split(lower, "/") {
		if part == "vendor" || part == "node_modules" || part == "generated" {
			return true
		}
	}
	base := path.Base(lower)
	return strings.HasSuffix(base, ".generated.go") || strings.HasSuffix(base, ".gen.go")
}

func exactSourceFileEmpty(source ExactSourceFile) bool {
	return source.FileID == "" && source.SHA256 == "" && source.Size == 0 && source.Mode == 0
}

func requirePhysicalAbsence(root, relative string) error {
	absolute := filepath.Join(root, filepath.FromSlash(relative))
	if _, err := os.Lstat(absolute); err == nil {
		return fmt.Errorf("repository desired present path %q already exists outside its indexed authority", relative)
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("inspect repository desired path %q: %w", relative, err)
	}
	return nil
}

func rejectIgnoredTarget(ctx context.Context, root, relative string, ignoreIndex bool) error {
	ignored, err := ignoredRepositoryTarget(ctx, root, relative, ignoreIndex)
	if err != nil {
		return err
	}
	if ignored {
		return fmt.Errorf("repository desired path %q is ignored and cannot be mutated", relative)
	}
	return nil
}

func ignoredRepositoryTarget(
	ctx context.Context,
	root string,
	relative string,
	ignoreIndex bool,
) (bool, error) {
	args := []string{"-C", root, "check-ignore", "--quiet"}
	if ignoreIndex {
		args = append(args, "--no-index")
	}
	args = append(args, "--", relative)
	command := exec.CommandContext(ctx, "git", args...)
	err := command.Run()
	if err == nil {
		return true, nil
	}
	var exit *exec.ExitError
	if errors.As(err, &exit) && exit.ExitCode() == 1 {
		return false, nil
	}
	return false, fmt.Errorf("check repository ignore authority for %q: %w", relative, err)
}
