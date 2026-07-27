package omni

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
)

func validateCodeContentProposalForArchitectItem(content string, contract ImplementationArchitectContract, item ArchitectWorkItem) error {
	trimmed := strings.TrimSpace(content)
	if trimmed == "" {
		return fmt.Errorf("generated content is empty")
	}
	path := filepath.ToSlash(strings.ToLower(item.Path))
	lower := strings.ToLower(trimmed)
	if err := validateArchitectContentKindForPath(path, trimmed); err != nil {
		return err
	}
	switch path {
	case "package.json":
		var pkg struct {
			Scripts         map[string]string `json:"scripts"`
			Dependencies    map[string]string `json:"dependencies"`
			DevDependencies map[string]string `json:"devDependencies"`
		}
		if err := json.Unmarshal([]byte(trimmed), &pkg); err != nil {
			return fmt.Errorf("package.json content must be valid JSON: %w", err)
		}
		if strings.Contains(strings.ToLower(pkg.Scripts["test"]), "no test specified") || strings.TrimSpace(pkg.Scripts["test"]) == "" {
			return fmt.Errorf("package.json must replace the default failing npm test script")
		}
		if strings.TrimSpace(pkg.Scripts["build"]) == "" {
			return fmt.Errorf("package.json must define an executable build script")
		}
		if workQueueContainsPath(contract.WorkQueue, "scripts/smoke-test.mjs") && !strings.Contains(pkg.Scripts["test"], "scripts/smoke-test.mjs") {
			return fmt.Errorf("package.json test script must run scripts/smoke-test.mjs")
		}
		if workQueueContainsPath(contract.WorkQueue, "vite.config.js") && pkg.Dependencies["@vitejs/plugin-react"] == "" && pkg.DevDependencies["@vitejs/plugin-react"] == "" {
			return fmt.Errorf("package.json must include @vitejs/plugin-react because vite.config.js imports it")
		}
	case "vite.config.js":
		if !strings.Contains(lower, "defineconfig") || !strings.Contains(lower, "@vitejs/plugin-react") {
			return fmt.Errorf("vite.config.js must enable the Vite React plugin")
		}
	case "index.html":
		if !strings.Contains(lower, "id=\"root\"") && !strings.Contains(lower, "id='root'") {
			return fmt.Errorf("index.html must include a React root mount element")
		}
		if !strings.Contains(lower, "src/index.js") && !strings.Contains(lower, "src/main.jsx") && !strings.Contains(lower, "src/main.js") {
			return fmt.Errorf("index.html must load the React entry module")
		}
	case "src/index.js", "src/main.jsx", "src/main.js":
		if !strings.Contains(lower, "createroot") || !strings.Contains(lower, "app") {
			return fmt.Errorf("React mount entry must create a root and render App")
		}
		if !strings.Contains(lower, "from './app") && !strings.Contains(lower, "from \"./app") {
			return fmt.Errorf("React mount entry must import the real App component")
		}
	case "scripts/smoke-test.mjs":
		if strings.Contains(lower, "<!doctype html") || strings.Contains(lower, "<html") {
			return fmt.Errorf("smoke-test.mjs must be JavaScript, not HTML")
		}
		if strings.Contains(lower, "require(") || strings.Contains(lower, "require('") || strings.Contains(lower, "require(\"") {
			return fmt.Errorf("smoke-test.mjs must use ESM import syntax because package.json sets type=module")
		}
		if !strings.Contains(lower, "readfilesync") || !strings.Contains(lower, "process.exit") {
			return fmt.Errorf("smoke-test.mjs must be an executable deterministic source probe")
		}
		if !strings.Contains(lower, "src/app.js") {
			return fmt.Errorf("smoke-test.mjs must inspect src/App.js rather than only index.html")
		}
		if missing := missingAcceptanceSignals(trimmed, contract.AcceptanceCriteria); len(missing) > 0 {
			return fmt.Errorf("smoke-test.mjs must check requested UI signal(s): %s", strings.Join(missing, ", "))
		}
	case "src/app.js", "src/app.jsx":
		if strings.Contains(lower, "reactdom") || strings.Contains(lower, "createroot") || strings.Contains(lower, "getelementbyid") {
			return fmt.Errorf("App component must implement the app UI, not duplicate the React mount entry")
		}
		if path == "src/app.js" && (strings.Contains(lower, "return (") || strings.Contains(lower, "<main") || strings.Contains(lower, "<section") || strings.Contains(lower, "<button")) {
			return fmt.Errorf("App.js must avoid JSX syntax; use React.createElement-compatible JavaScript")
		}
		if strings.Contains(lower, "placeholder-only") || strings.Contains(lower, "placeholder ui") || strings.Contains(lower, "render-only placeholder") {
			return fmt.Errorf("App component must implement substantive UI instead of placeholders")
		}
		if strings.Contains(lower, "return null") && !strings.Contains(lower, "usestate") && !strings.Contains(lower, "button") {
			return fmt.Errorf("App component must implement substantive UI instead of placeholders")
		}
		if !strings.Contains(lower, "export default") {
			return fmt.Errorf("App component must export a default component")
		}
		if len(contract.AcceptanceCriteria) > 0 {
			if missing := missingAcceptanceSignals(trimmed, contract.AcceptanceCriteria); len(missing) > 0 {
				return fmt.Errorf("App component must include requested UI signal(s): %s", strings.Join(missing, ", "))
			}
		} else if !strings.Contains(lower, "usestate") && !strings.Contains(lower, "button") && !strings.Contains(lower, "input") && !strings.Contains(lower, "textarea") {
			return fmt.Errorf("App component must include substantive interactive UI, not an empty shell")
		}
	case "src/app.css":
		if strings.Contains(lower, "import react") ||
			strings.Contains(lower, "react.createelement") ||
			strings.Contains(lower, "const ") ||
			strings.Contains(lower, "=>") ||
			strings.Contains(lower, "export default") {
			return fmt.Errorf("App stylesheet must be CSS, not JavaScript or React source")
		}
		if strings.Contains(lower, "placeholder") || strings.Contains(lower, "todo") || strings.Contains(lower, "add more styles") {
			return fmt.Errorf("App stylesheet must style substantive UI instead of placeholders or unfinished notes")
		}
		if len(contract.AcceptanceCriteria) > 0 {
			if missing := missingCSSAcceptanceSignals(trimmed, contract.AcceptanceCriteria); len(missing) > 0 {
				return fmt.Errorf("App stylesheet must style requested UI signal(s): %s", strings.Join(missing, ", "))
			}
		}
	}
	return nil
}

func validateArchitectContentKindForPath(path, content string) error {
	path = filepath.ToSlash(strings.ToLower(strings.TrimSpace(path)))
	trimmed := strings.TrimSpace(content)
	lower := strings.ToLower(trimmed)
	switch {
	case path == "vite.config.js":
		if strings.Contains(lower, "<!doctype html") || strings.Contains(lower, "<html") || looksLikeCSSContent(trimmed) {
			return fmt.Errorf("architect work item content kind rejected: vite.config.js must be JavaScript config, not HTML or CSS")
		}
	case strings.HasSuffix(path, ".html"):
		if !strings.Contains(lower, "<html") && !strings.Contains(lower, "<!doctype html") {
			return fmt.Errorf("architect work item content kind rejected: .html path requires HTML content")
		}
		if strings.Contains(lower, "export default") || strings.Contains(lower, "react.createelement") {
			return fmt.Errorf("architect work item content kind rejected: .html path received React/JavaScript module content")
		}
	case strings.HasSuffix(path, ".css"):
		if strings.Contains(lower, "import react") ||
			strings.Contains(lower, "from 'react'") ||
			strings.Contains(lower, `from "react"`) ||
			strings.Contains(lower, "export default") ||
			strings.Contains(lower, "react.createelement") ||
			strings.Contains(lower, "createroot(") {
			return fmt.Errorf("architect work item content kind rejected: .css path requires CSS, not JavaScript or React source")
		}
		if strings.Contains(lower, "<!doctype html") || strings.Contains(lower, "<html") {
			return fmt.Errorf("architect work item content kind rejected: .css path requires CSS, not HTML")
		}
	case strings.HasSuffix(path, ".js") || strings.HasSuffix(path, ".jsx") || strings.HasSuffix(path, ".mjs"):
		if strings.Contains(lower, "<!doctype html") || strings.Contains(lower, "<html") {
			return fmt.Errorf("architect work item content kind rejected: JavaScript path requires JS/JSX, not HTML")
		}
		if looksLikeCSSContent(trimmed) && !strings.Contains(lower, "export ") && !strings.Contains(lower, "import ") && !strings.Contains(lower, "function ") && !strings.Contains(lower, "const ") {
			return fmt.Errorf("architect work item content kind rejected: JavaScript path requires JS/JSX, not raw CSS")
		}
	case strings.HasSuffix(path, ".json"):
		if strings.Contains(lower, "<!doctype html") || strings.Contains(lower, "<html") || strings.Contains(lower, "export default") {
			return fmt.Errorf("architect work item content kind rejected: JSON path requires JSON content")
		}
	}
	return nil
}

func looksLikeCSSContent(content string) bool {
	lower := strings.ToLower(strings.TrimSpace(content))
	if lower == "" {
		return false
	}
	if strings.HasPrefix(lower, ".") || strings.HasPrefix(lower, "#") || strings.HasPrefix(lower, "body ") || strings.HasPrefix(lower, ":root") {
		return strings.Contains(lower, "{") && strings.Contains(lower, "}")
	}
	cssNeedles := []string{"display:", "color:", "background:", "padding:", "margin:", "grid-template", "font-family:", "border-radius:"}
	matches := 0
	for _, needle := range cssNeedles {
		if strings.Contains(lower, needle) {
			matches++
		}
	}
	return matches >= 2 && strings.Contains(lower, "{") && strings.Contains(lower, "}")
}

func isArchitectContentKindValidationError(err error) bool {
	return err != nil && strings.Contains(strings.ToLower(err.Error()), "content kind rejected")
}
