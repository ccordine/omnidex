package workspace

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"unicode/utf8"
)

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

func appendKeyFileSnippets(b *strings.Builder, root string, files []string, budget int) error {
	if len(files) == 0 {
		return nil
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
	if len(selected) == 0 || !appendWithinBudget(b, budget, "\nKey file snippets:\n") {
		return nil
	}
	for _, rel := range selected {
		if budget > 0 && budget-b.Len() < 120 {
			appendWithinBudget(b, budget, "... [snippet budget exhausted]\n")
			return nil
		}
		fullPath := filepath.Join(root, rel)
		content, readable, truncated, err := readTextPrefix(fullPath, maxSnippetFileBytes)
		if err != nil {
			return fmt.Errorf("read workspace snippet %q: %w", fullPath, err)
		}
		if !readable {
			appendWithinBudget(b, budget, fmt.Sprintf("## %s\n(binary or non-utf8 content omitted)\n", rel))
			continue
		}
		content = strings.TrimSpace(content)
		if content == "" {
			appendWithinBudget(b, budget, fmt.Sprintf("## %s\n(empty)\n", rel))
			continue
		}
		content = normalizeWhitespace(content)
		if len(content) > maxSnippetChars {
			content = truncateUTF8(content, maxSnippetChars) + "\n...[truncated]"
		} else if truncated {
			content += "\n...[file prefix truncated]"
		}
		section := fmt.Sprintf("## %s\n%s\n", rel, content)
		if !appendWithinBudget(b, budget, section) {
			appendWithinBudget(b, budget, "... [snippet output truncated]\n")
			return nil
		}
	}
	return nil
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
	if remaining < 8 {
		return false
	}
	b.WriteString(truncateUTF8(text, remaining))
	return false
}

func truncateUTF8(value string, maxBytes int) string {
	if maxBytes <= 0 {
		return ""
	}
	if len(value) <= maxBytes {
		return value
	}
	end := maxBytes
	for end > 0 && !utf8.ValidString(value[:end]) {
		end--
	}
	return value[:end]
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

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}
