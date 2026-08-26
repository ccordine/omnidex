package worker

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func createDirectCodingLanguageStageRoot(
	session *directCodingSession,
	config directCodingLanguageStageConfig,
) (string, string, error) {
	if !directCodingLanguageStageUsesDocker(config) {
		root, err := os.MkdirTemp("", "omnidex-"+config.Language+"-stage-")
		return root, "", err
	}
	if strings.TrimSpace(session.root) == "" {
		return "", "", fmt.Errorf("Docker-backed language stage requires an authoritative workspace root")
	}
	if err := validateV3CommandRoot(session.root); err != nil {
		return "", "", fmt.Errorf("Docker-backed language stage workspace: %w", err)
	}
	internalRoot := filepath.Join(session.root, ".omni")
	createdInternalRoot := false
	if err := os.Mkdir(internalRoot, 0o700); err != nil {
		if !os.IsExist(err) {
			return "", "", fmt.Errorf("create Docker-backed stage boundary: %w", err)
		}
		info, statErr := os.Lstat(internalRoot)
		if statErr != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
			return "", "", fmt.Errorf("Docker-backed stage boundary is not one exact directory")
		}
	} else {
		createdInternalRoot = true
	}
	root, err := os.MkdirTemp(internalRoot, "docker-"+config.Language+"-stage-")
	if err != nil {
		if createdInternalRoot {
			_ = os.Remove(internalRoot)
		}
		return "", "", err
	}
	removeEmptyRoot := ""
	if createdInternalRoot {
		removeEmptyRoot = internalRoot
	}
	return root, removeEmptyRoot, nil
}

func directCodingLanguageStageUsesDocker(config directCodingLanguageStageConfig) bool {
	for _, command := range config.CleanupCommands {
		if command.Name == "docker" {
			return true
		}
	}
	return false
}
