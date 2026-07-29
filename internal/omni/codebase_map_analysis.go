package omni

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

func packageJSONCommandSummaries(workspace string) []CommandSummary {
	blob, err := os.ReadFile(filepath.Join(workspace, "package.json"))
	if err != nil {
		return nil
	}
	var manifest struct {
		Scripts map[string]string `json:"scripts"`
	}
	if json.Unmarshal(blob, &manifest) != nil {
		return nil
	}
	commands := make([]CommandSummary, 0, len(manifest.Scripts))
	for name, command := range manifest.Scripts {
		commands = append(commands, CommandSummary{Name: name, Command: "npm run " + name, Source: "package.json: " + command})
	}
	sort.Slice(commands, func(i, j int) bool { return commands[i].Name < commands[j].Name })
	return commands
}

var goSymbolPattern = regexp.MustCompile(`(?m)^(func|type|const|var)\s+(?:\([^)]+\)\s*)?([A-Za-z_][A-Za-z0-9_]*)`)

func scanWorkspaceSymbols(index WorkspaceIndex) []SymbolSummary {
	symbols := make([]SymbolSummary, 0, len(index.Files))
	for _, file := range index.Files {
		if !strings.HasSuffix(file.Path, ".go") {
			continue
		}
		blob, err := os.ReadFile(filepath.Join(index.Workspace, filepath.FromSlash(file.Path)))
		if err != nil {
			continue
		}
		for _, match := range goSymbolPattern.FindAllSubmatchIndex(blob, -1) {
			symbols = append(symbols, SymbolSummary{
				Name: string(blob[match[4]:match[5]]), Kind: string(blob[match[2]:match[3]]),
				File: file.Path, Line: 1 + strings.Count(string(blob[:match[0]]), "\n"),
				Package: moduleForPath(file.Path), Purpose: filePurpose(file.Path), Tags: tagsForPath(file.Path),
			})
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

func (s SymbolSummary) LanguageLike() string { return languageForPath(s.File) }

func exportedSymbol(name, language string) bool {
	return name != "" && (language != "Go" || name[0] >= 'A' && name[0] <= 'Z')
}

func moduleResponsibilities(path string, files []string) []string {
	responsibilities := make([]string, 0, 2)
	for _, file := range files {
		if isTestPath(file) {
			responsibilities = append(responsibilities, "test coverage")
		}
	}
	if len(responsibilities) == 0 {
		responsibilities = append(responsibilities, "maintain "+path)
	}
	return dedupeStrings(responsibilities)
}

func dependencyEdges(files []FileSummary) []DependencyEdge {
	edges := make([]DependencyEdge, 0, len(files))
	for _, file := range files {
		if file.Module != "" && file.Module != "." {
			edges = append(edges, DependencyEdge{From: file.Module, To: file.Path, Kind: "contains"})
		}
	}
	return edges
}

func inferCodebaseRisks(CodebaseMap) []RiskSummary { return nil }

func sortCodebaseMap(codebase *CodebaseMap) {
	sort.Slice(codebase.Languages, func(i, j int) bool { return codebase.Languages[i].Language < codebase.Languages[j].Language })
	sort.Slice(codebase.Manifests, func(i, j int) bool { return codebase.Manifests[i].Path < codebase.Manifests[j].Path })
	sort.Slice(codebase.Entrypoints, func(i, j int) bool { return codebase.Entrypoints[i].Path < codebase.Entrypoints[j].Path })
	sort.Slice(codebase.Modules, func(i, j int) bool { return codebase.Modules[i].Path < codebase.Modules[j].Path })
	sort.Slice(codebase.Files, func(i, j int) bool { return codebase.Files[i].Path < codebase.Files[j].Path })
	sort.Slice(codebase.Tests, func(i, j int) bool { return codebase.Tests[i].Path < codebase.Tests[j].Path })
}

func hashJoin(values ...string) string { return workspaceHash(strings.Join(values, "|")) }

func limitStrings(values []string, limit int) []string {
	if limit <= 0 || len(values) <= limit {
		return values
	}
	return values[:limit]
}

func dedupeStrings(values []string) []string {
	result := make([]string, 0, len(values))
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, exists := seen[value]; exists {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	return result
}
