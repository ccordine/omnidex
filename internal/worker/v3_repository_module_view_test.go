package worker

import (
	"archive/zip"
	"context"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"golang.org/x/mod/module"
	"golang.org/x/mod/sumdb/dirhash"
	modulezip "golang.org/x/mod/zip"
)

func TestRepositoryGoModuleViewContainsOnlyResolvedBuildList(t *testing.T) {
	t.Parallel()
	hostCache := t.TempDir()
	resolved := writeRepositoryModuleCacheFixture(t, hostCache, "example.com/allowed", "v1.2.3")
	unrelated := filepath.Join(hostCache, "unrelated.example", "private@v1.0.0", "secret.pem")
	if err := os.MkdirAll(filepath.Dir(unrelated), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(unrelated, []byte("unrelated-module-secret"), 0o600); err != nil {
		t.Fatal(err)
	}
	view, err := projectRepositoryGoModuleView(
		context.Background(), t.TempDir(), hostCache, []repositoryGoResolvedModule{resolved},
	)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = view.Cleanup() })
	escapedPath, _ := module.EscapePath(resolved.Path)
	escapedVersion, _ := module.EscapeVersion(resolved.Version)
	for _, relative := range []string{
		escapedPath + "@" + escapedVersion + "/value.go",
		"cache/download/" + escapedPath + "/@v/" + escapedVersion + ".mod",
		"cache/download/" + escapedPath + "/@v/" + escapedVersion + ".zip",
	} {
		if _, err := os.Stat(filepath.Join(view.Root(), filepath.FromSlash(relative))); err != nil {
			t.Fatalf("projected dependency %q: %v", relative, err)
		}
	}
	if err := filepath.WalkDir(view.Root(), func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if strings.Contains(filepath.ToSlash(path), "unrelated.example") || entry.Name() == "secret.pem" {
			t.Fatalf("unrelated host cache entry entered module view: %s", path)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if err := view.VerifyExact(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(view.Root(), "unexpected"), []byte("drift"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := view.VerifyExact(context.Background()); err == nil {
		t.Fatal("module view accepted inventory drift")
	}
}

func TestRepositoryGoModuleViewFailsWhenExactCacheAuthorityIsMissing(t *testing.T) {
	t.Parallel()
	hostCache := t.TempDir()
	resolved := writeRepositoryModuleCacheFixture(t, hostCache, "example.com/allowed", "v1.2.3")
	escapedPath, _ := module.EscapePath(resolved.Path)
	escapedVersion, _ := module.EscapeVersion(resolved.Version)
	zipPath := filepath.Join(hostCache, "cache", "download", filepath.FromSlash(escapedPath), "@v", escapedVersion+".zip")
	if err := os.Remove(zipPath); err != nil {
		t.Fatal(err)
	}
	if view, err := projectRepositoryGoModuleView(
		context.Background(), t.TempDir(), hostCache, []repositoryGoResolvedModule{resolved},
	); err == nil || view != nil || !strings.Contains(err.Error(), "exact cached module") {
		t.Fatalf("missing authority view=%+v error=%v", view, err)
	}
}

func TestRepositoryGoModuleViewRejectsProjectedTreeOverflowAndCleans(t *testing.T) {
	hostCache := t.TempDir()
	resolved := writeRepositoryModuleCacheFixture(t, hostCache, "example.com/allowed", "v1.2.3")
	escapedPath, _ := module.EscapePath(resolved.Path)
	escapedVersion, _ := module.EscapeVersion(resolved.Version)
	download := filepath.Join(hostCache, "cache", "download", filepath.FromSlash(escapedPath), "@v")
	var copiedBytes int64
	for _, suffix := range []string{".mod", ".info", ".zip", ".ziphash"} {
		info, err := os.Stat(filepath.Join(download, escapedVersion+suffix))
		if err != nil {
			t.Fatal(err)
		}
		copiedBytes += info.Size()
	}
	for _, test := range []struct {
		name      string
		limits    repositoryGoModuleViewLimits
		wantError string
	}{
		{
			name: "expanded regular bytes",
			limits: repositoryGoModuleViewLimits{
				MaxRegularBytes: copiedBytes,
				MaxEntries:      maxRepositoryGoModuleViewEntries,
			},
			wantError: "regular-byte limit",
		},
		{
			name: "projected entries",
			limits: repositoryGoModuleViewLimits{
				MaxRegularBytes: maxRepositoryGoModuleViewBytes,
				MaxEntries:      1,
			},
			wantError: "entry limit",
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			sourceRoot := t.TempDir()
			projectionParent := t.TempDir()
			t.Setenv("TMPDIR", projectionParent)
			view, err := projectRepositoryGoModuleViewWithLimits(
				context.Background(), sourceRoot, hostCache,
				[]repositoryGoResolvedModule{resolved}, test.limits,
			)
			if err == nil || view != nil || !strings.Contains(err.Error(), test.wantError) {
				t.Fatalf("overflow view=%+v error=%v", view, err)
			}
			remaining, readErr := os.ReadDir(projectionParent)
			if readErr != nil {
				t.Fatal(readErr)
			}
			if len(remaining) != 0 {
				t.Fatalf("failed projection leaked temporary roots: %+v", remaining)
			}
		})
	}
}

func TestResolveRepositoryGoBuildListUsesOfflineExactCache(t *testing.T) {
	t.Parallel()
	hostCache := t.TempDir()
	resolved := writeRepositoryModuleCacheFixture(t, hostCache, "example.com/allowed", "v1.2.3")
	root := t.TempDir()
	goMod := "module example.com/source\n\ngo 1.22\n\nrequire " + resolved.Path + " " + resolved.Version + "\n"
	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte(goMod), 0o600); err != nil {
		t.Fatal(err)
	}
	goSum := resolved.Path + " " + resolved.Version + " " + resolved.Sum + "\n" +
		resolved.Path + " " + resolved.Version + "/go.mod " + resolved.GoModSum + "\n"
	if err := os.WriteFile(filepath.Join(root, "go.sum"), []byte(goSum), 0o600); err != nil {
		t.Fatal(err)
	}
	modules, err := resolveRepositoryGoBuildList(context.Background(), root, repositoryGoSandboxConfig{
		GoRoot: runtime.GOROOT(), ModuleCache: hostCache,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(modules) != 1 || modules[0] != resolved {
		t.Fatalf("resolved modules=%+v want=%+v", modules, resolved)
	}
}

func writeRepositoryModuleCacheFixture(
	t *testing.T,
	cache string,
	path string,
	version string,
) repositoryGoResolvedModule {
	t.Helper()
	m := module.Version{Path: path, Version: version}
	escapedPath, err := module.EscapePath(path)
	if err != nil {
		t.Fatal(err)
	}
	escapedVersion, err := module.EscapeVersion(version)
	if err != nil {
		t.Fatal(err)
	}
	download := filepath.Join(cache, "cache", "download", filepath.FromSlash(escapedPath), "@v")
	if err := os.MkdirAll(download, 0o700); err != nil {
		t.Fatal(err)
	}
	goMod := []byte("module " + path + "\n\ngo 1.22\n")
	zipPath := filepath.Join(download, escapedVersion+".zip")
	handle, err := os.OpenFile(zipPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	archive := zip.NewWriter(handle)
	for name, content := range map[string][]byte{
		"go.mod":   goMod,
		"value.go": []byte("package allowed\n\nfunc Value() int { return 1 }\n"),
	} {
		entry, createErr := archive.Create(path + "@" + version + "/" + name)
		if createErr != nil {
			t.Fatal(createErr)
		}
		if _, writeErr := entry.Write(content); writeErr != nil {
			t.Fatal(writeErr)
		}
	}
	if err := archive.Close(); err != nil {
		t.Fatal(err)
	}
	if err := handle.Close(); err != nil {
		t.Fatal(err)
	}
	zipSum, err := dirhash.HashZip(zipPath, dirhash.Hash1)
	if err != nil {
		t.Fatal(err)
	}
	goModName := "go.mod"
	goModSum, err := dirhash.Hash1([]string{goModName}, func(name string) (io.ReadCloser, error) {
		if name != goModName {
			return nil, os.ErrNotExist
		}
		return io.NopCloser(strings.NewReader(string(goMod))), nil
	})
	if err != nil {
		t.Fatal(err)
	}
	info, _ := json.Marshal(map[string]string{"Version": version, "Time": "2024-01-01T00:00:00Z"})
	for suffix, content := range map[string][]byte{
		".mod": goMod, ".info": info, ".ziphash": []byte(zipSum),
	} {
		if err := os.WriteFile(filepath.Join(download, escapedVersion+suffix), content, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	extracted := filepath.Join(cache, filepath.FromSlash(escapedPath)+"@"+escapedVersion)
	if err := modulezip.Unzip(extracted, m, zipPath); err != nil {
		t.Fatal(err)
	}
	return repositoryGoResolvedModule{
		Path: path, Version: version, Sum: zipSum, GoModSum: goModSum,
	}
}
