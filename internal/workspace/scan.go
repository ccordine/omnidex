package workspace

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

var ignoredDirs = map[string]struct{}{
	".git":         {},
	".idea":        {},
	".vscode":      {},
	"node_modules": {},
	"vendor":       {},
	"dist":         {},
	"build":        {},
	"target":       {},
}

var snippetCandidates = map[string]struct{}{
	"readme.md":           {},
	"readme.txt":          {},
	"go.mod":              {},
	"go.sum":              {},
	"package.json":        {},
	"pnpm-lock.yaml":      {},
	"yarn.lock":           {},
	"composer.json":       {},
	"requirements.txt":    {},
	"pyproject.toml":      {},
	"dockerfile":          {},
	"docker-compose.yml":  {},
	"docker-compose.yaml": {},
	"makefile":            {},
	".env":                {},
	".env.example":        {},
}

const (
	maxSnippetFileBytes = 64 * 1024
	maxSnippetChars     = 1400
	maxSnippetFiles     = 8
	maxResearchFiles    = 12
	excerptLineWindow   = 3
)

type Service struct {
	enabled  bool
	root     string
	maxFiles int
	budget   int
}

type FileExcerpt struct {
	Path     string   `json:"path"`
	Reason   string   `json:"reason,omitempty"`
	Excerpt  string   `json:"excerpt,omitempty"`
	Score    float64  `json:"score,omitempty"`
	Language string   `json:"language,omitempty"`
	Symbols  []string `json:"symbols,omitempty"`
}

type ResearchResult struct {
	Root            string        `json:"root"`
	FilesConsidered int           `json:"files_considered"`
	Excerpts        []FileExcerpt `json:"excerpts,omitempty"`
	Languages       []string      `json:"languages,omitempty"`
	Summary         string        `json:"summary,omitempty"`
	Context         string        `json:"context,omitempty"`
}

func New(enabled bool, root string, maxFiles, budget int) (*Service, error) {
	if maxFiles < 1 {
		return nil, fmt.Errorf("workspace max files must be positive, received %d", maxFiles)
	}
	if budget < 1 {
		return nil, fmt.Errorf("workspace context budget must be positive, received %d", budget)
	}
	return &Service{enabled: enabled, root: strings.TrimSpace(root), maxFiles: maxFiles, budget: budget}, nil
}

func (s *Service) Enabled() bool {
	return s != nil && s.enabled
}

func (s *Service) Root() string {
	if s == nil {
		return ""
	}
	return strings.TrimSpace(s.root)
}

// Scoped binds the scanner to one explicit workspace root. It never substitutes
// the configured root when the requested root is invalid.
func (s *Service) Scoped(root string) (*Service, error) {
	if s == nil || !s.enabled {
		return nil, fmt.Errorf("workspace scan is disabled")
	}
	root = strings.TrimSpace(root)
	if root == "" {
		return nil, fmt.Errorf("workspace root is required")
	}
	abs, err := filepath.Abs(root)
	if err != nil {
		return nil, fmt.Errorf("resolve workspace root %q: %w", root, err)
	}
	info, err := os.Stat(abs)
	if err != nil {
		return nil, fmt.Errorf("inspect workspace root %q: %w", abs, err)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("workspace root %q is not a directory", abs)
	}
	return &Service{enabled: true, root: abs, maxFiles: s.maxFiles, budget: s.budget}, nil
}

func (s *Service) Snapshot() (string, error) {
	if s == nil || !s.enabled {
		return "", fmt.Errorf("workspace scan is disabled")
	}
	if s.root == "" {
		return "", fmt.Errorf("workspace root is required")
	}
	files, err := s.walkFiles()
	if err != nil {
		return "", err
	}
	if len(files) == 0 {
		return "Workspace contains no files.", nil
	}
	var output strings.Builder
	output.WriteString(fmt.Sprintf("Files indexed: %d\n", len(files)))
	output.WriteString("File tree:\n")
	for _, file := range files {
		if !appendWithinBudget(&output, s.budget, "- "+file+"\n") {
			appendWithinBudget(&output, s.budget, "... [tree truncated]\n")
			break
		}
	}
	if err := appendKeyFileSnippets(&output, s.root, files, s.budget); err != nil {
		return "", err
	}
	return strings.TrimSpace(output.String()), nil
}

func (s *Service) Research(query string) (ResearchResult, error) {
	result := ResearchResult{Root: s.Root()}
	if s == nil || !s.enabled {
		return result, fmt.Errorf("workspace scan is disabled")
	}
	if s.root == "" {
		return result, fmt.Errorf("workspace root is required")
	}
	files, err := s.walkFiles()
	if err != nil {
		return result, err
	}
	result.FilesConsidered = len(files)
	if len(files) == 0 {
		result.Summary = fmt.Sprintf("workspace %s: no files found", s.root)
		result.Context = result.Summary
		return result, nil
	}
	tokens := tokenize(query)
	ranked := rankFiles(files, tokens)
	if len(ranked) == 0 {
		return noResearchMatch(result, query), nil
	}
	excerpts := make([]FileExcerpt, 0, minInt(len(ranked), maxResearchFiles))
	languages := map[string]struct{}{}
	for _, candidate := range ranked {
		excerpt, ok, err := s.loadExcerpt(candidate.Path, tokens, candidate.Score, candidate.MetadataRelevant)
		if err != nil {
			return result, err
		}
		if !ok {
			continue
		}
		excerpts = append(excerpts, excerpt)
		if excerpt.Language != "" {
			languages[excerpt.Language] = struct{}{}
		}
		if len(excerpts) >= maxResearchFiles {
			break
		}
	}
	if len(excerpts) == 0 {
		return noResearchMatch(result, query), nil
	}
	result.Excerpts = excerpts
	result.Languages = sortedKeys(languages)
	result.Summary = buildResearchSummary(result.Root, result.FilesConsidered, excerpts, result.Languages, s.budget)
	result.Context = result.Summary
	return result, nil
}

func noResearchMatch(result ResearchResult, query string) ResearchResult {
	result.Summary = fmt.Sprintf("No workspace files matched query %q among %d files.", strings.TrimSpace(query), result.FilesConsidered)
	result.Context = result.Summary
	return result
}

func (s *Service) walkFiles() ([]string, error) {
	files := make([]string, 0, minInt(s.maxFiles, 1024))
	err := filepath.WalkDir(s.root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return fmt.Errorf("walk workspace path %q: %w", path, walkErr)
		}
		name := entry.Name()
		if entry.IsDir() {
			if strings.HasPrefix(name, ".") && name != "." {
				return fs.SkipDir
			}
			if _, ignored := ignoredDirs[name]; ignored {
				return fs.SkipDir
			}
			return nil
		}
		relative, err := filepath.Rel(s.root, path)
		if err != nil {
			return fmt.Errorf("resolve workspace-relative path for %q: %w", path, err)
		}
		files = append(files, filepath.ToSlash(relative))
		if len(files) >= s.maxFiles {
			return fs.SkipAll
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("scan workspace %q: %w", s.root, err)
	}
	sort.Strings(files)
	return files, nil
}
