package api

import (
	"fmt"
	"net/http"
	"path/filepath"
	"strings"

	"github.com/gryph/omnidex/internal/hostbridge"
)

func (s *Server) populateScrumModalFileContext(
	r *http.Request,
	projectRoot string,
	relativePath string,
	fileOffset int,
	ctx *scrumModalRenderContext,
) error {
	if r == nil || ctx == nil {
		return fmt.Errorf("request and modal context are required for Scrum file paging")
	}
	filePath := projectRoot
	if relativePath != "" {
		filePath = filepath.Join(projectRoot, filepath.FromSlash(relativePath))
	}
	filePath, err := exactScrumProjectBrowsePath(projectRoot, filePath)
	if err != nil {
		return err
	}
	if fileOffset < 0 || fileOffset > hostbridge.MaxBrowseOffset {
		return fmt.Errorf("Scrum file page offset is outside its accepted range")
	}
	page, err := s.scrumProjectFilePage(r, projectRoot, filePath, fileOffset)
	if err != nil {
		return err
	}
	projectionRoot := page.RequiredRoot
	if strings.TrimSpace(projectionRoot) == "" {
		return fmt.Errorf("Scrum file page did not attest its required project root")
	}
	ctx.FilePath, err = scrumProjectRelativePath(projectionRoot, page.Path)
	if err != nil {
		return err
	}
	if page.Parent != "" {
		if parent, parentErr := scrumProjectRelativePath(projectionRoot, page.Parent); parentErr == nil {
			ctx.FileParent = parent
			ctx.FileHasParent = ctx.FilePath != ""
		}
	}
	ctx.FileOffset = page.Offset
	ctx.FileHasPrevious, ctx.FilePreviousOffset = page.HasPrevious, page.PreviousOffset
	ctx.FileHasMore, ctx.FileNextOffset = page.HasMore, page.NextOffset
	for _, entry := range page.Entries {
		path, relErr := scrumProjectRelativePath(projectionRoot, entry.Path)
		if relErr != nil {
			return fmt.Errorf("paged Scrum file escaped the project root: %w", relErr)
		}
		if entry.IsDir {
			ctx.Dirs = append(ctx.Dirs, path)
		} else {
			ctx.Files = append(ctx.Files, path)
		}
	}
	return nil
}

func scrumProjectRelativePath(projectRoot, target string) (string, error) {
	root, target, err := exactScrumProjectPathPair(projectRoot, target)
	if err != nil {
		return "", err
	}
	rel, err := filepath.Rel(root, target)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("path is outside the Scrum project")
	}
	if rel == "." {
		return "", nil
	}
	return filepath.ToSlash(rel), nil
}

func exactScrumProjectBrowsePath(projectRoot, target string) (string, error) {
	root, target, err := exactScrumProjectPathPair(projectRoot, target)
	if err != nil {
		return "", err
	}
	rel, err := filepath.Rel(root, target)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("Scrum file browse path escaped the project root")
	}
	return target, nil
}

func exactScrumProjectPathPair(projectRoot, target string) (string, string, error) {
	if strings.TrimSpace(projectRoot) == "" {
		return "", "", fmt.Errorf("Scrum project directory is required for file browsing")
	}
	root, err := filepath.Abs(projectRoot)
	if err != nil || target == "" {
		return "", "", fmt.Errorf("Scrum file browse path is invalid")
	}
	target, err = filepath.Abs(target)
	if err != nil {
		return "", "", fmt.Errorf("Scrum file browse path is invalid")
	}
	return filepath.Clean(root), filepath.Clean(target), nil
}
