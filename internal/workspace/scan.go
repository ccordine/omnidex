package workspace

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"
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

var (
	goSymbolRE     = regexp.MustCompile(`(?m)^\s*func\s+(?:\([^)]*\)\s*)?([A-Za-z_][A-Za-z0-9_]*)\s*\(|^\s*type\s+([A-Za-z_][A-Za-z0-9_]*)\s+struct\b|^\s*type\s+([A-Za-z_][A-Za-z0-9_]*)\s+interface\b|^\s*var\s+([A-Za-z_][A-Za-z0-9_]*)\b|^\s*const\s+([A-Za-z_][A-Za-z0-9_]*)\b`)
	jsSymbolRE     = regexp.MustCompile(`(?m)^\s*(?:export\s+)?(?:async\s+)?function\s+([A-Za-z_$][A-Za-z0-9_$]*)\s*\(|^\s*(?:export\s+)?class\s+([A-Za-z_$][A-Za-z0-9_$]*)\b|^\s*(?:export\s+)?const\s+([A-Za-z_$][A-Za-z0-9_$]*)\s*=|^\s*(?:export\s+)?let\s+([A-Za-z_$][A-Za-z0-9_$]*)\s*=|^\s*(?:export\s+)?var\s+([A-Za-z_$][A-Za-z0-9_$]*)\s*=`)
	phpSymbolRE    = regexp.MustCompile(`(?m)^\s*function\s+([A-Za-z_][A-Za-z0-9_]*)\s*\(|^\s*class\s+([A-Za-z_][A-Za-z0-9_]*)\b|^\s*interface\s+([A-Za-z_][A-Za-z0-9_]*)\b|^\s*trait\s+([A-Za-z_][A-Za-z0-9_]*)\b`)
	pythonSymbolRE = regexp.MustCompile(`(?m)^\s*def\s+([A-Za-z_][A-Za-z0-9_]*)\s*\(|^\s*class\s+([A-Za-z_][A-Za-z0-9_]*)\b`)
	sqlSymbolRE    = regexp.MustCompile(`(?im)\bcreate\s+(?:or\s+replace\s+)?(?:table|view|function|procedure|index)\s+([A-Za-z_][A-Za-z0-9_\.]*)`)
	javaSymbolRE   = regexp.MustCompile(`(?m)^\s*(?:public|protected|private)?\s*(?:static\s+)?class\s+([A-Za-z_][A-Za-z0-9_]*)\b|^\s*(?:public|protected|private)?\s*(?:static\s+)?(?:final\s+)?[A-Za-z0-9_<>,\[\]]+\s+([A-Za-z_][A-Za-z0-9_]*)\s*\(`)
)

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

type scoredFile struct {
	Path     string
	Score    float64
	Language string
	Symbols  []string
}

func New(enabled bool, root string, maxFiles, budget int) *Service {
	if maxFiles < 1 {
		maxFiles = 5000
	}
	if budget < 1 {
		budget = 6000
	}
	return &Service{enabled: enabled, root: strings.TrimSpace(root), maxFiles: maxFiles, budget: budget}
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

func (s *Service) Snapshot() (string, error) {
	if s == nil || !s.enabled {
		return "workspace scan disabled", nil
	}
	if s.root == "" {
		return "workspace scan skipped: WORKSPACE_ROOT not set", nil
	}
	files, err := s.walkFiles()
	if err != nil {
		return "", err
	}
	if len(files) == 0 {
		return fmt.Sprintf("workspace %s: no files found", s.root), nil
	}
	var b strings.Builder
	b.WriteString(fmt.Sprintf("Workspace root: %s\nFiles indexed: %d\n", s.root, len(files)))
	b.WriteString("File tree:\n")
	for _, file := range files {
		line := "- " + file + "\n"
		if !appendWithinBudget(&b, s.budget, line) {
			appendWithinBudget(&b, s.budget, "... [tree truncated]\n")
			break
		}
	}
	appendKeyFileSnippets(&b, s.root, files, s.budget)
	return strings.TrimSpace(b.String()), nil
}

func (s *Service) Research(query string) (ResearchResult, error) {
	result := ResearchResult{Root: s.Root()}
	if s == nil || !s.enabled {
		result.Summary = "workspace scan disabled"
		result.Context = result.Summary
		return result, nil
	}
	if s.root == "" {
		result.Summary = "workspace scan skipped: WORKSPACE_ROOT not set"
		result.Context = result.Summary
		return result, nil
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
		snapshot, err := s.Snapshot()
		if err != nil {
			return result, err
		}
		result.Summary = snapshot
		result.Context = snapshot
		return result, nil
	}
	excerpts := make([]FileExcerpt, 0, minInt(len(ranked), maxResearchFiles))
	languages := map[string]struct{}{}
	for _, candidate := range ranked {
		excerpt, ok := s.loadExcerpt(candidate.Path, tokens, candidate.Score)
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
		snapshot, err := s.Snapshot()
		if err != nil {
			return result, err
		}
		result.Summary = snapshot
		result.Context = snapshot
		return result, nil
	}
	result.Excerpts = excerpts
	result.Languages = sortedKeys(languages)
	result.Summary = buildResearchSummary(result.Root, result.FilesConsidered, excerpts, result.Languages, s.budget)
	result.Context = result.Summary
	return result, nil
}

func (s *Service) walkFiles() ([]string, error) {
	files := make([]string, 0, minInt(s.maxFiles, 1024))
	err := filepath.WalkDir(s.root, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return nil
		}
		name := d.Name()
		if d.IsDir() {
			if strings.HasPrefix(name, ".") && name != "." {
				return fs.SkipDir
			}
			if _, ok := ignoredDirs[name]; ok {
				return fs.SkipDir
			}
			return nil
		}
		rel, err := filepath.Rel(s.root, path)
		if err != nil {
			rel = path
		}
		rel = filepath.ToSlash(rel)
		files = append(files, rel)
		if len(files) >= s.maxFiles {
			return fs.SkipAll
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Strings(files)
	return files, nil
}

func rankFiles(files, tokens []string) []scoredFile {
	if len(files) == 0 {
		return nil
	}
	languageHints := languageHintsFromTokens(tokens)
	scored := make([]scoredFile, 0, len(files))
	for _, file := range files {
		lower := strings.ToLower(file)
		base := strings.ToLower(filepath.Base(lower))
		lang := detectLanguage(file)
		symbols := symbolHintsFromPath(file)
		score := 0.0
		if _, ok := snippetCandidates[base]; ok {
			score += 1.5
		}
		if hint, ok := languageHints[lang]; ok {
			score += hint
		}
		for _, token := range tokens {
			if strings.Contains(lower, token) {
				score += 3.0
			}
			if strings.Contains(base, token) {
				score += 1.5
			}
			for _, symbol := range symbols {
				if strings.Contains(strings.ToLower(symbol), token) {
					score += 1.1
				}
			}
		}
		score += extensionWeight(lower)
		if score <= 0 {
			continue
		}
		scored = append(scored, scoredFile{Path: file, Score: score, Language: lang, Symbols: symbols})
	}
	sort.SliceStable(scored, func(i, j int) bool {
		if scored[i].Score == scored[j].Score {
			return scored[i].Path < scored[j].Path
		}
		return scored[i].Score > scored[j].Score
	})
	return scored
}

func (s *Service) loadExcerpt(rel string, tokens []string, score float64) (FileExcerpt, bool) {
	fullPath := filepath.Join(s.root, rel)
	data, err := os.ReadFile(fullPath)
	if err != nil || len(data) == 0 {
		return FileExcerpt{}, false
	}
	if len(data) > maxSnippetFileBytes {
		data = data[:maxSnippetFileBytes]
	}
	if !utf8.Valid(data) {
		return FileExcerpt{}, false
	}
	language := detectLanguage(rel)
	symbols := extractSymbols(language, string(data))
	raw := strings.ReplaceAll(string(data), "\r\n", "\n")
	lines := strings.Split(raw, "\n")
	start := 0
	reason := "ranked by filename, language, and workspace heuristics"
	for i, line := range lines {
		lower := strings.ToLower(line)
		matched := false
		for _, token := range tokens {
			if strings.Contains(lower, token) {
				if i > excerptLineWindow {
					start = i - excerptLineWindow
				}
				reason = fmt.Sprintf("matched query token %q in file content", token)
				matched = true
				break
			}
			for _, symbol := range symbols {
				if strings.Contains(strings.ToLower(symbol), token) {
					if i > excerptLineWindow {
						start = i - excerptLineWindow
					}
					reason = fmt.Sprintf("matched query token %q against extracted symbol", token)
					matched = true
					break
				}
			}
			if matched {
				break
			}
		}
		if matched {
			break
		}
	}
	end := minInt(len(lines), start+excerptLineWindow*2+6)
	cleaned := make([]string, 0, end-start)
	for _, line := range lines[start:end] {
		line = strings.TrimRight(line, " \t")
		if strings.TrimSpace(line) == "" {
			continue
		}
		cleaned = append(cleaned, line)
	}
	excerpt := strings.TrimSpace(strings.Join(cleaned, "\n"))
	if excerpt == "" {
		return FileExcerpt{}, false
	}
	if len(excerpt) > maxSnippetChars {
		excerpt = excerpt[:maxSnippetChars] + "\n...[truncated]"
	}
	return FileExcerpt{Path: rel, Reason: reason, Excerpt: excerpt, Score: score, Language: language, Symbols: firstN(symbols, 8)}, true
}

func buildResearchSummary(root string, filesConsidered int, excerpts []FileExcerpt, languages []string, budget int) string {
	var b strings.Builder
	b.WriteString(fmt.Sprintf("Workspace root: %s\nFiles considered: %d\n", root, filesConsidered))
	if len(languages) > 0 {
		b.WriteString("Detected languages: " + strings.Join(languages, ", ") + "\n")
	}
	b.WriteString("Relevant file excerpts:\n")
	for _, excerpt := range excerpts {
		symbolLine := ""
		if len(excerpt.Symbols) > 0 {
			symbolLine = "Symbols: " + strings.Join(firstN(excerpt.Symbols, 6), ", ") + "\n"
		}
		section := fmt.Sprintf("## %s\nReason: %s\nScore: %.2f\nLanguage: %s\n%s%s\n\n", excerpt.Path, excerpt.Reason, excerpt.Score, excerpt.Language, symbolLine, excerpt.Excerpt)
		if !appendWithinBudget(&b, budget, section) {
			appendWithinBudget(&b, budget, "... [workspace research truncated]\n")
			break
		}
	}
	return strings.TrimSpace(b.String())
}

func tokenize(value string) []string {
	value = strings.TrimSpace(strings.ToLower(value))
	if value == "" {
		return nil
	}
	fields := strings.FieldsFunc(value, func(r rune) bool {
		return !(unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_' || r == '-' || r == '/')
	})
	seen := map[string]struct{}{}
	out := make([]string, 0, len(fields))
	for _, field := range fields {
		field = strings.Trim(field, "-_/.")
		if len(field) < 3 {
			continue
		}
		if _, ok := seen[field]; ok {
			continue
		}
		seen[field] = struct{}{}
		out = append(out, field)
		if len(out) >= 24 {
			break
		}
	}
	return out
}

func languageHintsFromTokens(tokens []string) map[string]float64 {
	out := map[string]float64{}
	for _, token := range tokens {
		switch token {
		case "go", "golang", "gomod":
			out["go"] += 1.8
		case "php", "laravel", "blade", "artisan", "composer":
			out["php"] += 1.8
		case "javascript", "typescript", "node", "react", "vite", "npm", "pnpm", "yarn", "tsx", "stimulus", "tailwind":
			out["typescript"] += 1.4
			out["javascript"] += 1.4
		case "python", "pytest", "pip":
			out["python"] += 1.8
		case "java", "kotlin", "spring", "gradle", "maven":
			out["java"] += 1.2
			out["kotlin"] += 1.2
		case "sql", "postgres", "migration", "schema", "query":
			out["sql"] += 1.6
		case "shell", "bash", "sh", "script", "docker":
			out["shell"] += 1.0
		}
	}
	return out
}

func extensionWeight(path string) float64 {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".go", ".md", ".txt", ".json", ".yaml", ".yml", ".toml", ".php", ".js", ".jsx", ".ts", ".tsx", ".py", ".sql", ".sh", ".bash", ".kt", ".java":
		return 0.3
	default:
		return 0
	}
}

func detectLanguage(path string) string {
	base := strings.ToLower(filepath.Base(path))
	if base == "dockerfile" || base == "makefile" {
		return "shell"
	}
	switch strings.ToLower(filepath.Ext(path)) {
	case ".go":
		return "go"
	case ".php", ".phtml", ".blade.php":
		return "php"
	case ".js", ".jsx", ".mjs", ".cjs":
		return "javascript"
	case ".ts", ".tsx":
		return "typescript"
	case ".py":
		return "python"
	case ".sql":
		return "sql"
	case ".java":
		return "java"
	case ".kt", ".kts":
		return "kotlin"
	case ".sh", ".bash", ".zsh":
		return "shell"
	case ".json":
		return "json"
	case ".yaml", ".yml":
		return "yaml"
	case ".toml":
		return "toml"
	case ".md":
		return "markdown"
	default:
		return ""
	}
}

func symbolHintsFromPath(path string) []string {
	base := strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
	base = strings.TrimSpace(base)
	if base == "" {
		return nil
	}
	parts := strings.FieldsFunc(base, func(r rune) bool {
		return !(unicode.IsLetter(r) || unicode.IsDigit(r))
	})
	return firstN(parts, 4)
}

func extractSymbols(language, body string) []string {
	var re *regexp.Regexp
	switch language {
	case "go":
		re = goSymbolRE
	case "javascript", "typescript":
		re = jsSymbolRE
	case "php":
		re = phpSymbolRE
	case "python":
		re = pythonSymbolRE
	case "sql":
		re = sqlSymbolRE
	case "java", "kotlin":
		re = javaSymbolRE
	default:
		return nil
	}
	matches := re.FindAllStringSubmatch(body, 24)
	seen := map[string]struct{}{}
	out := make([]string, 0, len(matches))
	for _, match := range matches {
		for _, part := range match[1:] {
			part = strings.TrimSpace(part)
			if part == "" {
				continue
			}
			if _, ok := seen[part]; ok {
				continue
			}
			seen[part] = struct{}{}
			out = append(out, part)
			break
		}
	}
	return out
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func appendKeyFileSnippets(b *strings.Builder, root string, files []string, budget int) {
	if len(files) == 0 {
		return
	}
	selected := make([]string, 0, maxSnippetFiles)
	for _, rel := range files {
		if len(selected) >= maxSnippetFiles {
			break
		}
		name := strings.ToLower(filepath.Base(rel))
		if _, ok := snippetCandidates[name]; !ok {
			continue
		}
		selected = append(selected, rel)
	}
	if len(selected) == 0 {
		return
	}
	if !appendWithinBudget(b, budget, "\nKey file snippets:\n") {
		return
	}
	for _, rel := range selected {
		if budget > 0 && budget-b.Len() < 120 {
			appendWithinBudget(b, budget, "... [snippet budget exhausted]\n")
			return
		}
		fullPath := filepath.Join(root, rel)
		data, err := os.ReadFile(fullPath)
		if err != nil {
			appendWithinBudget(b, budget, fmt.Sprintf("## %s\n(read error: %v)\n", rel, err))
			continue
		}
		if len(data) > maxSnippetFileBytes {
			data = data[:maxSnippetFileBytes]
		}
		if !utf8.Valid(data) {
			appendWithinBudget(b, budget, fmt.Sprintf("## %s\n(binary or non-utf8 content omitted)\n", rel))
			continue
		}
		content := strings.TrimSpace(string(data))
		if content == "" {
			appendWithinBudget(b, budget, fmt.Sprintf("## %s\n(empty)\n", rel))
			continue
		}
		content = normalizeWhitespace(content)
		if len(content) > maxSnippetChars {
			content = content[:maxSnippetChars] + "\n...[truncated]"
		}
		section := fmt.Sprintf("## %s\n%s\n", rel, content)
		if !appendWithinBudget(b, budget, section) {
			appendWithinBudget(b, budget, "... [snippet output truncated]\n")
			return
		}
	}
}

func appendWithinBudget(b *strings.Builder, budget int, text string) bool {
	if b == nil || text == "" {
		return true
	}
	if budget <= 0 || b.Len()+len(text) <= budget {
		b.WriteString(text)
		return true
	}
	remaining := budget - b.Len()
	if remaining <= 0 || remaining < 8 {
		return false
	}
	b.WriteString(text[:remaining])
	return false
}

func normalizeWhitespace(value string) string {
	value = strings.ReplaceAll(value, "\r\n", "\n")
	value = strings.ReplaceAll(value, "\r", "\n")
	lines := strings.Split(value, "\n")
	out := make([]string, 0, len(lines))
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		out = append(out, line)
	}
	return strings.Join(out, "\n")
}

func sortedKeys(values map[string]struct{}) []string {
	if len(values) == 0 {
		return nil
	}
	out := make([]string, 0, len(values))
	for key := range values {
		out = append(out, key)
	}
	sort.Strings(out)
	return out
}

func firstN(items []string, max int) []string {
	if len(items) == 0 {
		return nil
	}
	seen := map[string]struct{}{}
	out := make([]string, 0, minInt(len(items), max))
	for _, item := range items {
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}
		if _, ok := seen[item]; ok {
			continue
		}
		seen[item] = struct{}{}
		out = append(out, item)
		if max > 0 && len(out) >= max {
			break
		}
	}
	return out
}
