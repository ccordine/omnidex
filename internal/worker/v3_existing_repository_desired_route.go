package worker

import (
	"fmt"
	"path"

	"github.com/gryph/omnidex/internal/assemblyline"
	"github.com/gryph/omnidex/internal/station"
)

func classifyExistingRepositoryArtifactDirectives(
	session *directCodingSession,
	redacted string,
	identities []assemblyline.ArtifactIdentity,
) ([]assemblyline.ArtifactDirective, error) {
	if len(identities) == 0 {
		return nil, nil
	}
	if session == nil || session.repositoryIndex == nil {
		return nil, fmt.Errorf("artifact desired-state classification requires one indexed session")
	}
	indexed := make(map[string]struct{}, len(session.repositoryIndex.Snapshot.Files))
	for _, file := range session.repositoryIndex.Snapshot.Files {
		indexed[file.Path] = struct{}{}
	}
	present, absent := 0, 0
	for _, identity := range identities {
		clean := path.Clean(identity.Value)
		if clean != identity.Value {
			return nil, fmt.Errorf("artifact identity %q is not one normalized indexed path", identity.Token)
		}
		if _, exists := indexed[clean]; !exists {
			absent++
			continue
		}
		present++
	}
	if present > 0 && absent > 0 {
		return nil, fmt.Errorf(
			"rename, move, and mixed named file-state identity transfer are unsupported",
		)
	}
	if absent > 0 {
		return nil, fmt.Errorf("artifact identity is absent from the exact repository snapshot")
	}
	modelName, err := session.workerModel(station.CodingArtifactHandling)
	if err != nil {
		return nil, err
	}
	return classifyArtifactHandling(
		directCodingWorkerRuntime(session), modelName, redacted, identities,
	)
}
