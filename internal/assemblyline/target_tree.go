package assemblyline

import (
	"fmt"
	"path"
	"strings"
	"unicode/utf8"
)

const (
	maxTargetTreePaths = 128
	// MaxTargetTreePathBytes and MaxTargetTreeDepth are shared by target-tree
	// input validation, raw parsing/rendering, and response-bound derivation.
	MaxTargetTreePathBytes      = 512
	MaxTargetTreeDepth          = 16
	maxTargetTreePathBytes      = MaxTargetTreePathBytes
)

type TargetArtifactKind string

const (
	TargetArtifactImplementation TargetArtifactKind = "implementation"
	TargetArtifactVerification   TargetArtifactKind = "verification"
)

type TargetTree struct {
	Paths []string
}

func validateTargetTreePaths(label string, paths []string) error {
	if len(paths) > maxTargetTreePaths {
		return fmt.Errorf("target tree %s set exceeds %d paths", label, maxTargetTreePaths)
	}
	seen := make(map[string]struct{}, len(paths))
	for index, value := range paths {
		if err := validateTargetTreePath(value); err != nil {
			return fmt.Errorf("target tree %s %d: %w", label, index, err)
		}
		if _, duplicate := seen[value]; duplicate {
			return fmt.Errorf("target tree %s %d duplicates %q", label, index, value)
		}
		seen[value] = struct{}{}
	}
	return nil
}

func validateTargetTreePath(value string) error {
	if value == "" || len(value) > maxTargetTreePathBytes || value != path.Clean(value) ||
		strings.HasPrefix(value, "/") || value == "." || strings.HasPrefix(value, "../") ||
		strings.Contains(value, "\\") {
		return fmt.Errorf("path must be one normalized relative slash path")
	}
	names := strings.Split(value, "/")
	if len(names) > MaxTargetTreeDepth {
		return fmt.Errorf("path exceeds the target-tree depth limit of %d", MaxTargetTreeDepth)
	}
	for _, name := range names {
		if err := validateTargetTreeBasename(name); err != nil {
			return err
		}
	}
	return nil
}

func validateTargetTreeBasename(value string) error {
	if value == "" || value == "." || value == ".." || len(value) > 255 ||
		!utf8.ValidString(value) || strings.ContainsAny(value, "/\\\x00\r\n") {
		return fmt.Errorf("path contains an invalid basename")
	}
	return nil
}

func validateTargetTreeText(label, value string, maximum int) error {
	if value == "" || value != strings.TrimSpace(value) || !utf8.ValidString(value) ||
		strings.ContainsRune(value, '\x00') || len(value) > maximum {
		return fmt.Errorf("%s must be trimmed UTF-8 text of at most %d bytes", label, maximum)
	}
	return nil
}
