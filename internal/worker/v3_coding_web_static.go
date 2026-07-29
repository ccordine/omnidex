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
	Scripts         map[string]string `json:"scripts"`
	Dependencies    map[string]string `json:"dependencies"`
	DevDependencies map[string]string `json:"devDependencies"`
}

func typeScriptBrowserStaticFiles(packageName, productName, stylesheet string) []directCodingFileTask {
	manifest := typeScriptPackageManifest{
		Name: packageName, Private: true, Version: "1.0.0", Type: "module",
		Scripts: map[string]string{
			"dev": "vite", "test": "vitest run", "typecheck": "tsc --noEmit",
			"build": "npm run typecheck && vite build",
		},
		Dependencies: map[string]string{
			"react": "19.2.7", "react-dom": "19.2.7",
		},
		DevDependencies: map[string]string{
			"@testing-library/react": "16.3.2", "@types/react": "19.2.17",
			"@types/react-dom": "19.2.3", "@vitejs/plugin-react": "5.2.0",
			"jsdom": "26.1.0", "typescript": "5.9.3", "vite": "6.4.2", "vitest": "4.1.8",
		},
	}
	encoded, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		panic(fmt.Sprintf("marshal code-owned TypeScript package manifest: %v", err))
	}
	return []directCodingFileTask{
		{Path: ".gitignore", Content: "node_modules\ndist\n.vite\n*.log\n"},
		{Path: "package.json", Content: string(encoded) + "\n"},
		{Path: "index.html", Content: typeScriptWebIndexSource(productName)},
		{Path: "tsconfig.json", Content: typeScriptWebConfigSource()},
		{Path: "vite.config.ts", Content: typeScriptViteConfigSource()},
		{Path: "src/main.tsx", Content: typeScriptWebMainSource()},
		{Path: "src/styles.css", Content: stylesheet},
	}
}

func typeScriptWebIndexSource(productName string) string {
	return fmt.Sprintf(`<!doctype html>
<html lang="en">
  <head>
    <meta charset="UTF-8" />
    <meta name="viewport" content="width=device-width, initial-scale=1.0" />
    <meta name="theme-color" content="#0b0d12" />
    <title>%s</title>
  </head>
  <body>
    <div id="root"></div>
    <script type="module" src="/src/main.tsx"></script>
  </body>
</html>
`, html.EscapeString(productName))
}

func typeScriptWebConfigSource() string {
	return `{
  "compilerOptions": {
    "target": "ES2022",
    "useDefineForClassFields": true,
    "lib": ["ES2022", "DOM", "DOM.Iterable"],
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
    "jsx": "react-jsx",
    "types": ["vitest/globals"]
  },
  "include": ["src"],
  "exclude": ["dist", "node_modules"]
}
`
}

func typeScriptViteConfigSource() string {
	return `import { defineConfig } from 'vitest/config';
import react from '@vitejs/plugin-react';

export default defineConfig({
  plugins: [react()],
  test: {
    environment: 'jsdom',
    globals: true,
  },
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
