package omni

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
)

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
	if taskNeedsFileChunkContext(task) {
		route.ContextPolicy = "chunked_file_context_only"
		route.FileChunks = BuildRouteFileContextChunks(cm, route, terms, FileContextChunkConfig{})
	}
	return route
}

func taskNeedsFileChunkContext(task string) bool {
	lower := strings.ToLower(task)
	for _, term := range []string{
		"edit", "modify", "change", "update", "patch", "fix", "repair", "refactor", "implement", "add ",
		"remove", "delete", "rename", "write", "rewrite", "build", "create", "component", "function",
		"method", "class", "file", "document", "readme", "source", "code", "test",
	} {
		if strings.Contains(lower, term) {
			return true
		}
	}
	return false
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
