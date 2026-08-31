package worker

import (
	"context"
	"fmt"
	"os"
	"path"
	"path/filepath"

	"github.com/gryph/omnidex/internal/assemblyline"
	"github.com/gryph/omnidex/internal/modelcontext"
)

func objectiveWorkspaceState(root string) (assemblyline.ApplicationWorkspaceState, error) {
	root = filepath.Clean(root)
	if !filepath.IsAbs(root) {
		return "", fmt.Errorf("objective workspace root must be absolute")
	}
	entries, err := os.ReadDir(root)
	if err != nil {
		return "", fmt.Errorf("inspect objective workspace root %q: %w", root, err)
	}
	for _, entry := range entries {
		if entry.Name() != ".git" && entry.Name() != ".omni" {
			return assemblyline.ApplicationWorkspaceExisting, nil
		}
	}
	return assemblyline.ApplicationWorkspaceEmpty, nil
}

// objectiveInstructionPathProvenance derives artifact identities directly
// from the instruction's path grammar. It never inventories the workspace:
// existence and contents belong to the first filesystem consumer.
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
	selected := make(map[string]struct{})
	for _, identity := range modelcontext.PathIdentities(
		instruction, assemblyline.ArtifactIdentityProvenance{},
	) {
		relative, err := objectiveRelativeArtifactPath(root, identity.Value)
		if err == nil {
			selected[relative] = struct{}{}
		}
	}
	for _, token := range modelcontext.LexicalPathTokens(instruction) {
		if err := ctx.Err(); err != nil {
			return assemblyline.ArtifactIdentityProvenance{}, err
		}
		if _, _, recognized, err := recognizeDirectCodingArtifactAdapterForPath(token.Value); err != nil {
			return assemblyline.ArtifactIdentityProvenance{}, err
		} else if !recognized {
			continue
		}
		relative, err := objectiveRelativeArtifactPath(root, token.Value)
		if err == nil {
			selected[relative] = struct{}{}
		}
	}
	paths := make([]string, 0, len(selected))
	for path := range selected {
		paths = append(paths, path)
	}
	return modelcontext.NewArtifactIdentityProvenance(paths)
}

func objectiveRelativeArtifactPath(root, candidate string) (string, error) {
	if filepath.IsAbs(candidate) {
		relative, err := filepath.Rel(root, filepath.Clean(candidate))
		if err != nil {
			return "", err
		}
		candidate = filepath.ToSlash(relative)
	} else {
		candidate = filepath.ToSlash(candidate)
	}
	candidate = path.Clean(candidate)
	if candidate == "." || candidate == ".." || path.IsAbs(candidate) ||
		len(candidate) >= 3 && candidate[:3] == "../" {
		return "", fmt.Errorf("artifact path is outside the authoritative workspace")
	}
	return candidate, nil
}
