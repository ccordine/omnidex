package hostbridge

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

type Entry struct {
	Name  string `json:"name"`
	Path  string `json:"path"`
	IsDir bool   `json:"is_dir"`
}

type BrowseResult struct {
	Path    string  `json:"path"`
	Parent  string  `json:"parent,omitempty"`
	Entries []Entry `json:"entries"`
}

type BrowseOptions struct {
	ExtraRoots []string
}

func DefaultBrowseRoot() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil || strings.TrimSpace(home) == "" {
		return "", fmt.Errorf("home directory unavailable")
	}
	return filepath.Clean(home), nil
}

func ListDirectory(target string, opts BrowseOptions) (*BrowseResult, error) {
	if strings.TrimSpace(target) == "" {
		root, err := DefaultBrowseRoot()
		if err != nil {
			return nil, err
		}
		target = root
	}
	abs, err := filepath.Abs(target)
	if err != nil {
		return nil, err
	}
	abs = filepath.Clean(abs)
	if err := ensureBrowseAllowed(abs, opts); err != nil {
		return nil, err
	}
	stat, err := os.Stat(abs)
	if err != nil {
		return nil, fmt.Errorf("path does not exist")
	}
	if !stat.IsDir() {
		return nil, fmt.Errorf("path must be a directory")
	}
	entries, err := os.ReadDir(abs)
	if err != nil {
		return nil, err
	}
	items := make([]Entry, 0, len(entries))
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), ".") {
			continue
		}
		full := filepath.Join(abs, entry.Name())
		info, err := entry.Info()
		if err != nil {
			continue
		}
		items = append(items, Entry{
			Name:  entry.Name(),
			Path:  full,
			IsDir: info.IsDir(),
		})
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].IsDir != items[j].IsDir {
			return items[i].IsDir
		}
		return strings.ToLower(items[i].Name) < strings.ToLower(items[j].Name)
	})
	parent := ""
	if parentPath := filepath.Dir(abs); parentPath != abs {
		parent = parentPath
	}
	return &BrowseResult{
		Path:    abs,
		Parent:  parent,
		Entries: items,
	}, nil
}

func ensureBrowseAllowed(abs string, opts BrowseOptions) error {
	roots := make([]string, 0, 4+len(opts.ExtraRoots))
	if home, err := os.UserHomeDir(); err == nil && strings.TrimSpace(home) != "" {
		roots = append(roots, filepath.Clean(home))
	}
	for _, raw := range opts.ExtraRoots {
		root := filepath.Clean(strings.TrimSpace(raw))
		if root != "" {
			roots = append(roots, root)
		}
	}
	for _, envRoot := range strings.Split(os.Getenv("HOST_BROWSE_ROOTS"), ",") {
		root := filepath.Clean(strings.TrimSpace(envRoot))
		if root != "" {
			roots = append(roots, root)
		}
	}
	for _, root := range roots {
		if underRoot(abs, root) || underRoot(root, abs) {
			return nil
		}
	}
	return fmt.Errorf("path outside allowed browse roots")
}

func underRoot(path, root string) bool {
	if path == root {
		return true
	}
	sep := string(os.PathSeparator)
	return strings.HasPrefix(path, root+sep)
}
