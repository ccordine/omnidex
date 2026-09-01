package worker

import _ "embed"

//go:embed static/typescript_browser_package_lock.json
var typeScriptBrowserPackageLockTemplate []byte

func typeScriptBrowserPackageLock(
	profile directCodingProjectVersionProfile,
	packageName string,
) (string, map[string]string, map[string]string, error) {
	node, err := directCodingVersionComponent(profile, "node")
	if err != nil {
		return "", nil, nil, err
	}
	npm, err := directCodingVersionComponent(profile, "npm")
	if err != nil {
		return "", nil, nil, err
	}
	return materializePinnedNPMLock(
		typeScriptBrowserPackageLockTemplate,
		packageName,
		map[string]string{"node": node, "npm": npm},
	)
}
