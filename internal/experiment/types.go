// Package experiment executes code-owned commands against exact input files in
// disposable Docker containers. It has no model, host workspace, or tool-selection
// interface. Callers own image acquisition, validation, evidence, and publication.
package experiment

import (
	"fmt"
	"path"
	"regexp"
	"sort"
	"strings"
	"time"
	"unicode/utf8"
)

const (
	WorkingDirectory = "/workspace"
	MaxFiles         = 4096
	MaxFileBytes     = 32 * 1024 * 1024
	MaxInputBytes    = 256 * 1024 * 1024
	MaxArchiveBytes  = 512 * 1024 * 1024
	MaxStreamBytes   = 1024 * 1024
)

type File struct {
	Path    string
	Content []byte
	Mode    uint32
}

type Request struct {
	// ImageID is the exact image identity already observed by code. Run never
	// resolves a floating tag, pulls an image, or chooses another toolchain.
	ImageID     string
	Argv        []string
	Environment []string
	Stdin       []byte
	Input       []File
	Collect     []string
	Timeout     time.Duration
}

type Result struct {
	ContainerID      string
	ExecutionID      string
	NetworkEnabled   *bool
	ImageID          string
	Argv             []string
	Environment      []string
	WorkingDirectory string
	StartedAt        time.Time
	FinishedAt       time.Time
	ExitCode         *int
	Stdout           []byte
	Stderr           []byte
	StdoutComplete   bool
	StderrComplete   bool
	Files            []File
}

var imageIDPattern = regexp.MustCompile(`^sha256:[0-9a-f]{64}$`)
var containerIDPattern = regexp.MustCompile(`^[0-9a-f]{64}$`)
var environmentNamePattern = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

func validateRequest(request Request) error {
	if !imageIDPattern.MatchString(request.ImageID) {
		return fmt.Errorf("experiment requires one observed immutable Docker image ID")
	}
	if request.Timeout <= 0 || request.Timeout > 10*time.Minute {
		return fmt.Errorf("experiment timeout must be positive and at most ten minutes")
	}
	if len(request.Argv) == 0 || len(request.Argv) > 128 || request.Argv[0] == "" {
		return fmt.Errorf("experiment requires one bounded exact command")
	}
	bytes := 0
	for _, argument := range request.Argv {
		if !validText(argument) {
			return fmt.Errorf("experiment command contains invalid text")
		}
		bytes += len(argument)
	}
	if bytes > 64*1024 || len(request.Stdin) > MaxStreamBytes {
		return fmt.Errorf("experiment command or stdin exceeds its byte bound")
	}
	if len(request.Environment) > 64 || !sort.StringsAreSorted(request.Environment) {
		return fmt.Errorf("experiment environment must be bounded and sorted")
	}
	names := make(map[string]struct{})
	bytes = 0
	for _, entry := range request.Environment {
		name, _, found := strings.Cut(entry, "=")
		if !found || !environmentNamePattern.MatchString(name) || !validText(entry) {
			return fmt.Errorf("experiment environment contains an invalid value")
		}
		if _, duplicate := names[name]; duplicate {
			return fmt.Errorf("experiment environment repeats %s", name)
		}
		names[name] = struct{}{}
		bytes += len(entry)
	}
	if bytes > 64*1024 {
		return fmt.Errorf("experiment environment exceeds its byte bound")
	}
	if len(request.Input) > MaxFiles || len(request.Collect) > MaxFiles {
		return fmt.Errorf("experiment exceeds its file count bound")
	}
	input := make(map[string]struct{})
	bytes = 0
	for _, file := range request.Input {
		if err := validatePath(file.Path); err != nil {
			return err
		}
		if _, duplicate := input[file.Path]; duplicate {
			return fmt.Errorf("experiment repeats input file %q", file.Path)
		}
		input[file.Path] = struct{}{}
		if file.Mode == 0 || file.Mode&^uint32(0o777) != 0 || len(file.Content) > MaxFileBytes {
			return fmt.Errorf("experiment input %q has invalid permissions or size", file.Path)
		}
		if bytes > MaxInputBytes-len(file.Content) {
			return fmt.Errorf("experiment input exceeds its total byte bound")
		}
		bytes += len(file.Content)
	}
	for name := range input {
		for parent := path.Dir(name); parent != "."; parent = path.Dir(parent) {
			if _, file := input[parent]; file {
				return fmt.Errorf("experiment input file %q is also a directory", parent)
			}
		}
	}
	collected := make(map[string]struct{})
	for _, name := range request.Collect {
		if err := validatePath(name); err != nil {
			return err
		}
		if _, duplicate := collected[name]; duplicate {
			return fmt.Errorf("experiment repeats collected file %q", name)
		}
		collected[name] = struct{}{}
	}
	for name := range collected {
		for parent := path.Dir(name); parent != "."; parent = path.Dir(parent) {
			if _, file := collected[parent]; file {
				return fmt.Errorf("collected experiment file %q is also a directory", parent)
			}
		}
	}
	return nil
}

func validText(value string) bool {
	return utf8.ValidString(value) && !strings.ContainsRune(value, 0)
}

func validatePath(value string) error {
	if !validText(value) || value == "" || value != strings.TrimSpace(value) || len(value) > 4096 ||
		strings.Contains(value, "\\") || path.IsAbs(value) || path.Clean(value) != value || value == "." ||
		value == ".." || strings.HasPrefix(value, "../") {
		return fmt.Errorf("experiment path %q must be one exact relative slash path", value)
	}
	for _, segment := range strings.Split(value, "/") {
		if len(segment) > 255 {
			return fmt.Errorf("experiment path %q contains an oversized basename", value)
		}
	}
	return nil
}
