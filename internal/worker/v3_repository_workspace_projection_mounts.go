package worker

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	workspacefacts "github.com/gryph/omnidex/internal/workspace"
)

const maxRepositoryWorkspaceProjectionNodes = 200_000

type repositoryWorkspaceProjectionMount struct {
	Path       string
	Source     repositoryWorkspaceProjectionSource
	Directory  bool
	LinkTarget string
}

type repositoryWorkspaceProjectionNode struct {
	file     *repositoryWorkspaceProjectionFile
	children map[string]*repositoryWorkspaceProjectionNode
}

type repositoryWorkspaceProjectionBaseMatcher struct {
	root    string
	matches map[*repositoryWorkspaceProjectionNode]bool
	known   map[*repositoryWorkspaceProjectionNode]bool
}

type repositoryWorkspaceProjectionMountRoots struct {
	base  string
	delta string
}

func repositoryWorkspaceProjectionMounts(
	projection repositoryWorkspaceProjection,
	roots repositoryWorkspaceProjectionMountRoots,
) ([]repositoryWorkspaceProjectionMount, error) {
	if err := projection.validate(); err != nil {
		return nil, err
	}
	if err := roots.validate(projection); err != nil {
		return nil, err
	}
	root, err := newRepositoryWorkspaceProjectionTree(projection.files)
	if err != nil {
		return nil, err
	}
	matchers := map[repositoryWorkspaceProjectionSource]*repositoryWorkspaceProjectionBaseMatcher{
		repositoryWorkspaceProjectionBase: {
			root: roots.base, matches: make(map[*repositoryWorkspaceProjectionNode]bool),
			known: make(map[*repositoryWorkspaceProjectionNode]bool),
		},
	}
	if roots.delta != "" {
		matchers[repositoryWorkspaceProjectionDelta] = &repositoryWorkspaceProjectionBaseMatcher{
			root: roots.delta, matches: make(map[*repositoryWorkspaceProjectionNode]bool),
			known: make(map[*repositoryWorkspaceProjectionNode]bool),
		}
	}
	mounts := make([]repositoryWorkspaceProjectionMount, 0, len(projection.files))
	if err := appendRepositoryWorkspaceProjectionMounts(root, "", matchers, &mounts); err != nil {
		return nil, err
	}
	return mounts, nil
}

func (roots repositoryWorkspaceProjectionMountRoots) validate(
	projection repositoryWorkspaceProjection,
) error {
	for _, authority := range []struct {
		label string
		root  string
	}{
		{label: "source", root: roots.base},
		{label: "delta", root: roots.delta},
	} {
		label, root := authority.label, authority.root
		if root == "" {
			if label == "source" || projection.deltaRoot != "" {
				return fmt.Errorf("repository projection %s mount root is required", label)
			}
			continue
		}
		info, err := os.Stat(root)
		if err != nil {
			return fmt.Errorf("inspect repository projection %s mount root: %w", label, err)
		}
		if !info.IsDir() {
			return fmt.Errorf("repository projection %s mount root is not a directory", label)
		}
	}
	if projection.deltaRoot == "" && roots.delta != "" {
		return fmt.Errorf("repository snapshot projection received an unexpected delta mount root")
	}
	return nil
}

func newRepositoryWorkspaceProjectionTree(
	files []repositoryWorkspaceProjectionFile,
) (*repositoryWorkspaceProjectionNode, error) {
	root := &repositoryWorkspaceProjectionNode{children: make(map[string]*repositoryWorkspaceProjectionNode)}
	nodeCount := 1
	for index := range files {
		file := &files[index]
		node := root
		parts := strings.Split(file.Path, "/")
		for partIndex, part := range parts {
			if node.file != nil {
				return nil, fmt.Errorf("repository workspace projection path %q descends through a file", file.Path)
			}
			child := node.children[part]
			if child == nil {
				nodeCount++
				if nodeCount > maxRepositoryWorkspaceProjectionNodes {
					return nil, fmt.Errorf(
						"repository workspace projection exceeds %d exact path nodes",
						maxRepositoryWorkspaceProjectionNodes,
					)
				}
				child = &repositoryWorkspaceProjectionNode{children: make(map[string]*repositoryWorkspaceProjectionNode)}
				node.children[part] = child
			}
			node = child
			if partIndex == len(parts)-1 {
				if node.file != nil || len(node.children) != 0 {
					return nil, fmt.Errorf("repository workspace projection path %q collides with another entry", file.Path)
				}
				node.file = file
			}
		}
	}
	return root, nil
}

func appendRepositoryWorkspaceProjectionMounts(
	node *repositoryWorkspaceProjectionNode,
	relative string,
	matchers map[repositoryWorkspaceProjectionSource]*repositoryWorkspaceProjectionBaseMatcher,
	mounts *[]repositoryWorkspaceProjectionMount,
) error {
	if source, homogeneous := repositoryProjectionNodeDirectorySource(node); relative != "" && homogeneous {
		matcher := matchers[source]
		if matcher == nil {
			return fmt.Errorf("repository projection directory %q lacks its exact source", relative)
		}
		exact, err := matcher.exactDirectory(node, relative)
		if err != nil {
			return err
		}
		if exact {
			*mounts = append(*mounts, repositoryWorkspaceProjectionMount{
				Path: relative, Source: source, Directory: true,
			})
			return nil
		}
	}
	for _, name := range sortedRepositoryProjectionNodeNames(node) {
		child := node.children[name]
		childPath := name
		if relative != "" {
			childPath = relative + "/" + name
		}
		if child.file == nil {
			if err := appendRepositoryWorkspaceProjectionMounts(child, childPath, matchers, mounts); err != nil {
				return err
			}
			continue
		}
		mount := repositoryWorkspaceProjectionMount{Path: childPath, Source: child.file.Source}
		if child.file.Source == repositoryWorkspaceProjectionSymlink {
			mount.LinkTarget = child.file.LinkTarget
		}
		*mounts = append(*mounts, mount)
	}
	return nil
}

func repositoryProjectionNodeDirectorySource(
	node *repositoryWorkspaceProjectionNode,
) (repositoryWorkspaceProjectionSource, bool) {
	if node.file != nil {
		if node.file.Source == repositoryWorkspaceProjectionSymlink {
			return repositoryWorkspaceProjectionBase, true
		}
		return node.file.Source, true
	}
	var source repositoryWorkspaceProjectionSource
	for _, child := range node.children {
		childSource, homogeneous := repositoryProjectionNodeDirectorySource(child)
		if !homogeneous {
			return "", false
		}
		if source == "" {
			source = childSource
		} else if source != childSource {
			return "", false
		}
	}
	return source, source != ""
}

func (matcher *repositoryWorkspaceProjectionBaseMatcher) exactDirectory(
	node *repositoryWorkspaceProjectionNode,
	relative string,
) (bool, error) {
	if matcher.known[node] {
		return matcher.matches[node], nil
	}
	abs := filepath.Join(matcher.root, filepath.FromSlash(relative))
	entries, err := os.ReadDir(abs)
	if err != nil {
		return false, fmt.Errorf("inspect repository projection directory %q: %w", relative, err)
	}
	names := sortedRepositoryProjectionNodeNames(node)
	if len(entries) != len(names) {
		matcher.known[node] = true
		return false, nil
	}
	for index, entry := range entries {
		if entry.Name() != names[index] {
			matcher.known[node] = true
			return false, nil
		}
		child := node.children[names[index]]
		childPath := relative + "/" + names[index]
		info, err := os.Lstat(filepath.Join(matcher.root, filepath.FromSlash(childPath)))
		if err != nil {
			return false, fmt.Errorf("inspect repository projection entry %q: %w", childPath, err)
		}
		if child.file == nil {
			if !info.IsDir() {
				matcher.known[node] = true
				return false, nil
			}
			exact, err := matcher.exactDirectory(child, childPath)
			if err != nil {
				return false, err
			}
			if !exact {
				matcher.known[node] = true
				return false, nil
			}
			continue
		}
		if !repositoryProjectionEntryKindMatches(child.file.Kind, info.Mode()) {
			matcher.known[node] = true
			return false, nil
		}
		if child.file.Kind == workspacefacts.EntrySymlink {
			target, err := os.Readlink(filepath.Join(matcher.root, filepath.FromSlash(childPath)))
			if err != nil {
				return false, fmt.Errorf("read repository projection symlink %q: %w", childPath, err)
			}
			if target != child.file.LinkTarget {
				matcher.known[node] = true
				return false, nil
			}
		}
	}
	matcher.known[node] = true
	matcher.matches[node] = true
	return true, nil
}

func repositoryProjectionEntryKindMatches(kind workspacefacts.EntryKind, mode os.FileMode) bool {
	switch kind {
	case workspacefacts.EntryRegular:
		return mode.IsRegular()
	case workspacefacts.EntrySymlink:
		return mode&os.ModeSymlink != 0
	default:
		return false
	}
}

func sortedRepositoryProjectionNodeNames(node *repositoryWorkspaceProjectionNode) []string {
	names := make([]string, 0, len(node.children))
	for name := range node.children {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}
