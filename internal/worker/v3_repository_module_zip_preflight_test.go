package worker

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/binary"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"golang.org/x/mod/module"
)

func TestRepositoryModuleZipPreflightRejectsClassicLimitsBeforeLibraryParsing(t *testing.T) {
	t.Parallel()
	archive := repositoryModuleTestZip(t, []string{"wrong/one", "wrong/two", "wrong/three"})
	for name, test := range map[string]struct {
		mutate func([]byte)
		want   string
	}{
		"entry overflow": {
			mutate: func(raw []byte) {},
			want:   "entry limit",
		},
		"underreported entry overflow": {
			mutate: func(raw []byte) {
				eocd := repositoryTestZipEOCD(t, raw)
				binary.LittleEndian.PutUint16(raw[eocd+8:eocd+10], 1)
				binary.LittleEndian.PutUint16(raw[eocd+10:eocd+12], 1)
			},
			want: "entry limit",
		},
		"central directory overflow": {
			mutate: func(raw []byte) {
				eocd := repositoryTestZipEOCD(t, raw)
				binary.LittleEndian.PutUint32(raw[eocd+12:eocd+16], uint32(maxRepositoryGoModuleCentralDirectoryBytes+1))
			},
			want: "central-directory byte limit",
		},
		"multi disk": {
			mutate: func(raw []byte) {
				eocd := repositoryTestZipEOCD(t, raw)
				binary.LittleEndian.PutUint16(raw[eocd+4:eocd+6], 1)
			},
			want: "multi-disk",
		},
		"malformed end": {
			mutate: func(raw []byte) {
				eocd := repositoryTestZipEOCD(t, raw)
				raw[eocd] = 0
			},
			want: "end record",
		},
	} {
		t.Run(name, func(t *testing.T) {
			raw := append([]byte(nil), archive...)
			test.mutate(raw)
			path := filepath.Join(t.TempDir(), "module.zip")
			if err := os.WriteFile(path, raw, 0o600); err != nil {
				t.Fatal(err)
			}
			if _, err := preflightRepositoryGoModuleZip(path, 2); err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("preflight error=%v, want %q", err, test.want)
			}
		})
	}
}

func TestRepositoryModuleZipPreflightAcceptsBoundedZIP64Directory(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "module.zip")
	if err := os.WriteFile(path, repositorySyntheticZIP64(1, 46), 0o600); err != nil {
		t.Fatal(err)
	}
	result, err := preflightRepositoryGoModuleZip(path, 2)
	if err != nil {
		t.Fatal(err)
	}
	if result.Entries != 1 || result.Bytes != 46 {
		t.Fatalf("ZIP64 directory=%+v", result)
	}
}

func TestRepositoryCachedModuleInspectionPreflightsBeforeZipLibraries(t *testing.T) {
	t.Parallel()
	base := repositoryModuleTestZip(t, []string{"wrong/one", "wrong/two", "wrong/three"})
	for name, test := range map[string]struct {
		mutate           func([]byte)
		archiveAllowance int
		want             string
	}{
		"many entries": {
			mutate: func([]byte) {}, archiveAllowance: 2, want: "entry limit",
		},
		"oversize central directory": {
			mutate: func(raw []byte) {
				eocd := repositoryTestZipEOCD(t, raw)
				binary.LittleEndian.PutUint32(raw[eocd+12:eocd+16], uint32(maxRepositoryGoModuleCentralDirectoryBytes+1))
			},
			archiveAllowance: 10, want: "central-directory byte limit",
		},
	} {
		t.Run(name, func(t *testing.T) {
			item := repositoryGoResolvedModule{Path: "example.com/preflight", Version: "v1.0.0"}
			raw := append([]byte(nil), base...)
			test.mutate(raw)
			root, metadataEntries := writeRepositoryPreflightCache(t, item, raw)
			_, err := inspectRepositoryGoCachedModuleUsage(
				context.Background(), root, item, metadataEntries+test.archiveAllowance,
			)
			if err == nil || !strings.Contains(err.Error(), "preflight exact cached module archive") ||
				!strings.Contains(err.Error(), test.want) {
				t.Fatalf("cached inspection error=%v, want preflight %q", err, test.want)
			}
		})
	}
}

func TestRepositoryModuleZipPreflightRejectsZIP64LimitsBeforeDirectoryParsing(t *testing.T) {
	t.Parallel()
	for name, test := range map[string]struct {
		entries        uint64
		directoryBytes uint64
		limit          int
		want           string
	}{
		"entry overflow": {entries: 3, directoryBytes: 46, limit: 2, want: "entry limit"},
		"directory overflow": {
			entries: 1, directoryBytes: uint64(maxRepositoryGoModuleCentralDirectoryBytes + 1),
			limit: 2, want: "central-directory byte limit",
		},
	} {
		t.Run(name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "module.zip")
			if err := os.WriteFile(path, repositorySyntheticZIP64(test.entries, test.directoryBytes), 0o600); err != nil {
				t.Fatal(err)
			}
			if _, err := preflightRepositoryGoModuleZip(path, test.limit); err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("preflight error=%v, want %q", err, test.want)
			}
		})
	}
}

func TestRepositoryModuleZipPreflightFailureCleansProjection(t *testing.T) {
	hostCache := t.TempDir()
	resolved := writeRepositoryModuleCacheFixture(t, hostCache, "example.com/overflow", "v1.0.0")
	zipPath := repositoryTestCachedZipPath(t, hostCache, resolved)
	raw, err := os.ReadFile(zipPath)
	if err != nil {
		t.Fatal(err)
	}
	eocd := repositoryTestZipEOCD(t, raw)
	binary.LittleEndian.PutUint16(raw[eocd+8:eocd+10], 30)
	binary.LittleEndian.PutUint16(raw[eocd+10:eocd+12], 30)
	if err := os.WriteFile(zipPath, raw, 0o600); err != nil {
		t.Fatal(err)
	}
	projectionParent := t.TempDir()
	t.Setenv("TMPDIR", projectionParent)
	view, err := projectRepositoryGoModuleViewWithLimits(
		context.Background(), t.TempDir(), hostCache, []repositoryGoResolvedModule{resolved},
		repositoryGoModuleViewLimits{MaxRegularBytes: maxRepositoryGoModuleViewBytes, MaxEntries: 25},
	)
	if err == nil || view != nil || !strings.Contains(err.Error(), "entry limit") {
		t.Fatalf("projection view=%v error=%v", view, err)
	}
	entries, err := os.ReadDir(projectionParent)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Fatalf("failed ZIP preflight leaked projection roots: %+v", entries)
	}
}

func repositoryModuleTestZip(t *testing.T, names []string) []byte {
	t.Helper()
	var output bytes.Buffer
	archive := zip.NewWriter(&output)
	for _, name := range names {
		entry, err := archive.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := entry.Write([]byte("x")); err != nil {
			t.Fatal(err)
		}
	}
	if err := archive.Close(); err != nil {
		t.Fatal(err)
	}
	return output.Bytes()
}

func repositoryTestZipEOCD(t *testing.T, raw []byte) int {
	t.Helper()
	offset := bytes.LastIndex(raw, []byte{'P', 'K', 5, 6})
	if offset < 0 {
		t.Fatal("test ZIP has no EOCD")
	}
	return offset
}

func repositorySyntheticZIP64(entries, directoryBytes uint64) []byte {
	const directorySize = 46
	raw := make([]byte, directorySize+56+20+22)
	binary.LittleEndian.PutUint32(raw[0:4], repositoryZipCentralDirectorySignature)
	zip64 := directorySize
	binary.LittleEndian.PutUint32(raw[zip64:zip64+4], repositoryZip64EndSignature)
	binary.LittleEndian.PutUint64(raw[zip64+4:zip64+12], 44)
	binary.LittleEndian.PutUint64(raw[zip64+24:zip64+32], entries)
	binary.LittleEndian.PutUint64(raw[zip64+32:zip64+40], entries)
	binary.LittleEndian.PutUint64(raw[zip64+40:zip64+48], directoryBytes)
	locator := zip64 + 56
	binary.LittleEndian.PutUint32(raw[locator:locator+4], repositoryZip64LocatorSignature)
	binary.LittleEndian.PutUint64(raw[locator+8:locator+16], uint64(zip64))
	binary.LittleEndian.PutUint32(raw[locator+16:locator+20], 1)
	eocd := locator + 20
	binary.LittleEndian.PutUint32(raw[eocd:eocd+4], repositoryZipEndSignature)
	binary.LittleEndian.PutUint16(raw[eocd+8:eocd+10], ^uint16(0))
	binary.LittleEndian.PutUint16(raw[eocd+10:eocd+12], ^uint16(0))
	binary.LittleEndian.PutUint32(raw[eocd+12:eocd+16], ^uint32(0))
	binary.LittleEndian.PutUint32(raw[eocd+16:eocd+20], ^uint32(0))
	return raw
}

func repositoryTestCachedZipPath(t *testing.T, root string, item repositoryGoResolvedModule) string {
	t.Helper()
	escapedPath, err := module.EscapePath(item.Path)
	if err != nil {
		t.Fatal(err)
	}
	escapedVersion, err := module.EscapeVersion(item.Version)
	if err != nil {
		t.Fatal(err)
	}
	return filepath.Join(root, "cache", "download", filepath.FromSlash(escapedPath), "@v", escapedVersion+".zip")
}

func writeRepositoryPreflightCache(
	t *testing.T,
	item repositoryGoResolvedModule,
	zipRaw []byte,
) (string, int) {
	t.Helper()
	root := t.TempDir()
	escapedPath, err := module.EscapePath(item.Path)
	if err != nil {
		t.Fatal(err)
	}
	escapedVersion, err := module.EscapeVersion(item.Version)
	if err != nil {
		t.Fatal(err)
	}
	relativeDownload := filepath.ToSlash(filepath.Join("cache", "download", filepath.FromSlash(escapedPath), "@v"))
	download := filepath.Join(root, filepath.FromSlash(relativeDownload))
	if err := os.MkdirAll(download, 0o700); err != nil {
		t.Fatal(err)
	}
	metadataEntries := 0
	for suffix, content := range map[string][]byte{
		".mod": []byte("x"), ".info": []byte("x"), ".zip": zipRaw, ".ziphash": []byte("x"),
	} {
		if err := os.WriteFile(filepath.Join(download, escapedVersion+suffix), content, 0o600); err != nil {
			t.Fatal(err)
		}
		metadataEntries += conservativeRepositoryPathEntries(relativeDownload + "/" + escapedVersion + suffix)
	}
	return root, metadataEntries
}
