package queue

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

const (
	MigrationManifestName     = "SHA256SUMS"
	maxMigrationManifestBytes = 1024 * 1024
	maxMigrationFileBytes     = 16 * 1024 * 1024
	maxMigrationBundleBytes   = 64 * 1024 * 1024
	maxMigrationBundleEntries = 1024
)

var migrationFileNamePattern = regexp.MustCompile(`^[0-9]{3}_[A-Za-z0-9_]+\.sql$`)

type migrationBundleEntry struct {
	name   string
	sha256 string
	body   []byte
}

// MigrationBundle is an immutable, manifest-authorized set of migration bytes.
// Its fields are intentionally private so callers cannot fabricate an install
// authority after the filesystem has been read.
type MigrationBundle struct {
	manifestSHA256 string
	manifest       []byte
	entries        []migrationBundleEntry
}

func (b MigrationBundle) ManifestSHA256() string { return b.manifestSHA256 }

func (b MigrationBundle) validate() error {
	if !validMigrationDigest(b.manifestSHA256) || len(b.manifest) == 0 ||
		digestMigrationBytes(b.manifest) != b.manifestSHA256 {
		return fmt.Errorf("migration bundle manifest authority is invalid")
	}
	parsed, err := parseMigrationManifest(b.manifest)
	if err != nil || len(parsed) != len(b.entries) {
		return fmt.Errorf("migration bundle manifest projection is invalid")
	}
	for index := range b.entries {
		entry := b.entries[index]
		if entry.name != parsed[index].name || entry.sha256 != parsed[index].sha256 ||
			len(entry.body) == 0 || len(entry.body) > maxMigrationFileBytes ||
			digestMigrationBytes(entry.body) != entry.sha256 {
			return fmt.Errorf("migration bundle entry %d is invalid", index)
		}
	}
	return nil
}

// LoadMigrationBundle reads one exact directory into memory once and verifies
// its complete manifest before any database lock or migration is attempted.
func LoadMigrationBundle(
	exactDirectory string,
	expectedManifestSHA256 string,
) (MigrationBundle, error) {
	return loadMigrationBundle(exactDirectory, expectedManifestSHA256, nil)
}

type migrationLoadHook func(name string)

func loadMigrationBundle(
	exactDirectory string,
	expectedManifestSHA256 string,
	afterInitialStat migrationLoadHook,
) (MigrationBundle, error) {
	if exactDirectory == "" || !filepath.IsAbs(exactDirectory) ||
		filepath.Clean(exactDirectory) != exactDirectory ||
		!validMigrationDigest(expectedManifestSHA256) {
		return MigrationBundle{}, fmt.Errorf("migration bundle requires one exact absolute directory and manifest digest")
	}
	directoryInfo, err := os.Lstat(exactDirectory)
	if err != nil || directoryInfo.Mode()&os.ModeSymlink != 0 || !directoryInfo.IsDir() {
		return MigrationBundle{}, fmt.Errorf("migration bundle directory is not one real directory")
	}
	root, err := os.OpenRoot(exactDirectory)
	if err != nil {
		return MigrationBundle{}, fmt.Errorf("open migration bundle directory: %w", err)
	}
	defer root.Close()
	openedDirectory, err := root.Stat(".")
	if err != nil || !os.SameFile(directoryInfo, openedDirectory) {
		return MigrationBundle{}, fmt.Errorf("migration bundle directory changed while opening")
	}

	manifest, manifestInfo, err := readMigrationBundleFile(
		root, MigrationManifestName, maxMigrationManifestBytes, afterInitialStat,
	)
	if err != nil {
		return MigrationBundle{}, fmt.Errorf("read migration manifest: %w", err)
	}
	if digestMigrationBytes(manifest) != expectedManifestSHA256 {
		return MigrationBundle{}, fmt.Errorf("migration manifest differs from expected authority")
	}
	registered, err := parseMigrationManifest(manifest)
	if err != nil {
		return MigrationBundle{}, err
	}
	names, err := migrationDirectoryNames(root)
	if err != nil || !sameMigrationEntrySet(names, registered) {
		return MigrationBundle{}, fmt.Errorf("migration manifest does not cover the exact directory")
	}

	entries := make([]migrationBundleEntry, 0, len(registered))
	infos := map[string]fs.FileInfo{MigrationManifestName: manifestInfo}
	total := 0
	for _, expected := range registered {
		body, info, err := readMigrationBundleFile(
			root, expected.name, maxMigrationFileBytes, afterInitialStat,
		)
		if err != nil {
			return MigrationBundle{}, fmt.Errorf("read migration %q: %w", expected.name, err)
		}
		total += len(body)
		if total > maxMigrationBundleBytes || digestMigrationBytes(body) != expected.sha256 ||
			strings.TrimSpace(string(body)) == "" {
			return MigrationBundle{}, fmt.Errorf("migration %q differs from its bounded manifest entry", expected.name)
		}
		entries = append(entries, migrationBundleEntry{
			name: expected.name, sha256: expected.sha256, body: append([]byte{}, body...),
		})
		infos[expected.name] = info
	}
	if err := verifyMigrationBundleUnchanged(exactDirectory, root, directoryInfo, infos, registered); err != nil {
		return MigrationBundle{}, err
	}
	bundle := MigrationBundle{
		manifestSHA256: expectedManifestSHA256,
		manifest:       append([]byte{}, manifest...),
		entries:        entries,
	}
	if err := bundle.validate(); err != nil {
		return MigrationBundle{}, err
	}
	return bundle, nil
}

type migrationManifestEntry struct{ name, sha256 string }

func parseMigrationManifest(raw []byte) ([]migrationManifestEntry, error) {
	if len(raw) == 0 || len(raw) > maxMigrationManifestBytes || raw[len(raw)-1] != '\n' {
		return nil, fmt.Errorf("migration manifest must be one bounded newline-terminated file")
	}
	scanner := bufio.NewScanner(strings.NewReader(string(raw)))
	scanner.Buffer(make([]byte, 1024), maxMigrationManifestBytes)
	entries := make([]migrationManifestEntry, 0)
	previous := ""
	previousNumber := 0
	for scanner.Scan() {
		line := scanner.Text()
		if len(line) < 67 || line[64:66] != "  " {
			return nil, fmt.Errorf("migration manifest line is invalid")
		}
		entry := migrationManifestEntry{name: line[66:], sha256: line[:64]}
		if len(entry.name) > 255 || !migrationFileNamePattern.MatchString(entry.name) ||
			!validMigrationDigest(entry.sha256) || entry.name <= previous {
			return nil, fmt.Errorf("migration manifest order or identity is invalid")
		}
		number, err := strconv.Atoi(entry.name[:3])
		if err != nil || number < 1 ||
			(previousNumber == 0 && number != 1) ||
			(previousNumber > 0 && number != previousNumber && number != previousNumber+1) {
			return nil, fmt.Errorf("migration manifest has a missing numeric migration prefix")
		}
		entries = append(entries, entry)
		previous = entry.name
		previousNumber = number
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("scan migration manifest: %w", err)
	}
	if len(entries) == 0 || len(entries) > maxMigrationBundleEntries {
		return nil, fmt.Errorf("migration manifest entry count is invalid")
	}
	return entries, nil
}

func readMigrationBundleFile(
	root *os.Root,
	name string,
	maximum int64,
	afterInitialStat migrationLoadHook,
) ([]byte, fs.FileInfo, error) {
	initial, err := root.Lstat(name)
	if err != nil || initial.Mode()&os.ModeSymlink != 0 || !initial.Mode().IsRegular() ||
		initial.Size() <= 0 || initial.Size() > maximum {
		return nil, nil, fmt.Errorf("file is not one bounded regular file")
	}
	if afterInitialStat != nil {
		afterInitialStat(name)
	}
	file, err := root.Open(name)
	if err != nil {
		return nil, nil, err
	}
	defer file.Close()
	opened, err := file.Stat()
	if err != nil || !opened.Mode().IsRegular() || !os.SameFile(initial, opened) {
		return nil, nil, fmt.Errorf("file changed while opening")
	}
	raw, err := io.ReadAll(io.LimitReader(file, maximum+1))
	if err != nil || int64(len(raw)) > maximum {
		return nil, nil, fmt.Errorf("read bounded file: %w", err)
	}
	return raw, opened, nil
}

func migrationDirectoryNames(root *os.Root) ([]string, error) {
	directory, err := root.Open(".")
	if err != nil {
		return nil, err
	}
	defer directory.Close()
	entries, err := directory.ReadDir(maxMigrationBundleEntries + 2)
	if err != nil {
		return nil, err
	}
	if len(entries) == 0 || len(entries) > maxMigrationBundleEntries+1 {
		return nil, fmt.Errorf("migration directory entry count is invalid")
	}
	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		if entry.Name() != MigrationManifestName && !migrationFileNamePattern.MatchString(entry.Name()) {
			return nil, fmt.Errorf("migration directory contains invalid entry %q", entry.Name())
		}
		if entry.IsDir() || entry.Type()&os.ModeSymlink != 0 {
			return nil, fmt.Errorf("migration directory entry %q is not a regular file", entry.Name())
		}
		names = append(names, entry.Name())
	}
	sort.Strings(names)
	return names, nil
}

func sameMigrationEntrySet(names []string, entries []migrationManifestEntry) bool {
	if len(names) != len(entries)+1 {
		return false
	}
	expected := []string{MigrationManifestName}
	for _, entry := range entries {
		expected = append(expected, entry.name)
	}
	sort.Strings(expected)
	return strings.Join(names, "\x00") == strings.Join(expected, "\x00")
}

func verifyMigrationBundleUnchanged(
	exactDirectory string,
	root *os.Root,
	directoryInfo fs.FileInfo,
	infos map[string]fs.FileInfo,
	entries []migrationManifestEntry,
) error {
	currentDirectory, err := os.Lstat(exactDirectory)
	if err != nil || currentDirectory.Mode()&os.ModeSymlink != 0 ||
		!os.SameFile(directoryInfo, currentDirectory) {
		return fmt.Errorf("migration bundle directory changed while loading")
	}
	names, err := migrationDirectoryNames(root)
	if err != nil || !sameMigrationEntrySet(names, entries) {
		return fmt.Errorf("migration bundle entries changed while loading")
	}
	for name, initial := range infos {
		current, err := root.Lstat(name)
		if err != nil || current.Mode()&os.ModeSymlink != 0 || !current.Mode().IsRegular() ||
			!os.SameFile(initial, current) || current.Size() != initial.Size() ||
			!current.ModTime().Equal(initial.ModTime()) || current.Mode() != initial.Mode() {
			return fmt.Errorf("migration bundle file %q changed while loading", name)
		}
	}
	return nil
}

func validMigrationDigest(value string) bool {
	if len(value) != sha256.Size*2 || value != strings.ToLower(value) {
		return false
	}
	_, err := hex.DecodeString(value)
	return err == nil
}

func digestMigrationBytes(raw []byte) string {
	digest := sha256.Sum256(raw)
	return hex.EncodeToString(digest[:])
}
