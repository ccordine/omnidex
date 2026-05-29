package ollama

import "strings"

// MatchesOllamaModel reports whether an installed Ollama tag satisfies a configured model name.
// Configured names without a tag (e.g. nomic-embed-text) match any tag for that model.
func MatchesOllamaModel(configured, installed string) bool {
	configured = strings.TrimSpace(configured)
	installed = strings.TrimSpace(installed)
	if configured == "" || installed == "" {
		return false
	}
	if configured == installed {
		return true
	}

	configuredName, configuredTag, hasConfiguredTag := splitModelRef(configured)
	installedName, installedTag, hasInstalledTag := splitModelRef(installed)
	if configuredName != installedName {
		return false
	}
	if !hasConfiguredTag {
		return true
	}
	if !hasInstalledTag {
		return configuredTag == "latest"
	}
	return configuredTag == installedTag
}

// ModelIsAvailable reports whether configured appears in the installed tag list.
func ModelIsAvailable(installed []string, configured string) bool {
	for _, tag := range installed {
		if MatchesOllamaModel(configured, tag) {
			return true
		}
	}
	return false
}

func splitModelRef(name string) (base, tag string, hasTag bool) {
	if idx := strings.LastIndex(name, ":"); idx > 0 {
		return name[:idx], name[idx+1:], true
	}
	return name, "", false
}
