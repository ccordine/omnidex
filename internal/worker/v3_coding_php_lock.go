package worker

import (
	_ "embed"
)

//go:embed static/php_service_package_lock.json
var phpServicePackageLockTemplate []byte

func phpServicePackageLock(
	profile directCodingProjectVersionProfile,
	packageName string,
) (string, error) {
	node, err := directCodingVersionComponent(profile, "node")
	if err != nil {
		return "", err
	}
	lockVersion, err := directCodingNPMLockVersion(profile)
	if err != nil {
		return "", err
	}
	return materializePinnedNPMLock(
		profile.NPMLockTemplate, packageName, lockVersion, nil, profile.NPMDevDependencies,
		map[string]string{"node": node},
	)
}
