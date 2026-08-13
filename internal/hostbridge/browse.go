package hostbridge

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

const (
	DefaultBrowsePageSize = 50
	MaxBrowsePageSize     = 100
	browseReadChunkSize   = 64
)

type Entry struct {
	Name  string `json:"name"`
	Path  string `json:"path"`
	IsDir bool   `json:"is_dir"`
}

type BrowseResult struct {
	Path           string  `json:"path"`
	Parent         string  `json:"parent,omitempty"`
	Entries        []Entry `json:"entries"`
	Limit          int     `json:"limit"`
	Offset         int     `json:"offset"`
	HasPrevious    bool    `json:"has_previous"`
	PreviousOffset int     `json:"previous_offset"`
	HasMore        bool    `json:"has_more"`
	NextOffset     int     `json:"next_offset,omitempty"`
}

type BrowseOptions struct {
	ExtraRoots      []string
	Limit           int
	Offset          int
	DirectoriesOnly bool
}

func DefaultBrowseRoot() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil || strings.TrimSpace(home) == "" {
		return "", fmt.Errorf("home directory unavailable")
	}
	return filepath.Clean(home), nil
}

func ListDirectory(target string, opts BrowseOptions) (*BrowseResult, error) {
	if err := validateBrowseBounds(opts); err != nil {
		return nil, err
	}
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
	items, hasMore, err := readDirectoryPage(abs, opts)
	if err != nil {
		return nil, err
	}
	parent := ""
	if parentPath := filepath.Dir(abs); parentPath != abs {
		parent = parentPath
	}
	return &BrowseResult{
		Path:        abs,
		Parent:      parent,
		Entries:     items,
		Limit:       opts.Limit,
		Offset:      opts.Offset,
		HasPrevious: opts.Offset > 0,
		PreviousOffset: func() int {
			previous := opts.Offset - opts.Limit
			if previous < 0 {
				return 0
			}
			return previous
		}(),
		HasMore: hasMore,
		NextOffset: func() int {
			if !hasMore {
				return 0
			}
			return opts.Offset + len(items)
		}(),
	}, nil
}

func validateBrowseBounds(opts BrowseOptions) error {
	if opts.Limit < 1 || opts.Limit > MaxBrowsePageSize {
		return fmt.Errorf("browse limit must be between 1 and %d", MaxBrowsePageSize)
	}
	if opts.Offset < 0 {
		return fmt.Errorf("browse offset must be non-negative")
	}
	return nil
}

func readDirectoryPage(path string, opts BrowseOptions) ([]Entry, bool, error) {
	directory, err := os.Open(path)
	if err != nil {
		return nil, false, err
	}
	defer directory.Close()

	items := make([]Entry, 0, opts.Limit+1)
	matched := 0
	for len(items) <= opts.Limit {
		batch, readErr := directory.ReadDir(browseReadChunkSize)
		for _, entry := range batch {
			if strings.HasPrefix(entry.Name(), ".") || (opts.DirectoriesOnly && !entry.IsDir()) {
				continue
			}
			if matched < opts.Offset {
				matched++
				continue
			}
			items = append(items, Entry{
				Name:  entry.Name(),
				Path:  filepath.Join(path, entry.Name()),
				IsDir: entry.IsDir(),
			})
			if len(items) > opts.Limit {
				break
			}
		}
		if len(items) > opts.Limit {
			break
		}
		if readErr != nil && readErr != io.EOF {
			return nil, false, fmt.Errorf("read directory page: %w", readErr)
		}
		if readErr == io.EOF || len(batch) == 0 {
			break
		}
	}
	hasMore := len(items) > opts.Limit
	if hasMore {
		items = items[:opts.Limit]
	}
	return items, hasMore, nil
}

func NonEmptyEntries(entries []Entry) []Entry {
	if entries == nil {
		return []Entry{}
	}
	return entries
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
