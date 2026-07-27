package omni

import (
	"encoding/json"
	"os"
	"path/filepath"
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
	Intent               string             `json:"intent"`
	LikelyFiles          []string           `json:"likely_files,omitempty"`
	RelevantModules      []string           `json:"relevant_modules,omitempty"`
	VerificationCommands []string           `json:"verification_commands,omitempty"`
	KnownRisks           []string           `json:"known_risks,omitempty"`
	Reasons              []string           `json:"reasons,omitempty"`
	FileChunks           []FileContextChunk `json:"file_chunks,omitempty"`
	ContextPolicy        string             `json:"context_policy,omitempty"`
	Confidence           int                `json:"confidence"`
}

type FileContextChunk struct {
	ID         string `json:"id"`
	Path       string `json:"path"`
	SHA256     string `json:"sha256,omitempty"`
	StartLine  int    `json:"start_line"`
	EndLine    int    `json:"end_line"`
	LineCount  int    `json:"line_count"`
	Reason     string `json:"reason,omitempty"`
	Preview    string `json:"preview,omitempty"`
	SedCommand string `json:"sed_command"`
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
