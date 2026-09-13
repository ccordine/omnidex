package workspace

import (
	"container/heap"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"sort"
	"strings"
)

func ReadDirectory(ctx context.Context, root RootReader, relative, after string, limit int) (page DirectoryPage, resultErr error) {
	if err := validateRead(ctx, root, relative); err != nil {
		return page, err
	}
	if limit < 1 || limit > MaxDirectoryPageEntries {
		return page, fmt.Errorf("directory page limit must be between 1 and %d", MaxDirectoryPageEntries)
	}
	if after != "" && (strings.ContainsAny(after, `/\`) || validateRelativePath(after) != nil) {
		return page, fmt.Errorf("directory page cursor requires one exact basename")
	}
	before, err := root.Lstat(relative)
	if err != nil {
		return page, err
	}
	if !before.IsDir() {
		return page, fmt.Errorf("workspace listing requires an exact directory")
	}
	directory, err := root.Open(relative)
	if err != nil {
		return page, err
	}
	defer func() { resultErr = errors.Join(resultErr, directory.Close()) }()
	opened, err := directory.Stat()
	if err != nil || !os.SameFile(before, opened) {
		return page, errors.Join(fmt.Errorf("workspace listing directory changed"), err)
	}
	names := new(directoryNames)
	for {
		if err := ctx.Err(); err != nil {
			return page, err
		}
		entries, err := directory.ReadDir(128)
		for _, entry := range entries {
			name := entry.Name()
			if name <= after {
				continue
			}
			if names.Len() <= limit {
				heap.Push(names, name)
			} else if name < (*names)[0] {
				(*names)[0] = name
				heap.Fix(names, 0)
			}
		}
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return page, err
		}
	}
	selected := []string(*names)
	sort.Strings(selected)
	page.HasMore = len(selected) > limit
	if page.HasMore {
		selected = selected[:limit]
	}
	page.Entries = make([]Entry, 0, len(selected))
	for _, name := range selected {
		if err := validateRelativePath(name); err != nil {
			return page, err
		}
		entry, err := ReadEntry(ctx, root, path.Join(relative, name))
		if err != nil {
			return page, err
		}
		page.Entries = append(page.Entries, entry)
	}
	current, err := root.Lstat(relative)
	if err != nil || !os.SameFile(before, current) || !before.ModTime().Equal(current.ModTime()) {
		return page, errors.Join(fmt.Errorf("workspace directory changed during pagination"), err)
	}
	return page, nil
}

// Keep only the next bounded page while scanning the directory; its size does
// not determine the amount of memory allocated for a listing.
type directoryNames []string

func (names directoryNames) Len() int           { return len(names) }
func (names directoryNames) Less(i, j int) bool { return names[i] > names[j] }
func (names directoryNames) Swap(i, j int)      { names[i], names[j] = names[j], names[i] }
func (names *directoryNames) Push(value any)    { *names = append(*names, value.(string)) }
func (names *directoryNames) Pop() any {
	last := len(*names) - 1
	value := (*names)[last]
	*names = (*names)[:last]
	return value
}
