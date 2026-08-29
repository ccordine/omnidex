package changeapply

import (
	repositoryfacts "github.com/gryph/omnidex/internal/repository"
)

func exactGoPackagePlacement(
	snapshot repositoryfacts.Snapshot,
	analysis repositoryfacts.Analysis,
	artifactID string,
) (string, string, error) {
	placement, err := repositoryfacts.ResolveGoPackagePlacement(snapshot, analysis, artifactID)
	if err != nil {
		return "", "", err
	}
	return placement.Name, placement.Directory, nil
}
