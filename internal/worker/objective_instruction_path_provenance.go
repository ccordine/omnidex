package worker

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/gryph/omnidex/internal/assemblyline"
	"github.com/gryph/omnidex/internal/modelcontext"
)

type objectiveBasenameOwner struct {
	path      string
	ambiguous bool
}

// objectiveInstructionPathProvenance derives only the filesystem names needed
// to keep one instruction path-blind. It deliberately does not hash content,
// invoke Git, persist a repository snapshot, or build a semantic index. Those
// operations belong to the first workflow that actually consumes repository
// authority.
func objectiveInstructionPathProvenance(
	ctx context.Context,
	root string,
	instruction string,
) (assemblyline.ArtifactIdentityProvenance, error) {
	if ctx == nil {
		return assemblyline.ArtifactIdentityProvenance{}, fmt.Errorf(
			"objective instruction path provenance requires a context",
		)
	}
	root = filepath.Clean(root)
	if !filepath.IsAbs(root) {
		return assemblyline.ArtifactIdentityProvenance{}, fmt.Errorf(
			"objective instruction path provenance root must be absolute",
		)
	}
	before, err := os.Lstat(root)
	if err != nil || !before.IsDir() || before.Mode()&os.ModeSymlink != 0 {
		return assemblyline.ArtifactIdentityProvenance{}, fmt.Errorf(
			"objective instruction path provenance root %q is not one exact directory", root,
		)
	}

	selected := make(map[string]struct{})
	basenames := make(map[string]objectiveBasenameOwner)
	err = filepath.WalkDir(root, func(absolute string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			if absolute == root {
				return fmt.Errorf("walk objective instruction path %q: %w", absolute, walkErr)
			}
			if entry != nil && entry.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		if absolute == root {
			return nil
		}
		if entry.IsDir() && (entry.Name() == ".git" || entry.Name() == ".omni") {
			return filepath.SkipDir
		}
		if entry.IsDir() || !entry.Type().IsRegular() && entry.Type()&os.ModeSymlink == 0 {
			return nil
		}
		relative, err := filepath.Rel(root, absolute)
		if err != nil {
			return fmt.Errorf("derive objective instruction relative path: %w", err)
		}
		relative = filepath.ToSlash(relative)
		base := filepath.Base(absolute)
		if strings.Contains(instruction, relative) {
			selected[relative] = struct{}{}
		}
		if !strings.Contains(instruction, base) {
			return nil
		}
		owner, exists := basenames[base]
		if !exists {
			basenames[base] = objectiveBasenameOwner{path: relative}
			return nil
		}
		if owner.path != relative {
			owner.ambiguous = true
			basenames[base] = owner
		}
		return nil
	})
	if err != nil {
		return assemblyline.ArtifactIdentityProvenance{}, err
	}
	after, err := os.Lstat(root)
	if err != nil || !after.IsDir() || after.Mode()&os.ModeSymlink != 0 || !os.SameFile(before, after) {
		return assemblyline.ArtifactIdentityProvenance{}, fmt.Errorf(
			"objective instruction path provenance root changed during inspection",
		)
	}
	for _, owner := range basenames {
		if !owner.ambiguous {
			selected[owner.path] = struct{}{}
		}
	}
	paths := make([]string, 0, len(selected))
	for path := range selected {
		paths = append(paths, path)
	}
	return modelcontext.NewArtifactIdentityProvenance(paths)
}
