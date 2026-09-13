package model

import (
	"fmt"
	"path"
	"strings"
)

// A channel stores the client's native absolute path. Its representation must
// be checked independently of the operating system running the service. Only
// the filesystem owner resolves the path or establishes directory authority.
func validateChannelWorkspacePath(root string) error {
	if strings.HasPrefix(root, "/") {
		if path.Clean(root) != root {
			return fmt.Errorf("channel workspace root must be canonical")
		}
		return nil
	}
	if strings.Contains(root, "/") {
		return fmt.Errorf("Windows channel workspace root requires native separators")
	}
	var components []string
	switch {
	case len(root) >= 3 && asciiDriveLetter(root[0]) && root[1:3] == `:\`:
		if len(root) == 3 {
			return nil
		}
		components = strings.Split(root[3:], `\`)
	case strings.HasPrefix(root, `\\`):
		components = strings.Split(root[2:], `\`)
		if len(components) < 2 {
			return fmt.Errorf("UNC channel workspace root requires server and share")
		}
	default:
		return fmt.Errorf("channel workspace root must be absolute")
	}
	for _, component := range components {
		if component == "" || component == "." || component == ".." || strings.ContainsAny(component, `:<>"|?*`) {
			return fmt.Errorf("channel workspace root must be canonical")
		}
	}
	return nil
}

func asciiDriveLetter(value byte) bool {
	return value >= 'A' && value <= 'Z' || value >= 'a' && value <= 'z'
}
