package omni

import (
	"fmt"
	"path/filepath"
	"strings"
)

func deterministicReactPackageJSON(contract ImplementationArchitectContract) string {
	name := strings.Trim(contract.TargetRoot, "./ ")
	if name == "" || name == "." {
		name = "omnidex-app"
	}
	name = strings.ToLower(strings.NewReplacer("/", "-", "_", "-", " ", "-").Replace(name))
	return fmt.Sprintf(`{
  "name": %q,
  "version": "0.1.0",
  "private": true,
  "type": "module",
  "scripts": {
    "dev": "vite --host 0.0.0.0",
    "build": "vite build",
    "preview": "vite preview --host 0.0.0.0",
    "test": "node scripts/smoke-test.mjs"
  },
  "dependencies": {
    "@vitejs/plugin-react": "latest",
    "vite": "latest",
    "react": "latest",
    "react-dom": "latest",
    "lucide-react": "latest"
  },
  "devDependencies": {}
}
`, name)
}

func deterministicViteReactConfig() string {
	return `import { defineConfig } from 'vite';
import react from '@vitejs/plugin-react';

export default defineConfig({
  plugins: [react()],
});
`
}

func deterministicReactIndexHTML(entry string, contract ImplementationArchitectContract) string {
	entry = strings.TrimPrefix(filepath.ToSlash(strings.TrimSpace(entry)), "/")
	if entry == "" {
		entry = "src/main.jsx"
	}
	title := "Omnidex React App"
	if promptRequestsNoteApp(architectContractPrompt(contract), contract.SourceToolTask) {
		title = "Notes App"
	} else if promptRequestsMusicStudio(architectContractPrompt(contract), contract.SourceToolTask) {
		title = "Omnidex Beat Studio"
	} else if promptRequestsGraphingCalculator(architectContractPrompt(contract), contract.SourceToolTask) {
		title = "Graphing Calculator"
	}
	return fmt.Sprintf(`<!doctype html>
<html lang="en">
  <head>
    <meta charset="UTF-8" />
    <meta name="viewport" content="width=device-width, initial-scale=1.0" />
    <title>%s</title>
  </head>
  <body>
    <div id="root"></div>
    <script type="module" src="/%s"></script>
  </body>
</html>
`, title, entry)
}

func deterministicReactMountEntry(path string) string {
	return `import React from 'react';
import { createRoot } from 'react-dom/client';
import App from './App.js';
import './App.css';

createRoot(document.getElementById('root')).render(
  <React.StrictMode>
    <App />
  </React.StrictMode>
);
`
}

func deterministicReactSmokeTest(contract ImplementationArchitectContract) string {
	prompt := architectContractPrompt(contract)
	criteria := contract.AcceptanceCriteria
	signals := []string{}
	for _, criterion := range criteria {
		for _, signal := range acceptanceCriterionSignals(criterion) {
			signals = append(signals, signal)
		}
	}
	signals = uniqueNonEmptyStrings(signals)
	if len(signals) == 0 {
		return deterministicGenericReactSmokeTest(prompt)
	}
	lines := []string{
		"import { readFileSync } from 'node:fs';",
		"",
		"const app = readFileSync('src/App.js', 'utf8');",
		"const css = readFileSync('src/App.css', 'utf8');",
		"const combined = `${app}\\n${css}`.toLowerCase();",
		"const required = [",
	}
	for _, signal := range signals {
		lines = append(lines, fmt.Sprintf("  %q,", strings.ToLower(signal)))
	}
	lines = append(lines,
		"];",
		"const missing = required.filter((term) => !combined.includes(term));",
		"if (missing.length > 0) {",
		"  console.error(`Missing required acceptance signal(s): ${missing.join(', ')}`);",
		"  process.exit(1);",
		"}",
	)
	if promptRequestsMusicStudio(prompt, contract.SourceToolTask) {
		lines = append(lines,
			"const hasRange = combined.includes('type=\"range\"') || combined.includes(\"type: 'range'\") || combined.includes('type: \"range\"');",
			"if (!combined.includes('usestate') || !hasRange || !combined.includes('button')) {",
			"  console.error('Studio implementation must include interactive React controls.');",
			"  process.exit(1);",
			"}",
			"console.log('music studio smoke test passed');",
			"",
		)
	} else {
		lines = append(lines,
			"if (!combined.includes('export default')) {",
			"  console.error('App.js must export a React component');",
			"  process.exit(1);",
			"}",
			"console.log('react acceptance smoke test passed');",
			"",
		)
	}
	return strings.Join(lines, "\n")
}

func deterministicGenericReactSmokeTest(prompt string) string {
	forbiddenBlock := ""
	if !promptRequestsMusicStudio(prompt, "") {
		forbiddenBlock = `
const forbidden = ["studio-shell", "channel-rack", "piano-roll", "omnidex beat studio", "pattern step sequencer", "channel rack", "beat studio", "sequencer"];
const hit = forbidden.filter((term) => combined.includes(term));
if (hit.length > 0) {
  console.error("App implements foreign music-studio domain: " + hit.join(", "));
  process.exit(1);
}`
	}
	return "import { readFileSync } from 'node:fs';\n\n" +
		"const app = readFileSync('src/App.js', 'utf8');\n" +
		"const css = readFileSync('src/App.css', 'utf8');\n" +
		"const combined = (app + '\\n' + css).toLowerCase();\n" +
		"if (!combined.includes('export default')) {\n" +
		"  console.error('App.js must export a React component');\n" +
		"  process.exit(1);\n" +
		"}\n" +
		forbiddenBlock +
		"console.log('react smoke test passed');\n"
}
