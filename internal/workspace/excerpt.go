package workspace

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"unicode/utf8"
)

func (s *Service) loadExcerpt(rel string, tokens []string, score float64, metadataRelevant bool) (FileExcerpt, bool, error) {
	fullPath := filepath.Join(s.root, rel)
	body, readable, truncated, err := readTextPrefix(fullPath, maxSnippetFileBytes)
	if err != nil {
		return FileExcerpt{}, false, fmt.Errorf("read workspace excerpt %q: %w", fullPath, err)
	}
	if !readable || body == "" {
		return FileExcerpt{}, false, nil
	}
	language := detectLanguage(rel)
	symbols := extractSymbols(language, body)
	raw := strings.ReplaceAll(body, "\r\n", "\n")
	lines := strings.Split(raw, "\n")
	start := 0
	reason := "ranked by filename, language, and workspace heuristics"
	contentMatched := false
	for i, line := range lines {
		lower := strings.ToLower(line)
		matched := false
		for _, token := range tokens {
			if strings.Contains(lower, token) {
				if i > excerptLineWindow {
					start = i - excerptLineWindow
				}
				reason = fmt.Sprintf("matched query token %q in file content", token)
				contentMatched = true
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
					contentMatched = true
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
	if !metadataRelevant && !contentMatched {
		return FileExcerpt{}, false, nil
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
		return FileExcerpt{}, false, nil
	}
	if len(excerpt) > maxSnippetChars {
		excerpt = truncateUTF8(excerpt, maxSnippetChars) + "\n...[truncated]"
	} else if truncated {
		excerpt += "\n...[file prefix truncated]"
	}
	return FileExcerpt{Path: rel, Reason: reason, Excerpt: excerpt, Score: score, Language: language, Symbols: firstN(symbols, 8)}, true, nil
}

func readTextPrefix(path string, maxBytes int) (string, bool, bool, error) {
	if maxBytes < 1 {
		return "", false, false, fmt.Errorf("text prefix limit must be positive, received %d", maxBytes)
	}
	file, err := os.Open(path)
	if err != nil {
		return "", false, false, err
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, int64(maxBytes+utf8.UTFMax)))
	if err != nil {
		return "", false, false, err
	}
	truncated := len(data) > maxBytes
	if truncated {
		data = data[:maxBytes]
		for removed := 0; removed < utf8.UTFMax && !utf8.Valid(data); removed++ {
			data = data[:len(data)-1]
		}
	}
	if !utf8.Valid(data) {
		return "", false, truncated, nil
	}
	return string(data), true, truncated, nil
}
