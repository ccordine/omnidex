package omni

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
)

type CodebaseMap struct {
	Version       string              `json:"version"`
	WorkspaceID   string              `json:"workspace_id"`
	Root          string              `json:"root"`
	Revision      string              `json:"revision,omitempty"`
	GeneratedAt   string              `json:"generated_at"`
	Languages     []LanguageSummary   `json:"languages,omitempty"`
	Manifests     []ManifestSummary   `json:"manifests,omitempty"`
	Entrypoints   []EntrypointSummary `json:"entrypoints,omitempty"`
	Modules       []ModuleSummary     `json:"modules,omitempty"`
	Files         []FileSummary       `json:"files,omitempty"`
	Symbols       []SymbolSummary     `json:"symbols,omitempty"`
	Dependencies  []DependencyEdge    `json:"dependencies,omitempty"`
	Tests         []TestSummary       `json:"tests,omitempty"`
	Commands      []CommandSummary    `json:"commands,omitempty"`
	Risks         []RiskSummary       `json:"risks,omitempty"`
	OpenQuestions []string            `json:"open_questions,omitempty"`
}

type LanguageSummary struct {
	Language string `json:"language"`
	Files    int    `json:"files"`
	Bytes    int64  `json:"bytes"`
}

type ManifestSummary struct {
	Path   string `json:"path"`
	SHA256 string `json:"sha256"`
	Kind   string `json:"kind"`
}

type EntrypointSummary struct {
	Path   string `json:"path"`
	Kind   string `json:"kind"`
	Reason string `json:"reason"`
}

type ModuleSummary struct {
	Path             string   `json:"path"`
	Package          string   `json:"package,omitempty"`
	Purpose          string   `json:"purpose"`
	Responsibilities []string `json:"responsibilities,omitempty"`
	ImportantFiles   []string `json:"important_files,omitempty"`
	PublicSymbols    []string `json:"public_symbols,omitempty"`
	DependsOn        []string `json:"depends_on,omitempty"`
	UsedBy           []string `json:"used_by,omitempty"`
	Tests            []string `json:"tests,omitempty"`
	LastHash         string   `json:"last_hash,omitempty"`
	Confidence       int      `json:"confidence"`
	Stale            bool     `json:"stale,omitempty"`
}

type FileSummary struct {
	Path                    string   `json:"path"`
	Language                string   `json:"language,omitempty"`
	Module                  string   `json:"module,omitempty"`
	Purpose                 string   `json:"purpose,omitempty"`
	SHA256                  string   `json:"sha256"`
	SummaryGeneratedForHash string   `json:"summary_generated_for_hash"`
	Stale                   bool     `json:"stale"`
	Tags                    []string `json:"tags,omitempty"`
}

type SymbolSummary struct {
	Name    string   `json:"name"`
	Kind    string   `json:"kind"`
	File    string   `json:"file"`
	Line    int      `json:"line,omitempty"`
	Package string   `json:"package,omitempty"`
	Purpose string   `json:"purpose,omitempty"`
	Tags    []string `json:"tags,omitempty"`
}

type DependencyEdge struct {
	From string `json:"from"`
	To   string `json:"to"`
	Kind string `json:"kind"`
}

type TestSummary struct {
	Path    string   `json:"path"`
	Module  string   `json:"module,omitempty"`
	Command string   `json:"command,omitempty"`
	Tags    []string `json:"tags,omitempty"`
}

type CommandSummary struct {
	Name    string `json:"name"`
	Command string `json:"command"`
	Source  string `json:"source"`
}

type RiskSummary struct {
	Area   string `json:"area"`
	Risk   string `json:"risk"`
	Reason string `json:"reason,omitempty"`
}

type TaskRoute struct {
	Intent               string   `json:"intent"`
	LikelyFiles          []string `json:"likely_files,omitempty"`
	RelevantModules      []string `json:"relevant_modules,omitempty"`
	VerificationCommands []string `json:"verification_commands,omitempty"`
	KnownRisks           []string `json:"known_risks,omitempty"`
	Reasons              []string `json:"reasons,omitempty"`
	Confidence           int      `json:"confidence"`
}

type CodebaseMapConfig struct {
	MaxFiles     int
	PreviousPath string
}

type CodebaseExpertiseResult struct {
	Map            CodebaseMap    `json:"map"`
	StoredMemories []MemoryRecord `json:"stored_memories,omitempty"`
	StoredCount    int            `json:"stored_count,omitempty"`
}

func BuildCodebaseMap(workspace string, cfg CodebaseMapConfig) (CodebaseMap, error) {
	index, err := BuildWorkspaceIndex(workspace, cfg.MaxFiles)
	if err != nil {
		return CodebaseMap{}, err
	}
	var previous CodebaseMap
	if strings.TrimSpace(cfg.PreviousPath) != "" {
		previous, _ = ReadCodebaseMap(cfg.PreviousPath)
	}
	return BuildCodebaseMapFromIndex(index, previous), nil
}

func UpdateCodebaseMap(workspace, existingPath string, cfg CodebaseMapConfig) (CodebaseMap, error) {
	indexPath := filepath.Join(strings.TrimSpace(workspace), ".omni", "index.json")
	if strings.TrimSpace(workspace) == "" {
		workspace = workspacePathOrCurrentDir()
		indexPath = filepath.Join(workspace, ".omni", "index.json")
	}
	if strings.TrimSpace(existingPath) == "" {
		existingPath = DefaultCodebaseMapPath(workspace)
	}
	index, err := UpdateWorkspaceIndex(workspace, indexPath, cfg.MaxFiles)
	if err != nil {
		return CodebaseMap{}, err
	}
	previous, _ := ReadCodebaseMap(existingPath)
	return BuildCodebaseMapFromIndex(index, previous), nil
}

func BuildCodebaseMapFromIndex(index WorkspaceIndex, previous CodebaseMap) CodebaseMap {
	root := strings.TrimSpace(index.Workspace)
	cm := CodebaseMap{
		Version:     "1.0",
		WorkspaceID: workspaceHash(root),
		Root:        root,
		Revision:    workspaceIndexRevision(index),
		GeneratedAt: time.Now().UTC().Format(time.RFC3339),
	}
	previousFiles := map[string]FileSummary{}
	for _, file := range previous.Files {
		previousFiles[file.Path] = file
	}
	languages := map[string]LanguageSummary{}
	modules := map[string]*ModuleSummary{}
	for _, file := range index.Files {
		lang := languageForPath(file.Path)
		if lang != "" {
			summary := languages[lang]
			summary.Language = lang
			summary.Files++
			summary.Bytes += file.Size
			languages[lang] = summary
		}
		modulePath := moduleForPath(file.Path)
		module := modules[modulePath]
		if module == nil {
			module = &ModuleSummary{Path: modulePath, Purpose: modulePurpose(modulePath), Confidence: 60}
			modules[modulePath] = module
		}
		module.ImportantFiles = append(module.ImportantFiles, file.Path)
		module.LastHash = hashJoin(module.LastHash, file.SHA256)
		fs := FileSummary{
			Path:                    file.Path,
			Language:                lang,
			Module:                  modulePath,
			Purpose:                 filePurpose(file.Path),
			SHA256:                  file.SHA256,
			SummaryGeneratedForHash: file.SHA256,
			Tags:                    tagsForPath(file.Path),
		}
		if prev, ok := previousFiles[file.Path]; ok && prev.SummaryGeneratedForHash != "" && prev.SummaryGeneratedForHash != file.SHA256 {
			fs.Stale = true
		}
		cm.Files = append(cm.Files, fs)
		if isEntrypointPath(file.Path) {
			cm.Entrypoints = append(cm.Entrypoints, EntrypointSummary{Path: file.Path, Kind: entrypointKind(file.Path), Reason: "recognized entrypoint path"})
		}
		if isTestPath(file.Path) {
			test := TestSummary{Path: file.Path, Module: modulePath, Tags: tagsForPath(file.Path)}
			test.Command = verificationCommandForPath(file.Path, index)
			cm.Tests = append(cm.Tests, test)
			module.Tests = append(module.Tests, file.Path)
		}
	}
	for path, sum := range index.Manifests {
		cm.Manifests = append(cm.Manifests, ManifestSummary{Path: path, SHA256: sum, Kind: manifestKind(path)})
	}
	for _, manifest := range cm.Manifests {
		if manifest.Path == "package.json" {
			cm.Commands = append(cm.Commands, packageJSONCommandSummaries(index.Workspace)...)
		}
	}
	cm.Symbols = scanWorkspaceSymbols(index)
	for _, symbol := range cm.Symbols {
		module := modules[moduleForPath(symbol.File)]
		if module != nil && exportedSymbol(symbol.Name, symbol.LanguageLike()) {
			module.PublicSymbols = append(module.PublicSymbols, symbol.Name)
		}
	}
	for _, module := range modules {
		module.ImportantFiles = limitStrings(dedupeStrings(module.ImportantFiles), 20)
		module.PublicSymbols = limitStrings(dedupeStrings(module.PublicSymbols), 30)
		module.Tests = dedupeStrings(module.Tests)
		module.Responsibilities = moduleResponsibilities(module.Path, module.ImportantFiles)
		cm.Modules = append(cm.Modules, *module)
	}
	for _, summary := range languages {
		cm.Languages = append(cm.Languages, summary)
	}
	cm.Dependencies = dependencyEdges(cm.Files)
	cm.Risks = inferCodebaseRisks(cm)
	sortCodebaseMap(&cm)
	return cm
}

func WriteCodebaseMap(cm CodebaseMap, outputPath string) error {
	if strings.TrimSpace(outputPath) == "" {
		outputPath = DefaultCodebaseMapPath(cm.Root)
	}
	if err := os.MkdirAll(filepath.Dir(outputPath), 0o755); err != nil {
		return err
	}
	blob, err := json.MarshalIndent(cm, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(outputPath, append(blob, '\n'), 0o644)
}

func ReadCodebaseMap(path string) (CodebaseMap, error) {
	blob, err := os.ReadFile(path)
	if err != nil {
		return CodebaseMap{}, err
	}
	var cm CodebaseMap
	if err := json.Unmarshal(blob, &cm); err != nil {
		return CodebaseMap{}, err
	}
	return cm, nil
}

func DefaultCodebaseMapPath(workspace string) string {
	if strings.TrimSpace(workspace) == "" {
		workspace = workspacePathOrCurrentDir()
	}
	return filepath.Join(workspace, ".omni", "codebase-map.json")
}

func RouteTaskWithCodebaseMap(cm CodebaseMap, task string) TaskRoute {
	task = strings.TrimSpace(task)
	terms := routeTerms(task)
	route := TaskRoute{Intent: task, Confidence: 35}
	scoreByFile := map[string]int{}
	reasonByFile := map[string]string{}
	moduleScore := map[string]int{}
	for _, file := range cm.Files {
		haystack := strings.ToLower(strings.Join(append([]string{file.Path, file.Purpose, file.Module}, file.Tags...), " "))
		score := routeScore(haystack, terms)
		if score == 0 {
			continue
		}
		scoreByFile[file.Path] += score * 3
		reasonByFile[file.Path] = fmt.Sprintf("%s matched task terms", file.Path)
		moduleScore[file.Module] += score
	}
	for _, symbol := range cm.Symbols {
		haystack := strings.ToLower(symbol.Name + " " + symbol.File + " " + symbol.Kind + " " + strings.Join(symbol.Tags, " "))
		score := routeScore(haystack, terms)
		if score == 0 {
			continue
		}
		scoreByFile[symbol.File] += score
		reasonByFile[symbol.File] = fmt.Sprintf("symbol %s matched task terms", symbol.Name)
		moduleScore[moduleForPath(symbol.File)] += score
	}
	for _, risk := range cm.Risks {
		haystack := strings.ToLower(risk.Area + " " + risk.Risk + " " + risk.Reason)
		if routeScore(haystack, terms) > 0 {
			route.KnownRisks = append(route.KnownRisks, risk.Risk)
		}
	}
	route.LikelyFiles = topScoredKeys(scoreByFile, 8)
	route.RelevantModules = topScoredKeys(moduleScore, 5)
	for _, file := range route.LikelyFiles {
		if reason := reasonByFile[file]; reason != "" {
			route.Reasons = append(route.Reasons, reason)
		}
	}
	route.VerificationCommands = verificationCommandsForRoute(cm, route)
	if len(route.LikelyFiles) > 0 {
		route.Confidence = minInt(90, 45+len(route.LikelyFiles)*5)
	}
	return route
}

func QueryCodebaseMap(cm CodebaseMap, question string) map[string]any {
	route := RouteTaskWithCodebaseMap(cm, question)
	answer := "No strong route found. Inspect the workspace index or rebuild the codebase map."
	if len(route.LikelyFiles) > 0 {
		answer = "Likely relevant files: " + strings.Join(route.LikelyFiles, ", ")
	}
	return map[string]any{
		"answer": answer,
		"route":  route,
	}
}

func BuildCodebaseExpertiseOmnibus(ctx context.Context, workspace, subject string, index WorkspaceIndex, memory *PGMemoryStore, cfg ExpertiseResearchConfig) (CodebaseExpertiseResult, error) {
	_ = ctx
	if memory == nil {
		return CodebaseExpertiseResult{}, fmt.Errorf("memory store is required")
	}
	cm := BuildCodebaseMapFromIndex(index, CodebaseMap{})
	result := CodebaseExpertiseResult{Map: cm}
	content := formatCodebaseOmnibusMemory(cm, subject)
	tags := []string{"expertise", "workspace:" + cm.WorkspaceID, "codebase:" + filepath.Base(cm.Root), "expertise:workspace:" + filepath.Base(cm.Root) + ":" + slugTag(subject)}
	record, err := memory.AddMemory(ctx, defaultExpertiseAgentID, defaultExpertiseKind, content, tags)
	if err != nil {
		return result, err
	}
	result.StoredMemories = append(result.StoredMemories, record)
	result.StoredCount = 1
	return result, nil
}

func LoadCodebaseTaskRoute(workspace, task string) (TaskRoute, bool) {
	cm, err := ReadCodebaseMap(DefaultCodebaseMapPath(workspace))
	if err != nil || len(cm.Files) == 0 {
		return TaskRoute{}, false
	}
	return RouteTaskWithCodebaseMap(cm, task), true
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

func packageJSONCommandSummaries(workspace string) []CommandSummary {
	blob, err := os.ReadFile(filepath.Join(workspace, "package.json"))
	if err != nil {
		return nil
	}
	var pkg struct {
		Scripts map[string]string `json:"scripts"`
	}
	if json.Unmarshal(blob, &pkg) != nil {
		return nil
	}
	out := []CommandSummary{}
	for name, command := range pkg.Scripts {
		out = append(out, CommandSummary{Name: name, Command: "npm run " + name, Source: "package.json: " + command})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

var goSymbolRE = regexp.MustCompile(`(?m)^(func|type|const|var)\s+(?:\([^)]+\)\s*)?([A-Za-z_][A-Za-z0-9_]*)`)

func scanWorkspaceSymbols(index WorkspaceIndex) []SymbolSummary {
	symbols := []SymbolSummary{}
	for _, file := range index.Files {
		if !strings.HasSuffix(file.Path, ".go") {
			continue
		}
		blob, err := os.ReadFile(filepath.Join(index.Workspace, filepath.FromSlash(file.Path)))
		if err != nil {
			continue
		}
		matches := goSymbolRE.FindAllSubmatchIndex(blob, -1)
		for _, match := range matches {
			kind := string(blob[match[2]:match[3]])
			name := string(blob[match[4]:match[5]])
			line := 1 + strings.Count(string(blob[:match[0]]), "\n")
			symbols = append(symbols, SymbolSummary{Name: name, Kind: kind, File: file.Path, Line: line, Package: moduleForPath(file.Path), Purpose: filePurpose(file.Path), Tags: tagsForPath(file.Path)})
		}
	}
	sort.Slice(symbols, func(i, j int) bool {
		if symbols[i].File == symbols[j].File {
			return symbols[i].Line < symbols[j].Line
		}
		return symbols[i].File < symbols[j].File
	})
	return symbols
}

func (s SymbolSummary) LanguageLike() string {
	return languageForPath(s.File)
}

func exportedSymbol(name, lang string) bool {
	if name == "" {
		return false
	}
	if lang == "Go" {
		r := rune(name[0])
		return r >= 'A' && r <= 'Z'
	}
	return true
}

func moduleResponsibilities(path string, files []string) []string {
	resp := []string{}
	for _, file := range files {
		for _, tag := range tagsForPath(file) {
			switch tag {
			case "structured_command_loop":
				resp = append(resp, "structured command planning and execution")
			case "worksite":
				resp = append(resp, "workspace grounding")
			case "policy":
				resp = append(resp, "command and scope policy")
			case "memory":
				resp = append(resp, "memory persistence and recall")
			case "tests":
				resp = append(resp, "test coverage")
			}
		}
	}
	if len(resp) == 0 {
		resp = append(resp, "maintain "+path)
	}
	return dedupeStrings(resp)
}

func dependencyEdges(files []FileSummary) []DependencyEdge {
	edges := []DependencyEdge{}
	for _, file := range files {
		if file.Module != "" && file.Module != "." {
			edges = append(edges, DependencyEdge{From: file.Module, To: file.Path, Kind: "contains"})
		}
	}
	return edges
}

func inferCodebaseRisks(cm CodebaseMap) []RiskSummary {
	risks := []RiskSummary{}
	for _, file := range cm.Files {
		for _, tag := range file.Tags {
			switch tag {
			case "structured_command_loop":
				risks = append(risks, RiskSummary{Area: file.Path, Risk: "Planner loop changes can alter task execution control flow.", Reason: "structured command runtime"})
			case "policy":
				risks = append(risks, RiskSummary{Area: file.Path, Risk: "Policy changes can broaden or narrow execution scope.", Reason: "policy-tagged file"})
			}
		}
	}
	return risks
}

func routeTerms(task string) []string {
	task = strings.ToLower(task)
	terms := strings.FieldsFunc(task, func(r rune) bool {
		return !(r >= 'a' && r <= 'z' || r >= '0' && r <= '9' || r == '_')
	})
	expanded := append([]string{}, terms...)
	for _, term := range terms {
		switch term {
		case "scope":
			expanded = append(expanded, "worksite", "policy", "drift")
		case "loop", "repeated", "repeat":
			expanded = append(expanded, "llm_command", "progression", "progression_gate", "trace")
		case "recovery", "recover":
			expanded = append(expanded, "progression", "progression_gate", "failure", "loop")
		case "memory":
			expanded = append(expanded, "pgsql_memory", "session_memory", "expertise")
		case "recipe", "recipes":
			expanded = append(expanded, "recipe")
		}
	}
	return dedupeStrings(expanded)
}

func routeScore(haystack string, terms []string) int {
	score := 0
	for _, term := range terms {
		if len(term) < 3 {
			continue
		}
		if strings.Contains(haystack, term) {
			score++
		}
	}
	return score
}

func topScoredKeys(scores map[string]int, limit int) []string {
	keys := make([]string, 0, len(scores))
	for key, score := range scores {
		if strings.TrimSpace(key) != "" && score > 0 {
			keys = append(keys, key)
		}
	}
	sort.Slice(keys, func(i, j int) bool {
		if scores[keys[i]] == scores[keys[j]] {
			return keys[i] < keys[j]
		}
		return scores[keys[i]] > scores[keys[j]]
	})
	return limitStrings(keys, limit)
}

func verificationCommandsForRoute(cm CodebaseMap, route TaskRoute) []string {
	commands := []string{}
	for _, file := range route.LikelyFiles {
		for _, test := range cm.Tests {
			if test.Module == moduleForPath(file) && test.Command != "" {
				commands = append(commands, test.Command)
			}
		}
	}
	for _, command := range cm.Commands {
		if command.Name == "test" || command.Name == "build" {
			commands = append(commands, command.Command)
		}
	}
	return limitStrings(dedupeStrings(commands), 5)
}

func formatCodebaseOmnibusMemory(cm CodebaseMap, subject string) string {
	route := RouteTaskWithCodebaseMap(cm, subject)
	return strings.Join([]string{
		"CODEBASE_EXPERTISE",
		"subject=" + strings.TrimSpace(subject),
		"workspace=" + cm.Root,
		"revision=" + cm.Revision,
		"likely_files=" + strings.Join(route.LikelyFiles, ","),
		"modules=" + strings.Join(route.RelevantModules, ","),
		"verification=" + strings.Join(route.VerificationCommands, ","),
	}, "\n")
}

func sortCodebaseMap(cm *CodebaseMap) {
	sort.Slice(cm.Languages, func(i, j int) bool { return cm.Languages[i].Language < cm.Languages[j].Language })
	sort.Slice(cm.Manifests, func(i, j int) bool { return cm.Manifests[i].Path < cm.Manifests[j].Path })
	sort.Slice(cm.Entrypoints, func(i, j int) bool { return cm.Entrypoints[i].Path < cm.Entrypoints[j].Path })
	sort.Slice(cm.Modules, func(i, j int) bool { return cm.Modules[i].Path < cm.Modules[j].Path })
	sort.Slice(cm.Files, func(i, j int) bool { return cm.Files[i].Path < cm.Files[j].Path })
	sort.Slice(cm.Tests, func(i, j int) bool { return cm.Tests[i].Path < cm.Tests[j].Path })
	sort.Slice(cm.Risks, func(i, j int) bool { return cm.Risks[i].Area < cm.Risks[j].Area })
}

func hashJoin(values ...string) string {
	return workspaceHash(strings.Join(values, "|"))
}

func limitStrings(values []string, limit int) []string {
	if limit <= 0 || len(values) <= limit {
		return values
	}
	return values[:limit]
}

func slugTag(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	value = regexp.MustCompile(`[^a-z0-9]+`).ReplaceAllString(value, "-")
	return strings.Trim(value, "-")
}
