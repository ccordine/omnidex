package workspace

import (
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"unicode"
)

var (
	goSymbolRE     = regexp.MustCompile(`(?m)^\s*func\s+(?:\([^)]*\)\s*)?([A-Za-z_][A-Za-z0-9_]*)\s*\(|^\s*type\s+([A-Za-z_][A-Za-z0-9_]*)\s+struct\b|^\s*type\s+([A-Za-z_][A-Za-z0-9_]*)\s+interface\b|^\s*var\s+([A-Za-z_][A-Za-z0-9_]*)\b|^\s*const\s+([A-Za-z_][A-Za-z0-9_]*)\b`)
	jsSymbolRE     = regexp.MustCompile(`(?m)^\s*(?:export\s+)?(?:async\s+)?function\s+([A-Za-z_$][A-Za-z0-9_$]*)\s*\(|^\s*(?:export\s+)?class\s+([A-Za-z_$][A-Za-z0-9_$]*)\b|^\s*(?:export\s+)?const\s+([A-Za-z_$][A-Za-z0-9_$]*)\s*=|^\s*(?:export\s+)?let\s+([A-Za-z_$][A-Za-z0-9_$]*)\s*=|^\s*(?:export\s+)?var\s+([A-Za-z_$][A-Za-z0-9_$]*)\s*=`)
	phpSymbolRE    = regexp.MustCompile(`(?m)^\s*function\s+([A-Za-z_][A-Za-z0-9_]*)\s*\(|^\s*class\s+([A-Za-z_][A-Za-z0-9_]*)\b|^\s*interface\s+([A-Za-z_][A-Za-z0-9_]*)\b|^\s*trait\s+([A-Za-z_][A-Za-z0-9_]*)\b`)
	pythonSymbolRE = regexp.MustCompile(`(?m)^\s*def\s+([A-Za-z_][A-Za-z0-9_]*)\s*\(|^\s*class\s+([A-Za-z_][A-Za-z0-9_]*)\b`)
	sqlSymbolRE    = regexp.MustCompile(`(?im)\bcreate\s+(?:or\s+replace\s+)?(?:table|view|function|procedure|index)\s+([A-Za-z_][A-Za-z0-9_\.]*)`)
	javaSymbolRE   = regexp.MustCompile(`(?m)^\s*(?:public|protected|private)?\s*(?:static\s+)?class\s+([A-Za-z_][A-Za-z0-9_]*)\b|^\s*(?:public|protected|private)?\s*(?:static\s+)?(?:final\s+)?[A-Za-z0-9_<>,\[\]]+\s+([A-Za-z_][A-Za-z0-9_]*)\s*\(`)
)

type scoredFile struct {
	Path             string
	Score            float64
	Language         string
	Symbols          []string
	MetadataRelevant bool
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
		metadataRelevant := false
		if _, ok := snippetCandidates[base]; ok {
			score += 1.5
			metadataRelevant = true
		}
		if hint, ok := languageHints[lang]; ok {
			score += hint
			metadataRelevant = true
		}
		for _, token := range tokens {
			if strings.Contains(lower, token) {
				score += 3.0
				metadataRelevant = true
			}
			if strings.Contains(base, token) {
				score += 1.5
				metadataRelevant = true
			}
			for _, symbol := range symbols {
				if strings.Contains(strings.ToLower(symbol), token) {
					score += 1.1
					metadataRelevant = true
				}
			}
		}
		score += extensionWeight(lower)
		if score <= 0 {
			continue
		}
		scored = append(scored, scoredFile{
			Path:             file,
			Score:            score,
			Language:         lang,
			Symbols:          symbols,
			MetadataRelevant: metadataRelevant,
		})
	}
	sort.SliceStable(scored, func(i, j int) bool {
		if scored[i].Score == scored[j].Score {
			return scored[i].Path < scored[j].Path
		}
		return scored[i].Score > scored[j].Score
	})
	return scored
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
		if len(field) < 3 && !relevantShortToken(field) {
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

func relevantShortToken(token string) bool {
	switch token {
	case "ai", "db", "go", "js", "ts", "ui":
		return true
	default:
		return false
	}
}

func languageHintsFromTokens(tokens []string) map[string]float64 {
	out := map[string]float64{}
	for _, token := range tokens {
		switch token {
		case "go", "golang", "gomod":
			out["go"] += 1.8
		case "php", "blade", "artisan", "composer":
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
