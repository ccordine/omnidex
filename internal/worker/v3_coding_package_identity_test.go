package worker

import (
	"encoding/json"
	"html"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestGeneratedPackageNameIsIndependentOfWorkspaceAndProductTitle(t *testing.T) {
	for _, fixture := range []struct{ directory, product, requirement string }{
		{"観測 🌳", "An observation display", "Expose one observable state."},
		{strings.Repeat("long-workspace-name-", 10), "A confirmation view", "The finished software lets a user confirm the item."},
	} {
		t.Run(fixture.product, func(t *testing.T) {
			root := filepath.Join(t.TempDir(), fixture.directory)
			if err := os.Mkdir(root, 0o700); err != nil {
				t.Fatal(err)
			}
			program := testTypeScriptBrowserProgramAtRoot(t, fixture.product, fixture.requirement, root)
			manifest := testBrowserManifest(t, program.StaticFiles)
			if manifest.Name != "workload" {
				t.Fatalf("package name=%q; want the workspace-local technical name", manifest.Name)
			}
			foundLock, foundTitle := false, false
			for _, file := range program.StaticFiles {
				switch file.Path {
				case "package-lock.json":
					var lock struct {
						Name     string
						Packages map[string]struct{ Name string }
					}
					if err := json.Unmarshal(file.Content, &lock); err != nil {
						t.Fatal(err)
					}
					if lock.Name != manifest.Name || lock.Packages[""].Name != manifest.Name {
						t.Fatalf("lock and manifest package names differ: %#v", lock)
					}
					foundLock = true
				case "index.html":
					if !strings.Contains(string(file.Content), "<title>"+html.EscapeString(fixture.product)+"</title>") {
						t.Fatalf("technical package name replaced the product title: %s", file.Content)
					}
					foundTitle = true
				}
			}
			if !foundLock || !foundTitle {
				t.Fatal("browser compilation omitted its lock or product title")
			}
		})
	}
	program := testExactGoSieveProgram(t)
	for _, file := range program.StaticFiles {
		if file.Path == "go.mod" {
			if !strings.HasPrefix(string(file.Content), "module example.invalid/workload\n") {
				t.Fatalf("Go compilation did not use the local technical name: %s", file.Content)
			}
			return
		}
	}
	t.Fatal("Go compilation omitted its module manifest")
}

func TestWorkStatePackagesDoNotHashContentForAuthority(t *testing.T) {
	for _, packageName := range []string{
		"assemblyline", "contextcompiler", "datasource", "db", "evidence",
		"modelconfig", "modelcontext", "omni", "queue", "roleplay", "worker", "workspace",
	} {
		root := filepath.Join("..", packageName)
		entries, err := os.ReadDir(root)
		if err != nil {
			t.Fatal(err)
		}
		for _, entry := range entries {
			if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".go") || strings.HasSuffix(entry.Name(), "_test.go") {
				continue
			}
			path := filepath.Join(root, entry.Name())
			source, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			for _, retired := range []string{
				`"crypto/sha`, `"crypto/md5"`, "portableWorkDigestPattern",
				"directCodingDigest", "normalizeDirectCodingModuleSegment",
			} {
				if strings.Contains(string(source), retired) {
					t.Errorf("work-state source %s reintroduced %q", path, retired)
				}
			}
		}
	}
}
