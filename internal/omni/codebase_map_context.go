package omni

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

type FileContextChunkConfig struct {
	ChunkLines       int
	OverlapLines     int
	MaxChunksPerFile int
	MaxTotalChunks   int
	PreviewChars     int
}

func BuildRouteFileContextChunks(cm CodebaseMap, route TaskRoute, terms []string, cfg FileContextChunkConfig) []FileContextChunk {
	cfg = normalizeFileContextChunkConfig(cfg)
	if strings.TrimSpace(cm.Root) == "" || len(route.LikelyFiles) == 0 {
		return nil
	}
	fileByPath := map[string]FileSummary{}
	for _, file := range cm.Files {
		fileByPath[file.Path] = file
	}
	chunks := []FileContextChunk{}
	for _, rel := range route.LikelyFiles {
		if len(chunks) >= cfg.MaxTotalChunks {
			break
		}
		file := fileByPath[rel]
		if strings.TrimSpace(file.Path) == "" || !isCodeContextFile(file.Path) {
			continue
		}
		path := filepath.Join(cm.Root, filepath.FromSlash(file.Path))
		blob, err := os.ReadFile(path)
		if err != nil || looksBinary(blob) {
			continue
		}
		fileChunks := chunkFileForContext(file, string(blob), terms, cfg)
		for _, chunk := range fileChunks {
			chunks = append(chunks, chunk)
			if len(chunks) >= cfg.MaxTotalChunks {
				break
			}
		}
	}
	return chunks
}

func normalizeFileContextChunkConfig(cfg FileContextChunkConfig) FileContextChunkConfig {
	if cfg.ChunkLines <= 0 {
		cfg.ChunkLines = 120
	}
	if cfg.OverlapLines < 0 {
		cfg.OverlapLines = 0
	}
	if cfg.OverlapLines >= cfg.ChunkLines {
		cfg.OverlapLines = cfg.ChunkLines / 10
	}
	if cfg.MaxChunksPerFile <= 0 {
		cfg.MaxChunksPerFile = 2
	}
	if cfg.MaxTotalChunks <= 0 {
		cfg.MaxTotalChunks = 8
	}
	if cfg.PreviewChars <= 0 {
		cfg.PreviewChars = 700
	}
	return cfg
}

func chunkFileForContext(file FileSummary, content string, terms []string, cfg FileContextChunkConfig) []FileContextChunk {
	lines := strings.Split(content, "\n")
	if len(lines) == 0 {
		return nil
	}
	step := cfg.ChunkLines - cfg.OverlapLines
	if step <= 0 {
		step = cfg.ChunkLines
	}
	candidates := []FileContextChunk{}
	for start := 0; start < len(lines); start += step {
		end := minInt(len(lines), start+cfg.ChunkLines)
		if start >= end {
			break
		}
		text := strings.Join(lines[start:end], "\n")
		score := routeScore(strings.ToLower(file.Path+" "+text), terms)
		if score == 0 && start > 0 {
			continue
		}
		startLine := start + 1
		endLine := end
		chunk := FileContextChunk{
			ID:         fmt.Sprintf("%s:%d-%d", file.Path, startLine, endLine),
			Path:       file.Path,
			SHA256:     file.SHA256,
			StartLine:  startLine,
			EndLine:    endLine,
			LineCount:  endLine - startLine + 1,
			Reason:     fileContextChunkReason(score, startLine),
			Preview:    truncateForStructuredContext(trimCodePreview(text), cfg.PreviewChars),
			SedCommand: fmt.Sprintf("sed -n '%d,%dp' %s", startLine, endLine, shellQuoteCodebasePath(file.Path)),
		}
		candidates = append(candidates, chunk)
		if len(candidates) >= cfg.MaxChunksPerFile {
			break
		}
	}
	return candidates
}

func fileContextChunkReason(score, startLine int) string {
	if score > 0 {
		return "chunk text matched task terms"
	}
	if startLine == 1 {
		return "file header chunk for orientation"
	}
	return "route selected file chunk"
}

func trimCodePreview(text string) string {
	lines := strings.Split(text, "\n")
	out := make([]string, 0, minInt(len(lines), 24))
	for _, line := range lines {
		if strings.TrimSpace(line) == "" && len(out) == 0 {
			continue
		}
		out = append(out, line)
		if len(out) >= 24 {
			break
		}
	}
	return strings.TrimSpace(strings.Join(out, "\n"))
}

func isCodeContextFile(path string) bool {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".go", ".js", ".jsx", ".mjs", ".cjs", ".ts", ".tsx", ".css", ".html", ".md", ".json", ".toml", ".yaml", ".yml", ".zig", ".rs", ".php":
		return true
	default:
		return false
	}
}

func looksBinary(blob []byte) bool {
	if len(blob) == 0 {
		return false
	}
	limit := minInt(len(blob), 4096)
	for i := 0; i < limit; i++ {
		if blob[i] == 0 {
			return true
		}
	}
	return false
}

func shellQuoteCodebasePath(value string) string {
	if value == "" {
		return "''"
	}
	return "'" + strings.ReplaceAll(value, "'", "'\\''") + "'"
}

func workspaceIndexRevision(index WorkspaceIndex) string {
	parts := make([]string, 0, len(index.Manifests)+1)
	parts = append(parts, index.Workspace)
	for path, hash := range index.Manifests {
		parts = append(parts, path+"="+hash)
	}
	sort.Strings(parts)
	return hashJoin(parts...)
}

func languageForPath(path string) string {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".go":
		return "Go"
	case ".js", ".jsx", ".mjs", ".cjs":
		return "JavaScript"
	case ".ts", ".tsx":
		return "TypeScript"
	case ".css":
		return "CSS"
	case ".html":
		return "HTML"
	case ".md":
		return "Markdown"
	case ".json":
		return "JSON"
	case ".zig":
		return "Zig"
	case ".rs":
		return "Rust"
	case ".php":
		return "PHP"
	default:
		return ""
	}
}

func moduleForPath(path string) string {
	path = filepath.ToSlash(path)
	parts := strings.Split(path, "/")
	if len(parts) >= 2 && (parts[0] == "internal" || parts[0] == "cmd") {
		return strings.Join(parts[:2], "/")
	}
	if len(parts) >= 2 && (parts[0] == "docs" || parts[0] == "skills") {
		return strings.Join(parts[:2], "/")
	}
	if len(parts) >= 2 && parts[0] == "src" {
		return strings.Join(parts[:2], "/")
	}
	if len(parts) > 1 {
		return parts[0]
	}
	return "."
}

func filePurpose(path string) string {
	lower := strings.ToLower(path)
	switch {
	case strings.Contains(lower, "test"):
		return "tests or validation"
	case strings.Contains(lower, "worksite"):
		return "workspace grounding and operation classification"
	case strings.Contains(lower, "llm_command"):
		return "structured command planning and execution loop"
	case strings.Contains(lower, "recipe"):
		return "workflow recipes and runtime constraints"
	case strings.Contains(lower, "policy"):
		return "command or scope policy"
	case strings.Contains(lower, "memory"):
		return "persistent memory or retrieval"
	case strings.Contains(lower, "trace"):
		return "run trace and timeline summarization"
	case strings.Contains(lower, "evidence"):
		return "evidence ledger and observed results"
	default:
		return "project file"
	}
}

func modulePurpose(path string) string {
	if path == "." {
		return "repository root"
	}
	return "Owns " + strings.ReplaceAll(path, "/", " / ") + " behavior"
}

func tagsForPath(path string) []string {
	lower := strings.ToLower(path)
	tags := []string{"path:" + path, "module:" + moduleForPath(path)}
	for _, pair := range []struct{ needle, tag string }{
		{"scope", "scope"},
		{"drift", "scope_drift"},
		{"worksite", "worksite"},
		{"llm_command", "structured_command_loop"},
		{"loop", "loop"},
		{"progression", "progression"},
		{"recipe", "recipes"},
		{"policy", "policy"},
		{"memory", "memory"},
		{"evidence", "evidence"},
		{"trace", "trace"},
		{"test", "tests"},
	} {
		if strings.Contains(lower, pair.needle) {
			tags = append(tags, pair.tag)
		}
	}
	return dedupeStrings(tags)
}

func manifestKind(path string) string {
	switch filepath.Base(path) {
	case "go.mod":
		return "go_module"
	case "package.json":
		return "node_package"
	default:
		return "manifest"
	}
}

func isEntrypointPath(path string) bool {
	base := filepath.Base(path)
	return base == "main.go" || base == "App.jsx" || base == "App.tsx" || base == "index.html" || base == "main.tsx" || base == "main.jsx"
}

func entrypointKind(path string) string {
	if strings.HasSuffix(path, ".go") {
		return "go"
	}
	if strings.HasSuffix(path, ".html") {
		return "web"
	}
	return "frontend"
}

func isTestPath(path string) bool {
	return strings.HasSuffix(path, "_test.go") || strings.Contains(path, ".test.") || strings.Contains(path, ".spec.")
}

func verificationCommandForPath(path string, index WorkspaceIndex) string {
	if strings.HasSuffix(path, "_test.go") {
		return "go test ./..."
	}
	if hasManifest(index, "package.json") {
		return "npm test"
	}
	return ""
}

func hasManifest(index WorkspaceIndex, path string) bool {
	_, ok := index.Manifests[path]
	return ok
}
