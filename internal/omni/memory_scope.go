package omni

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
)

func filterExecutionSessionMemories(memories []SessionMemory, prompt, activeDirectory string, limit int) []SessionMemory {
	if len(memories) == 0 {
		return nil
	}
	if limit <= 0 {
		limit = len(memories)
	}
	workspaceScopedTag := ""
	if strings.TrimSpace(activeDirectory) != "" {
		workspaceScopedTag = "workspace:" + workspaceHash(activeDirectory)
	}
	identities := currentProjectIdentityTokens(activeDirectory, prompt)
	out := []SessionMemory{}
	for i := len(memories) - 1; i >= 0; i-- {
		memory := memories[i]
		record := MemoryRecord{
			Kind:    memory.Kind,
			Content: memory.Content,
			Tags:    memory.Tags,
		}
		if !executionMemoryRecordAllowed(record, workspaceScopedTag) || memoryRecordLooksForeignProject(record, identities, workspaceScopedTag) {
			continue
		}
		memory.Tags = memoryAuthorityTags(memory, identities, workspaceScopedTag)
		if strings.TrimSpace(memory.Kind) == validatedPlaybookKind {
			memory.Content = validatedPlaybookMemorySummary(memory)
		}
		out = append(out, memory)
		if len(out) >= limit {
			break
		}
	}
	for i, j := 0, len(out)-1; i < j; i, j = i+1, j-1 {
		out[i], out[j] = out[j], out[i]
	}
	return out
}

func memoryAuthorityTags(memory SessionMemory, identities map[string]bool, workspaceScopedTag string) []string {
	tags := cleanMemoryTags(memory.Tags)
	tags = append(tags, "advisory-only", "may-create-scope:false")
	if memoryScopedToCurrentWorkspace(tags, workspaceScopedTag) {
		tags = append(tags, "workspace-scoped")
	}
	record := MemoryRecord{Kind: memory.Kind, Content: memory.Content, Tags: tags}
	if memoryRecordLooksForeignProject(record, identities, workspaceScopedTag) {
		tags = append(tags, "foreign-project-memory")
	}
	return cleanMemoryTags(tags)
}

func memoryScopedToCurrentWorkspace(tags []string, workspaceScopedTag string) bool {
	if strings.TrimSpace(workspaceScopedTag) == "" {
		return false
	}
	for _, tag := range cleanMemoryTags(tags) {
		if tag == workspaceScopedTag {
			return true
		}
	}
	return false
}

func sessionMemoryAllowedForPrep(memory SessionMemory, prompt, activeDirectory string) bool {
	filtered := filterExecutionSessionMemories([]SessionMemory{memory}, prompt, activeDirectory, 1)
	return len(filtered) == 1
}

func packageNameTokens(activeDirectory string) map[string]bool {
	out := map[string]bool{}
	if strings.TrimSpace(activeDirectory) == "" {
		return out
	}
	blob, err := os.ReadFile(filepath.Join(activeDirectory, "package.json"))
	if err != nil {
		return out
	}
	var pkg struct {
		Name string `json:"name"`
	}
	if json.Unmarshal(blob, &pkg) != nil {
		return out
	}
	for _, token := range projectIdentityTokensFromText(pkg.Name) {
		out[token] = true
	}
	return out
}

func projectIdentityTokensFromText(text string) []string {
	tokens := []string{}
	for _, token := range strings.FieldsFunc(strings.ToLower(text), func(r rune) bool {
		return !(r >= 'a' && r <= 'z') && !(r >= '0' && r <= '9')
	}) {
		if len(token) >= 3 {
			tokens = append(tokens, token)
		}
	}
	return cleanStringList(tokens)
}

func gitRepoTopLevel(dir string) (string, error) {
	dir = strings.TrimSpace(dir)
	if dir == "" {
		return "", os.ErrNotExist
	}
	abs, err := filepath.Abs(dir)
	if err != nil {
		return "", err
	}
	for {
		if info, err := os.Stat(filepath.Join(abs, ".git")); err == nil && info.IsDir() {
			return abs, nil
		}
		next := filepath.Dir(abs)
		if next == abs {
			return "", os.ErrNotExist
		}
		abs = next
	}
}
