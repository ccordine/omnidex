package worker

import (
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
