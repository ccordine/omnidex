package worker

import _ "embed"

//go:embed static/typescript_browser_package_lock.json
var typeScriptBrowserPackageLockTemplate []byte

func typeScriptBrowserPackageLock(
	profile directCodingProjectVersionProfile,
	packageName string,
) (string, error) {
	node, err := directCodingVersionComponent(profile, "node")
	if err != nil {
		return "", err
	}
	npm, err := directCodingVersionComponent(profile, "npm")
	if err != nil {
		return "", err
	}
	lockVersion, err := directCodingNPMLockVersion(profile)
	if err != nil {
		return "", err
	}
	return materializePinnedNPMLock(
		profile.NPMLockTemplate,
		packageName,
		lockVersion,
		profile.NPMDependencies,
		profile.NPMDevDependencies,
		map[string]string{"node": node, "npm": npm},
	)
}
