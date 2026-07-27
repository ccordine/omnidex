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
