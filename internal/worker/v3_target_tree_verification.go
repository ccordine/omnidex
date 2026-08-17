package worker

import (
	"fmt"
	"os"

	"github.com/gryph/omnidex/internal/assemblyline"
)

// directCodingTargetTreeWorkspaceDiagnostic proves semantic artifacts exist in
// the host workspace before command verification. It never trusts an in-memory
// document or a container stage as completion evidence.
func directCodingTargetTreeWorkspaceDiagnostic(
	root string,
	target assemblyline.TargetTree,
) (*directCodingDiagnostic, error) {
	if len(target.Artifacts) == 0 {
		return nil, fmt.Errorf("accepted target tree cannot be empty")
	}
	for _, artifact := range target.Artifacts {
		resolved, err := resolveV3WorkspaceFile(root, artifact.Path)
		if err != nil {
			return nil, fmt.Errorf("resolve target artifact %s: %w", artifact.ID, err)
		}
		info, err := os.Stat(resolved)
		if err != nil {
			if os.IsNotExist(err) {
				return directCodingStaticFileDiagnostic(artifact.Path, "accepted target-tree artifact is absent from the host workspace"), nil
			}
			return nil, fmt.Errorf("inspect target artifact %s: %w", artifact.Path, err)
		}
		if info.IsDir() {
			return directCodingStaticFileDiagnostic(artifact.Path, "accepted target-tree artifact is a directory in the host workspace"), nil
		}
	}
	return nil, nil
}
