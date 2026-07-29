package omni

import (
	"strings"
	"time"
)

type CodebaseMap struct {
	WorkspaceID   string              `json:"workspace_id"`
	Root          string              `json:"root"`
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
	Truncated     bool                `json:"truncated,omitempty"`
}

type LanguageSummary struct {
	Language string `json:"language"`
	Files    int    `json:"files"`
	Bytes    int64  `json:"bytes"`
}

type ManifestSummary struct {
	Path string `json:"path"`
	Kind string `json:"kind"`
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
	Confidence       int      `json:"confidence"`
}

type FileSummary struct {
	Path     string   `json:"path"`
	Language string   `json:"language,omitempty"`
	Module   string   `json:"module,omitempty"`
	Purpose  string   `json:"purpose,omitempty"`
	Tags     []string `json:"tags,omitempty"`
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

type CodebaseMapConfig struct {
	MaxFiles int
}

func BuildCodebaseMap(workspace string, cfg CodebaseMapConfig) (CodebaseMap, error) {
	index, err := BuildWorkspaceIndex(workspace, cfg.MaxFiles)
	if err != nil {
		return CodebaseMap{}, err
	}
	return BuildCodebaseMapFromIndex(index), nil
}

func BuildCodebaseMapFromIndex(index WorkspaceIndex) CodebaseMap {
	root := strings.TrimSpace(index.Workspace)
	cm := CodebaseMap{
		WorkspaceID: workspaceHash(root),
		Root:        root,
		GeneratedAt: time.Now().UTC().Format(time.RFC3339),
		Truncated:   index.Truncated,
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
		fs := FileSummary{
			Path: file.Path, Language: lang, Module: modulePath,
			Purpose: filePurpose(file.Path), Tags: tagsForPath(file.Path),
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
	for _, path := range index.Manifests {
		cm.Manifests = append(cm.Manifests, ManifestSummary{Path: path, Kind: manifestKind(path)})
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
