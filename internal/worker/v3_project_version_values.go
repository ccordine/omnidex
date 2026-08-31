package worker

import (
	"fmt"
	"strings"
)

func directCodingVersionComponent(
	profile directCodingProjectVersionProfile,
	name string,
) (string, error) {
	for _, component := range profile.Components {
		if component.Name == name {
			return component.Version, nil
		}
	}
	return "", fmt.Errorf("version profile %s lacks component %s", profile.ID, name)
}

func normalizedSemver(value string) string {
	value = strings.TrimPrefix(strings.TrimSpace(value), "v")
	if strings.Count(value, ".") == 1 {
		value += ".0"
	}
	return "v" + value
}
