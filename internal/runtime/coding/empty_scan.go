package coding

import (
	"context"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

type fileEmptyScanner struct {
	root string
}

func NewEmptyFileScanner(root string) EmptyFileScanner {
	root = strings.TrimSpace(root)
	if root == "" {
		root = "."
	}
	return fileEmptyScanner{root: root}
}

func (s fileEmptyScanner) ScanEmptyFiles(ctx context.Context, workspace string) (EmptyFileReport, error) {
	root := strings.TrimSpace(workspace)
	if root == "" {
		root = s.root
	}
	var files []string
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		if entry.IsDir() {
			switch entry.Name() {
			case ".git", "node_modules", "vendor":
				if path != root {
					return filepath.SkipDir
				}
			}
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if info.Size() != 0 {
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		files = append(files, filepath.ToSlash(rel))
		return nil
	})
	if err != nil {
		if os.IsNotExist(err) {
			return EmptyFileReport{}, nil
		}
		return EmptyFileReport{}, err
	}
	sort.Strings(files)
	return EmptyFileReport{Files: files}, nil
}
