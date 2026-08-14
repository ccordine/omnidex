package projectgit

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"unicode/utf8"
)

const maxGitMarkerBytes = 4 * 1024

func hasRepositoryMarker(location string) (bool, error) {
	info, err := os.Stat(location)
	if err != nil {
		return false, fmt.Errorf("inspect project location: %w", err)
	}
	if !info.IsDir() {
		return false, fmt.Errorf("project location is not a directory")
	}
	current, err := filepath.Abs(location)
	if err != nil {
		return false, fmt.Errorf("resolve project location: %w", err)
	}
	ceilings, err := repositoryCeilings()
	if err != nil {
		return false, err
	}
	for {
		if _, stop := ceilings[current]; stop {
			break
		}
		marker := filepath.Join(current, ".git")
		markerInfo, markerErr := os.Lstat(marker)
		if markerErr == nil {
			valid, validationErr := validRepositoryMarker(marker, markerInfo)
			if validationErr != nil {
				return false, validationErr
			} else if valid {
				return true, nil
			}
			if markerInfo.Mode()&os.ModeSymlink != 0 || (!markerInfo.IsDir() && !markerInfo.Mode().IsRegular()) {
				return false, fmt.Errorf("git marker %q is not a regular file or directory", marker)
			}
		} else if !errors.Is(markerErr, os.ErrNotExist) {
			return false, fmt.Errorf("inspect git marker %q: %w", marker, markerErr)
		}
		parent := filepath.Dir(current)
		if parent == current {
			break
		}
		current = parent
	}
	return false, nil
}

func repositoryCeilings() (map[string]struct{}, error) {
	ceilings := map[string]struct{}{}
	raw := os.Getenv("GIT_CEILING_DIRECTORIES")
	if raw == "" {
		return ceilings, nil
	}
	for _, value := range filepath.SplitList(raw) {
		if value == "" || value != filepath.Clean(value) || !filepath.IsAbs(value) {
			return nil, fmt.Errorf("GIT_CEILING_DIRECTORIES contains an inexact path")
		}
		ceilings[value] = struct{}{}
	}
	return ceilings, nil
}

func validRepositoryMarker(marker string, info os.FileInfo) (bool, error) {
	if info.Mode()&os.ModeSymlink != 0 {
		return false, fmt.Errorf("git marker %q must not be a symlink", marker)
	}
	gitDir := marker
	if info.Mode().IsRegular() {
		line, err := readMarkerLine(marker)
		if err != nil {
			return false, fmt.Errorf("read git marker %q: %w", marker, err)
		}
		if !strings.HasPrefix(line, "gitdir: ") {
			return false, fmt.Errorf("git marker %q is malformed", marker)
		}
		gitDir = strings.TrimPrefix(line, "gitdir: ")
		if !filepath.IsAbs(gitDir) {
			gitDir = filepath.Join(filepath.Dir(marker), gitDir)
		}
	} else if !info.IsDir() {
		return false, fmt.Errorf("git marker %q is not a regular file or directory", marker)
	}
	if err := requireRegularFile(filepath.Join(gitDir, "HEAD")); err != nil {
		return false, fmt.Errorf("git marker %q lacks a regular HEAD: %w", marker, err)
	}
	if err := requireGitConfig(gitDir); err != nil {
		return false, fmt.Errorf("git marker %q lacks a regular config: %w", marker, err)
	}
	return true, nil
}

func requireGitConfig(gitDir string) error {
	config := filepath.Join(gitDir, "config")
	if err := requireRegularFile(config); err == nil {
		return nil
	}
	commonLine, err := readMarkerLine(filepath.Join(gitDir, "commondir"))
	if err != nil {
		return err
	}
	commonDir := commonLine
	if !filepath.IsAbs(commonDir) {
		commonDir = filepath.Join(gitDir, commonDir)
	}
	return requireRegularFile(filepath.Join(commonDir, "config"))
}

func requireRegularFile(path string) error {
	info, err := os.Stat(path)
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("%q is not a regular file", path)
	}
	return nil
}

func readMarkerLine(path string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer file.Close()
	reader := &io.LimitedReader{R: file, N: maxGitMarkerBytes + 1}
	raw, err := io.ReadAll(reader)
	if err != nil {
		return "", err
	}
	if len(raw) > maxGitMarkerBytes || !utf8.Valid(raw) || strings.ContainsRune(string(raw), '\x00') {
		return "", fmt.Errorf("marker file is not one bounded UTF-8 line")
	}
	line := strings.TrimSuffix(strings.TrimSuffix(string(raw), "\n"), "\r")
	if line == "" || strings.ContainsAny(line, "\r\n") || line != strings.TrimSpace(line) {
		return "", fmt.Errorf("marker file is not one exact nonblank line")
	}
	return line, nil
}
