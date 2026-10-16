package worker

import (
	"context"
	"fmt"
	"path"
	"strings"

	"github.com/gryph/omnidex/internal/assemblyline"
	"github.com/gryph/omnidex/internal/model"
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
	selected := make(map[string]struct{})
	for _, token := range modelcontext.LexicalPathTokens(instruction) {
		if err := ctx.Err(); err != nil {
			return assemblyline.ArtifactIdentityProvenance{}, err
		}
		recognized := recognizedObjectiveArtifactName(token.Value)
		qualified := false
		for _, identity := range modelcontext.PathIdentities(
			token.Value, assemblyline.ArtifactIdentityProvenance{},
		) {
			if identity.Start == 0 && identity.End == len(token.Value) {
				qualified = true
				break
			}
		}
		if token.Quoted && !plausibleQuotedObjectiveArtifact(token.Value, recognized) {
			continue
		}
		if token.Quoted && strings.ContainsAny(token.Value, `/\`) {
			qualified = true
		}
		if !recognized && !qualified {
			continue
		}
		relative, err := objectiveRelativeArtifactPath(root, token.Value)
		if err != nil {
			continue
		}
		selected[relative] = struct{}{}
	}
	paths := make([]string, 0, len(selected))
	for path := range selected {
		paths = append(paths, path)
	}
	return modelcontext.NewArtifactIdentityProvenance(paths)
}

func recognizedObjectiveArtifactName(value string) bool {
	base := path.Base(strings.ReplaceAll(value, `\`, "/"))
	switch base {
	case "go.mod", "go.sum", "Cargo.toml", "Cargo.lock", "Dockerfile", "Makefile", ".gitignore":
		return true
	}
	extension := strings.ToLower(path.Ext(base))
	switch extension {
	case ".go", ".ts", ".tsx", ".js", ".jsx", ".mjs", ".cjs", ".css", ".html", ".java", ".rs", ".json", ".yaml", ".yml", ".toml", ".sql", ".sh", ".py", ".md", ".txt":
		return true
	default:
		return false
	}
}

func plausibleQuotedObjectiveArtifact(value string, recognized bool) bool {
	if value == "" || value == "-" || value == "--" || value != strings.TrimSpace(value) ||
		strings.ContainsAny(value, "\r\n\t{}[]<>,;'\"") {
		return false
	}
	if strings.ContainsRune(value, ':') && !(len(value) >= 2 && value[1] == ':') {
		return false
	}
	return recognized || !strings.ContainsRune(value, ' ') || strings.ContainsAny(value, `/\`)
}

func objectiveRelativeArtifactPath(root, candidate string) (string, error) {
	if err := model.ValidateChannelWorkspaceRoot(root); err != nil {
		return "", err
	}
	windows := !strings.HasPrefix(root, "/")
	if windows {
		root = strings.ReplaceAll(root, `\`, "/")
		candidate = strings.ReplaceAll(candidate, `\`, "/")
	} else if strings.Contains(candidate, `\`) {
		return "", fmt.Errorf("artifact path must use client path syntax")
	}
	absolute := strings.HasPrefix(candidate, "/")
	if windows && strings.Contains(candidate, ":") {
		if len(candidate) < 3 || candidate[1:3] != ":/" {
			return "", fmt.Errorf("artifact path must not be drive-relative")
		}
		absolute = true
	}
	if absolute {
		candidate = cleanRootedObjectiveArtifactPath(candidate, windows)
		var contained bool
		candidate, contained = strings.CutPrefix(candidate, strings.TrimSuffix(root, "/")+"/")
		if !contained {
			return "", fmt.Errorf("artifact path is outside the authoritative workspace")
		}
	}
	candidate = path.Clean(candidate)
	if candidate == "." || candidate == ".." || path.IsAbs(candidate) ||
		len(candidate) >= 3 && candidate[:3] == "../" {
		return "", fmt.Errorf("artifact path is outside the authoritative workspace")
	}
	return candidate, nil
}

func cleanRootedObjectiveArtifactPath(candidate string, windows bool) string {
	if !windows {
		return path.Clean(candidate)
	}
	if len(candidate) >= 3 && candidate[1:3] == ":/" {
		return candidate[:2] + path.Clean(candidate[2:])
	}
	if strings.HasPrefix(candidate, "//") {
		parts := strings.SplitN(candidate[2:], "/", 3)
		if len(parts) == 3 {
			return "//" + parts[0] + "/" + parts[1] + path.Clean("/"+parts[2])
		}
	}
	return candidate
}
