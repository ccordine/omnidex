package cognitiongauntlet

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

const (
	offlineMigrationManifestName     = "SHA256SUMS"
	maxOfflineMigrationManifestBytes = 1024 * 1024
	maxOfflineMigrationBytes         = 16 * 1024 * 1024
)

var offlineMigrationName = regexp.MustCompile(`^[0-9]{3}_[A-Za-z0-9_]+\.sql$`)

func releaseMigrationBundle(
	executable string,
	expectedManifestSHA256 string,
) (string, error) {
	if executable == "" || filepath.Clean(executable) != executable ||
		!validDigest(expectedManifestSHA256) {
		return "", fmt.Errorf("release migration bundle authority is invalid")
	}
	resolved, err := filepath.EvalSymlinks(executable)
	if err != nil {
		return "", fmt.Errorf("resolve release executable: %w", err)
	}
	resolved, err = filepath.Abs(resolved)
	if err != nil || filepath.Base(filepath.Dir(resolved)) != "bin" {
		return "", fmt.Errorf("release executable is not inside the registered bin directory")
	}
	directory := filepath.Join(filepath.Dir(filepath.Dir(resolved)), "migrations")
	if err := verifyMigrationBundle(directory, expectedManifestSHA256); err != nil {
		return "", err
	}
	return directory, nil
}

func verifyMigrationBundle(directory string, expectedManifestSHA256 string) error {
	manifestPath := filepath.Join(directory, offlineMigrationManifestName)
	raw, err := readBoundedRegularFile(manifestPath, maxOfflineMigrationManifestBytes)
	if err != nil {
		return fmt.Errorf("read release migration manifest: %w", err)
	}
	if digestBytes(raw) != expectedManifestSHA256 || len(raw) == 0 || raw[len(raw)-1] != '\n' {
		return fmt.Errorf("release migration manifest differs from embedded authority")
	}
	entries, err := parseMigrationManifest(raw)
	if err != nil {
		return err
	}
	directoryEntries, err := os.ReadDir(directory)
	if err != nil {
		return fmt.Errorf("read release migration directory: %w", err)
	}
	actualNames := make([]string, 0, len(directoryEntries))
	for _, entry := range directoryEntries {
		if entry.Name() == offlineMigrationManifestName {
			continue
		}
		if entry.Type()&os.ModeSymlink != 0 || entry.IsDir() || !offlineMigrationName.MatchString(entry.Name()) {
			return fmt.Errorf("release migration bundle contains unregistered entry %q", entry.Name())
		}
		actualNames = append(actualNames, entry.Name())
	}
	sort.Strings(actualNames)
	if len(actualNames) != len(entries) {
		return fmt.Errorf("release migration manifest does not cover the exact directory")
	}
	for index, entry := range entries {
		if actualNames[index] != entry.name {
			return fmt.Errorf("release migration manifest entry %q is absent", entry.name)
		}
		path := filepath.Join(directory, entry.name)
		fileRaw, err := readBoundedRegularFile(path, maxOfflineMigrationBytes)
		if err != nil {
			return fmt.Errorf("read release migration %q: %w", entry.name, err)
		}
		if digestBytes(fileRaw) != entry.sha256 {
			return fmt.Errorf("release migration %q differs from its manifest", entry.name)
		}
	}
	return nil
}

type migrationManifestEntry struct {
	sha256 string
	name   string
}

func parseMigrationManifest(raw []byte) ([]migrationManifestEntry, error) {
	scanner := bufio.NewScanner(strings.NewReader(string(raw)))
	entries := make([]migrationManifestEntry, 0)
	previous := ""
	for scanner.Scan() {
		line := scanner.Text()
		if len(line) < 67 || line[64:66] != "  " {
			return nil, fmt.Errorf("release migration manifest line is invalid")
		}
		entry := migrationManifestEntry{sha256: line[:64], name: line[66:]}
		if !validDigest(entry.sha256) || !offlineMigrationName.MatchString(entry.name) ||
			entry.name <= previous {
			return nil, fmt.Errorf("release migration manifest order or identity is invalid")
		}
		entries = append(entries, entry)
		previous = entry.name
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	if len(entries) == 0 {
		return nil, fmt.Errorf("release migration manifest is empty")
	}
	return entries, nil
}

func readBoundedRegularFile(path string, maximum int64) ([]byte, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil || !info.Mode().IsRegular() || info.Size() <= 0 || info.Size() > maximum {
		return nil, fmt.Errorf("file is not one bounded regular file")
	}
	return io.ReadAll(io.LimitReader(file, maximum+1))
}

func digestBytes(raw []byte) string {
	digest := sha256.Sum256(raw)
	return hex.EncodeToString(digest[:])
}
