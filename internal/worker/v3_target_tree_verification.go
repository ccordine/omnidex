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
	if len(target.Paths) == 0 {
		return nil, fmt.Errorf("accepted target tree cannot be empty")
	}
	for _, filePath := range target.Paths {
		resolved, err := resolveV3WorkspaceFile(root, filePath)
		if err != nil {
			return nil, fmt.Errorf("resolve target tree path %s: %w", filePath, err)
		}
		info, err := os.Stat(resolved)
		if err != nil {
			if os.IsNotExist(err) {
				return directCodingStaticFileDiagnostic(filePath, "accepted target-tree path is absent from the host workspace"), nil
			}
			return nil, fmt.Errorf("inspect target tree path %s: %w", filePath, err)
		}
		if info.IsDir() {
			return directCodingStaticFileDiagnostic(filePath, "accepted target-tree path is a directory in the host workspace"), nil
		}
	}
	return nil, nil
}
