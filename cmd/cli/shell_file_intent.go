package main

import (
	"os"
	"path/filepath"
	"strings"
	"time"
)

func parseRepoWalkthroughIntent(lower string) bool {
	phrases := []string{
		"walk me through current changes",
		"walk through current changes",
		"walk me through changes",
		"walk through changes",
		"where did i leave off",
		"where i left off",
		"pick back up",
		"resume this project",
		"show repo changes",
		"show repository changes",
		"show git changes",
		"summarize repo changes",
		"summarize repository changes",
		"what changed in this repo",
		"what changed in this repository",
		"what files changed recently",
		"chronological order of changed files",
		"changed files chronological",
	}
	if containsAnyPhrase(lower, phrases) {
		return true
	}
	if containsAnyPhrase(lower, []string{"git status", "repo status", "repository status"}) &&
		containsAnyPhrase(lower, []string{"walk", "explain", "summarize", "chronological", "recent"}) {
		return true
	}
	return false
}

func parseDoItIntent(lower string, state *localShellState) (string, bool) {
	if state == nil {
		return "", false
	}
	if strings.TrimSpace(state.LastSuggestedCommand) == "" {
		return "", false
	}
	if !state.LastSuggestedAt.IsZero() && time.Since(state.LastSuggestedAt) > localShellSuggestionTTL {
		return "", false
	}
	doItPhrases := []string{
		"do it for me",
		"please do it",
		"do that for me",
		"go ahead and do it",
		"go ahead",
		"run it",
	}
	if !containsAnyPhrase(lower, doItPhrases) {
		return "", false
	}
	return state.LastSuggestedCommand, true
}

func parseCreateFileIntent(clean, lower string) (string, bool) {
	commonTestFilePhrases := []string{
		"make me a test file",
		"make a test file",
		"create a test file",
		"create me a test file",
		"touch test file",
		"create test file",
	}
	if containsAnyPhrase(lower, commonTestFilePhrases) {
		return "test", true
	}

	if matches := shellCreateFileNestedPattern.FindStringSubmatch(clean); len(matches) == 3 {
		container := cleanShellToken(matches[1])
		name := cleanShellToken(matches[2])
		if container != "" && name != "" {
			if strings.ContainsAny(container, `\/`) || filepath.Ext(container) != "" {
				return name, true
			}
			return filepath.Join(container, name), true
		}
	}

	if matches := shellCreateFilePattern.FindStringSubmatch(clean); len(matches) == 2 {
		name := cleanShellToken(matches[1])
		if name != "" {
			return name, true
		}
	}
	if matches := shellTypedFilePattern.FindStringSubmatch(clean); len(matches) == 3 {
		name := cleanShellToken(matches[1])
		ext := normalizeTypedFileExtension(matches[2])
		if name != "" && ext != "" {
			if strings.Contains(filepath.Base(name), ".") {
				return filepath.Clean(name), true
			}
			return filepath.Clean(name + "." + ext), true
		}
	}
	if matches := shellCreateFileAltPattern.FindStringSubmatch(clean); len(matches) == 2 {
		name := cleanShellToken(matches[1])
		if name != "" {
			return name, true
		}
	}
	if containsAnyPhrase(lower, []string{"create", "make", "touch"}) {
		matches := shellFilenameTokenPattern.FindAllString(clean, -1)
		for i := len(matches) - 1; i >= 0; i-- {
			name := cleanShellToken(matches[i])
			if name == "" {
				continue
			}
			return name, true
		}
	}

	if strings.Contains(lower, "create file") || strings.Contains(lower, "make file") {
		return "test", true
	}
	return "", false
}

func normalizeTypedFileExtension(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "html":
		return "html"
	case "css":
		return "css"
	case "js", "javascript":
		return "js"
	case "json":
		return "json"
	case "md", "markdown":
		return "md"
	case "txt", "text":
		return "txt"
	default:
		return ""
	}
}

func parseRenameIntent(clean, lower string) (string, string, bool) {
	if matches := shellRenamePattern.FindStringSubmatch(clean); len(matches) == 3 {
		source := cleanShellToken(matches[1])
		target := cleanShellToken(matches[2])
		if source != "" && target != "" {
			return source, target, true
		}
	}
	if matches := shellRenameTestPattern.FindStringSubmatch(clean); len(matches) == 2 {
		target := cleanShellToken(matches[1])
		if target != "" {
			return "test", target, true
		}
	}
	if strings.Contains(lower, "rename that file") || strings.Contains(lower, "rename file") {
		return "", "", false
	}
	return "", "", false
}

func parseExplicitRunCommand(clean, lower string) (string, bool) {
	if matches := shellBacktickPattern.FindStringSubmatch(clean); len(matches) == 2 {
		if containsAnyPhrase(lower, []string{"run", "execute", "do it", "command"}) {
			return strings.TrimSpace(matches[1]), true
		}
	}

	markers := []string{
		"please run ",
		"can you run ",
		"run ",
		"please execute ",
		"can you execute ",
		"execute ",
	}
	for _, marker := range markers {
		idx := strings.Index(lower, marker)
		if idx < 0 {
			continue
		}
		start := idx + len(marker)
		command := strings.TrimSpace(clean[start:])
		command = strings.Trim(command, " \t\r\n\"'`.,!?;:()[]{}")
		if command != "" {
			return command, true
		}
	}

	tokens := strings.Fields(clean)
	if len(tokens) > 0 {
		name := strings.ToLower(strings.TrimSpace(tokens[0]))
		if _, ok := allowedLocalShellCommands[name]; ok {
			return strings.Join(tokens, " "), true
		}
	}

	return "", false
}

func parseRepositoryWorkflowIntent(lower string) (string, bool) {
	if containsAnyPhrase(lower, []string{
		"install requirements",
		"check requirements",
		"install dependencies",
		"install deps",
		"set up dependencies",
		"setup dependencies",
	}) {
		if command := inferDependencyInstallCommand(); command != "" {
			return command, true
		}
	}

	if containsAnyPhrase(lower, []string{
		"run tests",
		"run test",
		"test it",
		"test this",
		"execute tests",
		"verify it works",
		"make sure it works",
	}) {
		if command := inferRepositoryTestCommand(); command != "" {
			return command, true
		}
	}

	if containsAnyPhrase(lower, []string{
		"spin up docker",
		"start docker environment",
		"start docker test environment",
		"bring up docker",
		"docker test environment",
		"start test environment",
		"run docker compose",
	}) {
		if command := inferDockerUpCommand(); command != "" {
			return command, true
		}
	}

	if containsAnyPhrase(lower, []string{
		"stop docker environment",
		"stop test environment",
		"bring docker down",
		"shutdown docker environment",
	}) {
		if fileExists("docker-compose.yml") || fileExists("docker-compose.yaml") {
			return "docker compose down", true
		}
	}

	return "", false
}

func inferDependencyInstallCommand() string {
	if fileExists("./scripts/setup-host-deps.sh") {
		return "./scripts/setup-host-deps.sh --profile all -y"
	}
	if fileExists("scripts/setup-host-deps.sh") {
		return "scripts/setup-host-deps.sh --profile all -y"
	}
	if fileExists("package.json") {
		return "npm install"
	}
	if fileExists("go.mod") {
		return "go mod tidy"
	}
	if fileExists("requirements.txt") {
		return "pip install -r requirements.txt"
	}
	if fileExists("pyproject.toml") || fileExists("setup.py") {
		return "pip install -e ."
	}
	return ""
}

func inferRepositoryTestCommand() string {
	if fileExists("go.mod") {
		return "go test ./..."
	}
	if fileExists("package.json") {
		return "npm test"
	}
	if fileExists("Makefile") || fileExists("makefile") {
		return "make test"
	}
	if fileExists("pyproject.toml") || fileExists("requirements.txt") || fileExists("setup.py") {
		return "pytest -q"
	}
	return ""
}

func inferDockerUpCommand() string {
	if fileExists("docker-compose.yml") || fileExists("docker-compose.yaml") {
		return "docker compose up --build -d"
	}
	if fileExists("Dockerfile") {
		return "docker build -t local-test ."
	}
	return ""
}

func fileExists(path string) bool {
	info, err := os.Stat(strings.TrimSpace(path))
	if err != nil {
		return false
	}
	return !info.IsDir()
}

func cleanShellToken(raw string) string {
	value := strings.TrimSpace(raw)
	value = strings.Trim(value, " \t\r\n\"'`.,!?;:()[]{}")
	return value
}
