package hostbridge

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func CreateDirectory(parent, name string, opts BrowseOptions) (string, error) {
	parent = strings.TrimSpace(parent)
	name = strings.TrimSpace(name)
	if parent == "" {
		root, err := DefaultBrowseRoot()
		if err != nil {
			return "", err
		}
		parent = root
	}
	if err := validateDirectoryName(name); err != nil {
		return "", err
	}
	absParent, err := filepath.Abs(parent)
	if err != nil {
		return "", err
	}
	absParent = filepath.Clean(absParent)
	if err := ensureBrowseAllowed(absParent, opts); err != nil {
		return "", err
	}
	stat, err := os.Stat(absParent)
	if err != nil {
		return "", fmt.Errorf("parent directory does not exist")
	}
	if !stat.IsDir() {
		return "", fmt.Errorf("parent path must be a directory")
	}
	target := filepath.Clean(filepath.Join(absParent, name))
	if err := ensureBrowseAllowed(target, opts); err != nil {
		return "", err
	}
	if _, err := os.Stat(target); err == nil {
		return "", fmt.Errorf("directory already exists")
	} else if !os.IsNotExist(err) {
		return "", err
	}
	if err := os.Mkdir(target, 0o755); err != nil {
		return "", err
	}
	return target, nil
}

func validateDirectoryName(name string) error {
	if name == "" {
		return fmt.Errorf("folder name is required")
	}
	if name == "." || name == ".." {
		return fmt.Errorf("invalid folder name")
	}
	if strings.ContainsAny(name, `/\`) {
		return fmt.Errorf("folder name cannot contain path separators")
	}
	if strings.Contains(name, "\x00") {
		return fmt.Errorf("invalid folder name")
	}
	return nil
}
