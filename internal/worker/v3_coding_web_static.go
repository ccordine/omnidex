package worker

import (
	"encoding/json"
	"fmt"
	"html"
)

type typeScriptPackageManifest struct {
	Name            string            `json:"name"`
	Private         bool              `json:"private"`
	Version         string            `json:"version"`
	Type            string            `json:"type"`
	Engines         map[string]string `json:"engines"`
	Scripts         map[string]string `json:"scripts"`
	Dependencies    map[string]string `json:"dependencies"`
	DevDependencies map[string]string `json:"devDependencies"`
}

func typeScriptBrowserStaticFiles(
	profile directCodingProjectVersionProfile,
	packageName, productName, stylesheet string,
) ([]directCodingFileTask, error) {
	node, err := directCodingVersionComponent(profile, "node")
	if err != nil {
		return nil, err
	}
	npm, err := directCodingVersionComponent(profile, "npm")
	if err != nil {
		return nil, err
	}
	ecmascript, err := directCodingVersionComponent(profile, "ecmascript")
	if err != nil {
		return nil, err
	}
	manifest := typeScriptPackageManifest{
		Name: packageName, Private: true, Version: "1.0.0", Type: "module",
		Engines: map[string]string{"node": node, "npm": npm},
		Scripts: map[string]string{
			"dev": "vite", "typecheck": "tsc --noEmit",
			"build": "npm run typecheck && vite build",
		},
		Dependencies:    profile.NPMDependencies,
		DevDependencies: profile.NPMDevDependencies,
	}
	encoded, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("marshal code-owned TypeScript package manifest: %w", err)
	}
	packageLock, err := typeScriptBrowserPackageLock(profile, packageName)
	if err != nil {
		return nil, err
	}
	return []directCodingFileTask{
		{Path: ".gitignore", Content: "node_modules\ndist\n.vite\n*.log\n"},
		{Path: "package.json", Content: string(encoded) + "\n"},
		{Path: "package-lock.json", Content: packageLock},
		{Path: "index.html", Content: typeScriptWebIndexSource(productName)},
		{Path: "tsconfig.json", Content: typeScriptWebConfigSource(ecmascript)},
		{Path: "vite.config.ts", Content: typeScriptViteConfigSource()},
		{Path: "src/styles.css", Content: typeScriptTailwindStylesSource(stylesheet)},
	}, nil
}

func typeScriptWebIndexSource(productName string) string {
	return fmt.Sprintf(`<!doctype html>
<html lang="en">
  <head>
    <meta charset="UTF-8" />
    <meta name="viewport" content="width=device-width, initial-scale=1.0" />
    <title>%s</title>
  </head>
  <body>
    <div id="root"></div>
    <script type="module" src="/src/main.tsx"></script>
  </body>
</html>
`, html.EscapeString(productName))
}

func typeScriptWebConfigSource(ecmascript string) string {
	return fmt.Sprintf(`{
  "compilerOptions": {
    "target": %q,
    "useDefineForClassFields": true,
    "lib": [%q, "DOM", "DOM.Iterable"],
    "allowJs": false,
    "skipLibCheck": true,
    "esModuleInterop": true,
    "allowSyntheticDefaultImports": true,
    "strict": true,
    "forceConsistentCasingInFileNames": true,
    "module": "ESNext",
    "moduleResolution": "Bundler",
    "resolveJsonModule": true,
    "isolatedModules": true,
    "noEmit": true,
		"jsx": "react-jsx"
  },
  "include": ["src"],
  "exclude": ["dist", "node_modules"]
}
`, ecmascript, ecmascript)
}

func typeScriptViteConfigSource() string {
	return `import { defineConfig } from 'vite';
import react from '@vitejs/plugin-react';
import tailwindcss from '@tailwindcss/vite';

export default defineConfig({
  plugins: [react(), tailwindcss()],
});
`
}

func typeScriptWebMainSource() string {
	return `import { createRoot } from 'react-dom/client';
import { App } from './App';
import './styles.css';

const root = document.getElementById('root');
if (root === null) throw new Error('Application root #root is missing.');

createRoot(root).render(<App />);
`
}
