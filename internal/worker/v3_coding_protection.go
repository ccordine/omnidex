package worker

import (
	"fmt"
)

func directCodingProtectedPathSet(paths []string) map[string]struct{} {
	protected := make(map[string]struct{}, len(paths))
	for _, path := range paths {
		protected[path] = struct{}{}
	}
	return protected
}

func rejectDirectCodingProtectedMutation(path string, protected map[string]struct{}) error {
	if _, exists := protected[path]; exists {
		return fmt.Errorf("coding path %s is server-protected by an explicit user requirement", path)
	}
	return nil
}
