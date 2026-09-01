package worker

import (
	"encoding/json"
	"fmt"
	"strings"
)

const typeScriptBrowserTailwindImport = `@import "tailwindcss";`

func typeScriptTailwindStylesSource(stylesheet string) string {
	stylesheet = strings.TrimSpace(stylesheet)
	if stylesheet == "" {
		return typeScriptBrowserTailwindImport + "\n"
	}
	return typeScriptBrowserTailwindImport + "\n\n" + stylesheet + "\n"
}

func validateTypeScriptBrowserTailwindAuthority(files map[string]string) error {
	manifestSource, manifestExists := files["package.json"]
	lockSource, lockExists := files["package-lock.json"]
	config, configExists := files["vite.config.ts"]
	stylesheet, stylesheetExists := files["src/styles.css"]
	if !manifestExists || !lockExists || !configExists || !stylesheetExists {
		return fmt.Errorf("TypeScript browser Tailwind toolchain requires manifest, lock, Vite config, and stylesheet")
	}
	var manifest typeScriptPackageManifest
	if err := json.Unmarshal([]byte(manifestSource), &manifest); err != nil {
		return fmt.Errorf("decode TypeScript browser package manifest: %w", err)
	}
	var lock struct {
		Packages map[string]struct {
			Version   string `json:"version"`
			Integrity string `json:"integrity"`
		} `json:"packages"`
	}
	if err := json.Unmarshal([]byte(lockSource), &lock); err != nil {
		return fmt.Errorf("decode TypeScript browser package lock: %w", err)
	}
	for _, dependency := range []string{"@tailwindcss/vite", "tailwindcss"} {
		version := manifest.DevDependencies[dependency]
		entry, exists := lock.Packages["node_modules/"+dependency]
		if version == "" || !exists || entry.Version != version ||
			!validSHA512Integrity(entry.Integrity) {
			return fmt.Errorf(
				"TypeScript browser lock lacks integrity-pinned %s %s", dependency, version,
			)
		}
	}
	if config != typeScriptViteConfigSource() {
		return fmt.Errorf("TypeScript browser Vite config differs from its code-owned Tailwind authority")
	}
	if !strings.HasPrefix(stylesheet, typeScriptBrowserTailwindImport+"\n") {
		return fmt.Errorf("TypeScript browser stylesheet lacks its code-owned Tailwind import")
	}
	return nil
}
