package worker

import (
	"strings"
	"testing"
)

func TestTypeScriptBrowserUsesOneIntegrityLockedNPMGraph(t *testing.T) {
	files, err := typeScriptBrowserStaticFiles(
		requireDirectCodingVersionProfile(t, typeScriptBrowserVersionProfileV1),
		"locked-browser", "Locked browser", genericBrowserStylesSource(),
	)
	if err != nil {
		t.Fatal(err)
	}
	lock := ""
	for _, file := range files {
		if file.Path == "package-lock.json" {
			lock = file.Content
			break
		}
	}
	if lock == "" {
		t.Fatal("TypeScript browser static files omit package-lock.json")
	}
	for _, required := range []string{
		`"name": "locked-browser"`, `"lockfileVersion": 3`,
		`"react": "19.2.7"`, `"react-dom": "19.2.7"`,
		`"@tailwindcss/vite": "4.1.12"`, `"tailwindcss": "4.1.12"`,
		`"typescript": "5.9.3"`, `"vite": "6.4.2"`, `"vitest": "4.1.8"`,
		`"node_modules/@tailwindcss/vite"`, `"node_modules/tailwindcss"`,
		`"node_modules/react"`, `"integrity": "sha512-`,
	} {
		if !strings.Contains(lock, required) {
			t.Fatalf("TypeScript browser lock omits %s", required)
		}
	}
	if strings.Contains(lock, `"name": "typescript-browser"`) {
		t.Fatal("TypeScript browser lock retained its template package identity")
	}
	wantCommand := "ci --ignore-scripts --no-audit --no-fund"
	if got := strings.Join(directCodingNPMInstallArgs(), " "); got != wantCommand {
		t.Fatalf("TypeScript npm setup=%q want=%q", got, wantCommand)
	}
}

func TestPinnedNPMLockRejectsManifestDrift(t *testing.T) {
	template := []byte(`{
  "name":"fixture",
  "lockfileVersion":3,
  "packages":{
    "":{"name":"fixture","dependencies":{"alpha":"1.0.0"}},
    "node_modules/alpha":{"version":"1.0.0","integrity":"sha512-MDAwMDAwMDAwMDAwMDAwMDAwMDAwMDAwMDAwMDAwMDAwMDAwMDAwMDAwMDAwMDAwMDAwMDAwMDAwMDAwMDAwMA=="}
  }
}`)
	if _, err := materializePinnedNPMLock(
		template, "renamed", 3, map[string]string{"alpha": "2.0.0"}, nil,
	); err == nil {
		t.Fatal("npm lock accepted a direct dependency version drift")
	}
}
