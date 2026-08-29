package worker

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
	"unicode/utf8"
)

const (
	directCodingProjectEnvironmentWorkdir        = "/workspace"
	directCodingDockerEnvironmentAuthoritySchema = "omnidex.project-environment.v1"
	maxDirectCodingEnvironmentDockerfileBytes    = 32 * 1024
	maxDirectCodingProjectEnvironmentCommandArgs = 64
)

var (
	directCodingEnvironmentIDPattern      = regexp.MustCompile(`^[a-z][a-z0-9_]{0,95}$`)
	directCodingEnvironmentProgramPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9._+-]{0,63}$`)
	directCodingEnvironmentImageIDPattern = regexp.MustCompile(`^sha256:[0-9a-f]{64}$`)
	directCodingEnvironmentShellPrograms  = map[string]struct{}{
		"ash": {}, "bash": {}, "cmd": {}, "dash": {}, "fish": {}, "ksh": {},
		"powershell": {}, "pwsh": {}, "sh": {}, "zsh": {},
	}
)

// directCodingDockerEnvironmentSpec is immutable code-owned authority for one
// bounded project verification image. Docker is the sole external executor;
// Programs are argv[0] values permitted inside that image.
type directCodingDockerEnvironmentSpec struct {
	ID                string
	Dockerfile        string
	Programs          []string
	WorkspaceReadOnly bool
}

func (spec directCodingDockerEnvironmentSpec) validate() error {
	if !directCodingEnvironmentIDPattern.MatchString(spec.ID) {
		return fmt.Errorf("project development environment has an invalid ID %q", spec.ID)
	}
	if err := validateDirectCodingEnvironmentDockerfile(spec.Dockerfile); err != nil {
		return fmt.Errorf("project development environment %s Dockerfile: %w", spec.ID, err)
	}
	if !spec.WorkspaceReadOnly {
		return fmt.Errorf("project development environment %s requires a read-only workspace bind", spec.ID)
	}
	if !sort.StringsAreSorted(spec.Programs) {
		return fmt.Errorf("project development environment %s programs must be ordered and unique", spec.ID)
	}
	last := ""
	for _, program := range spec.Programs {
		if !directCodingEnvironmentProgramPattern.MatchString(program) || program != strings.ToLower(program) {
			return fmt.Errorf("project development environment %s has invalid program %q", spec.ID, program)
		}
		if _, shell := directCodingEnvironmentShellPrograms[program]; shell {
			return fmt.Errorf("project development environment %s cannot register shell program %q", spec.ID, program)
		}
		if program == last {
			return fmt.Errorf("project development environment %s programs must be ordered and unique", spec.ID)
		}
		last = program
	}
	return nil
}

// directCodingDockerEnvironmentAuthority is the complete durable identity of
// one built project environment. Recovery validates every field and rebuilds
// from this authority; it never consults a current binary default.
type directCodingDockerEnvironmentAuthority struct {
	Schema            string   `json:"schema"`
	ID                string   `json:"id"`
	Dockerfile        string   `json:"dockerfile"`
	Programs          []string `json:"programs"`
	WorkspaceReadOnly bool     `json:"workspace_read_only"`
	AuthoritySHA256   string   `json:"authority_sha256"`
	ImageID           string   `json:"image_id"`
}

func newDirectCodingDockerEnvironmentAuthority(
	spec directCodingDockerEnvironmentSpec,
	imageID string,
) (*directCodingDockerEnvironmentAuthority, error) {
	if err := spec.validate(); err != nil {
		return nil, err
	}
	if !directCodingEnvironmentImageIDPattern.MatchString(imageID) {
		return nil, fmt.Errorf("project development environment has invalid immutable image ID %q", imageID)
	}
	digest, err := directCodingDockerEnvironmentAuthorityDigest(spec)
	if err != nil {
		return nil, err
	}
	return &directCodingDockerEnvironmentAuthority{
		Schema: directCodingDockerEnvironmentAuthoritySchema,
		ID:     spec.ID, Dockerfile: spec.Dockerfile,
		Programs:          append([]string(nil), spec.Programs...),
		WorkspaceReadOnly: spec.WorkspaceReadOnly,
		AuthoritySHA256:   digest, ImageID: imageID,
	}, nil
}

func (authority *directCodingDockerEnvironmentAuthority) validate() error {
	if authority == nil || authority.Schema != directCodingDockerEnvironmentAuthoritySchema {
		return fmt.Errorf("project development environment authority has an invalid schema")
	}
	spec := authority.spec()
	if err := spec.validate(); err != nil {
		return err
	}
	if !directCodingEnvironmentImageIDPattern.MatchString(authority.ImageID) {
		return fmt.Errorf("project development environment authority has an invalid immutable image ID")
	}
	digest, err := directCodingDockerEnvironmentAuthorityDigest(spec)
	if err != nil {
		return err
	}
	if authority.AuthoritySHA256 != digest {
		return fmt.Errorf("project development environment authority digest differs from its exact definition")
	}
	return nil
}

func (authority *directCodingDockerEnvironmentAuthority) spec() directCodingDockerEnvironmentSpec {
	if authority == nil {
		return directCodingDockerEnvironmentSpec{}
	}
	return directCodingDockerEnvironmentSpec{
		ID: authority.ID, Dockerfile: authority.Dockerfile,
		Programs:          append([]string(nil), authority.Programs...),
		WorkspaceReadOnly: authority.WorkspaceReadOnly,
	}
}

func cloneDirectCodingDockerEnvironmentAuthority(
	authority *directCodingDockerEnvironmentAuthority,
) *directCodingDockerEnvironmentAuthority {
	if authority == nil {
		return nil
	}
	clone := *authority
	clone.Programs = append([]string(nil), authority.Programs...)
	return &clone
}

func directCodingDockerEnvironmentAuthorityDigest(
	spec directCodingDockerEnvironmentSpec,
) (string, error) {
	if err := spec.validate(); err != nil {
		return "", err
	}
	canonical, err := json.Marshal(struct {
		Schema            string   `json:"schema"`
		ID                string   `json:"id"`
		Dockerfile        string   `json:"dockerfile"`
		Programs          []string `json:"programs"`
		WorkspaceReadOnly bool     `json:"workspace_read_only"`
	}{
		Schema: directCodingDockerEnvironmentAuthoritySchema,
		ID:     spec.ID, Dockerfile: spec.Dockerfile,
		Programs: spec.Programs, WorkspaceReadOnly: spec.WorkspaceReadOnly,
	})
	if err != nil {
		return "", fmt.Errorf("encode canonical project development environment authority: %w", err)
	}
	digest := sha256.Sum256(canonical)
	return hex.EncodeToString(digest[:]), nil
}

func validateDirectCodingEnvironmentDockerfile(content string) error {
	if content == "" || len(content) > maxDirectCodingEnvironmentDockerfileBytes ||
		!utf8.ValidString(content) || strings.ContainsAny(content, "\x00\r") ||
		!strings.HasSuffix(content, "\n") {
		return fmt.Errorf("must be bounded normalized UTF-8 ending in one newline")
	}
	fromCount := 0
	for number, raw := range strings.Split(strings.TrimSuffix(content, "\n"), "\n") {
		line := strings.TrimSpace(raw)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		fields := strings.Fields(line)
		instruction := strings.ToUpper(fields[0])
		switch instruction {
		case "FROM":
			if len(fields) != 2 && (len(fields) != 4 || !strings.EqualFold(fields[2], "AS")) {
				return fmt.Errorf("line %d has unsupported FROM syntax", number+1)
			}
			if !directCodingPinnedContainerImagePattern.MatchString(fields[1]) {
				return fmt.Errorf("line %d FROM image must be digest-pinned", number+1)
			}
			fromCount++
		case "ADD":
			return fmt.Errorf("line %d cannot consume local build-context input", number+1)
		case "COPY":
			if len(fields) < 4 || !strings.HasPrefix(fields[1], "--from=") {
				return fmt.Errorf("line %d cannot consume local build-context input", number+1)
			}
		}
	}
	if fromCount == 0 {
		return fmt.Errorf("requires at least one digest-pinned FROM image")
	}
	return nil
}

type directCodingDockerWorkspaceMapping struct {
	RuntimeRoot string
	HostRoot    string
}

func (mapping directCodingDockerWorkspaceMapping) validate() error {
	for _, candidate := range []struct {
		name string
		path string
	}{{"runtime", mapping.RuntimeRoot}, {"host", mapping.HostRoot}} {
		if candidate.path == "" || !filepath.IsAbs(candidate.path) ||
			filepath.Clean(candidate.path) != candidate.path || strings.TrimSpace(candidate.path) != candidate.path {
			return fmt.Errorf("project Docker %s workspace root must be one normalized absolute path", candidate.name)
		}
		if strings.ContainsAny(candidate.path, "\x00\r\n,") {
			return fmt.Errorf("project Docker %s workspace root contains a Docker mount delimiter", candidate.name)
		}
		if err := validateV3CommandRoot(candidate.path); err != nil {
			return fmt.Errorf("project Docker %s workspace root is not an exact directory: %w", candidate.name, err)
		}
	}
	runtimeInfo, err := os.Lstat(mapping.RuntimeRoot)
	if err != nil {
		return fmt.Errorf("inspect project Docker runtime workspace: %w", err)
	}
	hostInfo, err := os.Lstat(mapping.HostRoot)
	if err != nil {
		return fmt.Errorf("inspect project Docker host workspace: %w", err)
	}
	if !os.SameFile(runtimeInfo, hostInfo) {
		return fmt.Errorf("project Docker runtime and host roots do not identify the same directory")
	}
	return nil
}

type directCodingProjectEnvironmentCommand struct {
	Program string
	Args    []string
	Timeout time.Duration
}

func (command directCodingProjectEnvironmentCommand) validate(
	spec directCodingDockerEnvironmentSpec,
) error {
	if _, shell := directCodingEnvironmentShellPrograms[command.Program]; shell {
		return fmt.Errorf("project environment command cannot use shell program %q", command.Program)
	}
	if !directCodingEnvironmentProgramPattern.MatchString(command.Program) ||
		command.Program != strings.ToLower(command.Program) {
		return fmt.Errorf("project environment command requires one bare registered program")
	}
	index := sort.SearchStrings(spec.Programs, command.Program)
	if index == len(spec.Programs) || spec.Programs[index] != command.Program {
		return fmt.Errorf("project environment program %q is not registered by %s", command.Program, spec.ID)
	}
	if len(command.Args) > maxDirectCodingProjectEnvironmentCommandArgs {
		return fmt.Errorf("project environment command exceeds the %d-argument limit", maxDirectCodingProjectEnvironmentCommandArgs)
	}
	for _, arg := range command.Args {
		if strings.ContainsAny(arg, "\x00\r\n") {
			return fmt.Errorf("project environment command arguments cannot contain NUL or newlines")
		}
	}
	if command.Timeout <= 0 || command.Timeout > maxV3CommandLimit {
		return fmt.Errorf("project environment command timeout must be between 1ns and %s", maxV3CommandLimit)
	}
	return nil
}
