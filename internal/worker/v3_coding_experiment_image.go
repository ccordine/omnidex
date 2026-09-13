package worker

import "fmt"

func directCodingExperimentImage(profile directCodingProjectVersionProfile) (string, error) {
	switch profile.StackID {
	case genericJavaScriptCommandLineAdapter, genericTypeScriptBrowserAdapter:
		return "node:22-alpine", nil
	case genericGoCommandLineAdapter:
		version, err := directCodingVersionComponent(profile, "go")
		if err != nil {
			return "", err
		}
		return "golang:" + version + "-alpine", nil
	case genericRustCommandLineAdapter:
		version, err := directCodingVersionComponent(profile, "rust_version")
		if err != nil {
			return "", err
		}
		return "rust:" + version + "-slim", nil
	case genericJavaCommandLineAdapter:
		release, err := directCodingVersionComponent(profile, "java_release")
		if err != nil {
			return "", err
		}
		return "eclipse-temurin:" + release + "-jdk-jammy", nil
	default:
		return "", fmt.Errorf("stack %s has no registered experiment image", profile.StackID)
	}
}
