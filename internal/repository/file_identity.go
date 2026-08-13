package repository

import "fmt"

// FileIDForAbsentPath derives the identity a regular file will receive after a
// code-owned desired-state transition creates it. It grants no path selection
// authority: the caller must already hold a validated snapshot and path.
func FileIDForAbsentPath(snapshot Snapshot, path string) (string, error) {
	if err := snapshot.Validate(); err != nil {
		return "", fmt.Errorf("derive repository file identity: %w", err)
	}
	if err := validateRelativeRepositoryPath(path); err != nil {
		return "", fmt.Errorf("derive repository file identity: %w", err)
	}
	for _, file := range snapshot.Files {
		if file.Path == path {
			return "", fmt.Errorf("derive repository file identity: path %q already exists", path)
		}
	}
	for _, exclusion := range snapshot.Exclusions {
		if exclusion.Path == path {
			return "", fmt.Errorf(
				"derive repository file identity: path %q has excluded authority %q",
				path, exclusion.Reason,
			)
		}
	}
	return opaqueID("file_", snapshot.RepositoryID, path), nil
}
