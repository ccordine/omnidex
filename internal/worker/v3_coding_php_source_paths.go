package worker

import (
	"fmt"
	"sort"
	"strings"
)

func phpServiceSourcePaths(program directCodingProgram) ([]string, error) {
	seen := make(map[string]struct{})
	for _, document := range program.Source.Documents {
		if !strings.HasSuffix(document.Path, ".php") {
			continue
		}
		seen[document.Path] = struct{}{}
	}
	for _, file := range program.StaticFiles {
		if strings.HasSuffix(file.Path, ".php") {
			seen[file.Path] = struct{}{}
		}
	}
	paths := make([]string, 0, len(seen))
	for sourcePath := range seen {
		paths = append(paths, sourcePath)
	}
	sort.Strings(paths)
	return paths, nil
}

func requirePHPServiceSourcePaths(paths []string, required ...string) error {
	present := make(map[string]struct{}, len(paths))
	for _, sourcePath := range paths {
		present[sourcePath] = struct{}{}
	}
	for _, sourcePath := range required {
		if _, exists := present[sourcePath]; !exists {
			return fmt.Errorf("PHP HTTP verification stage lacks required source %s", sourcePath)
		}
	}
	return nil
}
