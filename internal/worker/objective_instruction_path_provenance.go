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
	workspaceRoot, err := os.OpenRoot(root)
	if err != nil {
		return assemblyline.ArtifactIdentityProvenance{}, fmt.Errorf(
			"open objective workspace root: %w", err,
		)
	}
	defer workspaceRoot.Close()
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
		_, _, recognized, err := recognizeDirectCodingArtifactAdapterForPath(token.Value)
		if err != nil {
			return assemblyline.ArtifactIdentityProvenance{}, err
		}
		relative, err := objectiveRelativeArtifactPath(root, token.Value)
		if err != nil {
			continue
		}
		_, statErr := workspaceRoot.Lstat(relative)
		if recognized || token.Quoted || statErr == nil {
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
