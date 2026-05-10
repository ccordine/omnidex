package ingest

import (
	"archive/zip"
	"bytes"
	"fmt"
	"html"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/ledongthuc/pdf"
)

var (
	whitespaceRE     = regexp.MustCompile(`\s+`)
	srtTimecodeRE    = regexp.MustCompile(`^\d{2}:\d{2}:\d{2}[,\.]\d{3}\s+-->\s+\d{2}:\d{2}:\d{2}[,\.]\d{3}`)
	vttTimecodeRE    = regexp.MustCompile(`^(\d+:)?\d{2}:\d{2}\.\d{3}\s+-->\s+(\d+:)?\d{2}:\d{2}\.\d{3}`)
	docxTextNodeRE   = regexp.MustCompile(`(?is)<w:t[^>]*>(.*?)</w:t>`)
	xmlTagRE         = regexp.MustCompile(`(?is)<[^>]+>`)
	subtitleStyleRE  = regexp.MustCompile(`(?i)\{\\.*?\}|<[^>]+>`)
	numberOnlyLineRE = regexp.MustCompile(`^\d+$`)
)

type ParsedFile struct {
	Path    string
	Format  string
	Content string
}

func ParseFile(path string) (ParsedFile, error) {
	ext := strings.ToLower(filepath.Ext(path))
	switch ext {
	case ".txt", ".md", ".markdown", ".log", ".json", ".yaml", ".yml", ".csv":
		return parseTextLike(path, ext)
	case ".srt":
		return parseSRT(path)
	case ".vtt":
		return parseVTT(path)
	case ".docx":
		return parseDOCX(path)
	case ".pdf":
		return parsePDF(path)
	default:
		return ParsedFile{}, fmt.Errorf("unsupported file type %q", ext)
	}
}

func ChunkText(content string, chunkSize, overlap int) []string {
	content = normalizeWhitespace(content)
	if content == "" {
		return nil
	}
	if chunkSize <= 0 {
		chunkSize = 1800
	}
	if overlap < 0 {
		overlap = 0
	}
	if overlap >= chunkSize {
		overlap = chunkSize / 4
	}

	runes := []rune(content)
	out := make([]string, 0, (len(runes)/chunkSize)+1)

	start := 0
	for start < len(runes) {
		end := start + chunkSize
		if end > len(runes) {
			end = len(runes)
		}

		chunk := strings.TrimSpace(string(runes[start:end]))
		if chunk != "" {
			out = append(out, chunk)
		}

		if end >= len(runes) {
			break
		}
		start = end - overlap
		if start < 0 {
			start = 0
		}
	}

	return out
}

func parseTextLike(path, format string) (ParsedFile, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return ParsedFile{}, err
	}
	return ParsedFile{
		Path:    path,
		Format:  strings.TrimPrefix(format, "."),
		Content: normalizeWhitespace(string(data)),
	}, nil
}

func parseSRT(path string) (ParsedFile, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return ParsedFile{}, err
	}

	lines := strings.Split(string(data), "\n")
	out := make([]string, 0, len(lines))
	for _, line := range lines {
		line = strings.TrimSpace(strings.TrimRight(line, "\r"))
		if line == "" {
			continue
		}
		if numberOnlyLineRE.MatchString(line) {
			continue
		}
		if srtTimecodeRE.MatchString(line) {
			continue
		}
		line = subtitleStyleRE.ReplaceAllString(line, "")
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		out = append(out, line)
	}

	return ParsedFile{
		Path:    path,
		Format:  "srt",
		Content: normalizeWhitespace(strings.Join(out, " ")),
	}, nil
}

func parseVTT(path string) (ParsedFile, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return ParsedFile{}, err
	}

	lines := strings.Split(string(data), "\n")
	out := make([]string, 0, len(lines))
	for _, line := range lines {
		line = strings.TrimSpace(strings.TrimRight(line, "\r"))
		if line == "" {
			continue
		}
		lower := strings.ToLower(line)
		if lower == "webvtt" || strings.HasPrefix(lower, "kind:") || strings.HasPrefix(lower, "language:") {
			continue
		}
		if strings.HasPrefix(lower, "note") {
			continue
		}
		if numberOnlyLineRE.MatchString(line) {
			continue
		}
		if vttTimecodeRE.MatchString(line) {
			continue
		}
		line = subtitleStyleRE.ReplaceAllString(line, "")
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		out = append(out, line)
	}

	return ParsedFile{
		Path:    path,
		Format:  "vtt",
		Content: normalizeWhitespace(strings.Join(out, " ")),
	}, nil
}

func parseDOCX(path string) (ParsedFile, error) {
	zr, err := zip.OpenReader(path)
	if err != nil {
		return ParsedFile{}, err
	}
	defer zr.Close()

	var docXML []byte
	for _, file := range zr.File {
		if file.Name != "word/document.xml" {
			continue
		}
		rc, err := file.Open()
		if err != nil {
			return ParsedFile{}, err
		}
		docXML, err = io.ReadAll(rc)
		rc.Close()
		if err != nil {
			return ParsedFile{}, err
		}
		break
	}

	if len(docXML) == 0 {
		return ParsedFile{}, fmt.Errorf("word/document.xml not found in docx")
	}

	matches := docxTextNodeRE.FindAllSubmatch(docXML, -1)
	out := make([]string, 0, len(matches))
	for _, match := range matches {
		if len(match) < 2 {
			continue
		}
		text := strings.TrimSpace(string(match[1]))
		if text == "" {
			continue
		}
		text = html.UnescapeString(text)
		out = append(out, text)
	}

	if len(out) == 0 {
		raw := xmlTagRE.ReplaceAllString(string(docXML), " ")
		raw = html.UnescapeString(raw)
		out = append(out, raw)
	}

	return ParsedFile{
		Path:    path,
		Format:  "docx",
		Content: normalizeWhitespace(strings.Join(out, " ")),
	}, nil
}

func parsePDF(path string) (ParsedFile, error) {
	f, reader, err := pdf.Open(path)
	if err != nil {
		return ParsedFile{}, err
	}
	defer f.Close()

	plain, err := reader.GetPlainText()
	if err != nil {
		return ParsedFile{}, err
	}

	buf := &bytes.Buffer{}
	if _, err := io.Copy(buf, plain); err != nil {
		return ParsedFile{}, err
	}

	return ParsedFile{
		Path:    path,
		Format:  "pdf",
		Content: normalizeWhitespace(buf.String()),
	}, nil
}

func normalizeWhitespace(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	value = strings.ReplaceAll(value, "\u00a0", " ")
	value = whitespaceRE.ReplaceAllString(value, " ")
	return strings.TrimSpace(value)
}

func InferTagsFromPath(path, format string) []string {
	base := filepath.Base(path)
	ext := strings.TrimPrefix(strings.ToLower(filepath.Ext(base)), ".")
	base = strings.TrimSuffix(base, filepath.Ext(base))
	base = strings.ToLower(base)

	parts := strings.FieldsFunc(base, func(r rune) bool {
		return !(r >= 'a' && r <= 'z') && !(r >= '0' && r <= '9')
	})

	out := make([]string, 0, len(parts)+4)
	seen := map[string]struct{}{
		"reference": {},
	}
	out = append(out, "reference")

	if format != "" {
		tag := strings.ToLower(strings.TrimSpace(format))
		if _, ok := seen[tag]; !ok {
			seen[tag] = struct{}{}
			out = append(out, tag)
		}
	}
	if ext != "" {
		tag := "ext_" + ext
		if _, ok := seen[tag]; !ok {
			seen[tag] = struct{}{}
			out = append(out, tag)
		}
	}

	for _, token := range parts {
		token = strings.TrimSpace(token)
		if token == "" {
			continue
		}
		if _, err := strconv.Atoi(token); err == nil {
			continue
		}
		if len(token) < 3 {
			continue
		}
		if _, ok := seen[token]; ok {
			continue
		}
		seen[token] = struct{}{}
		out = append(out, token)
	}

	return out
}
