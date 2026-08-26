package worker

import (
	"fmt"
	"regexp"
)

var v3CargoNamePattern = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_-]*$`)

func validateV3Cargo(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("command.run requires an exact Cargo operation")
	}
	for _, exact := range [][]string{
		{"build"},
		{"check"},
		{"test"},
		{"fmt", "--check"},
		{"build", "--locked", "--offline"},
		{"check", "--locked", "--offline", "--all-targets"},
		{"test", "--locked", "--offline"},
		{"clippy", "--locked", "--offline", "--all-targets", "--", "-D", "warnings"},
	} {
		if slicesEqualStrings(args, exact) {
			return nil
		}
	}
	if len(args) == 5 && slicesEqualStrings(args[:4], []string{
		"test", "--locked", "--offline", "--test",
	}) && v3CargoNamePattern.MatchString(args[4]) {
		return nil
	}
	if args[0] == "init" {
		return validateV3CargoInit(args)
	}
	return fmt.Errorf("command.run permits only exact code-owned Cargo verification operations")
}

func validateV3CargoInit(args []string) error {
	seenKind := false
	seenName := false
	seenVCS := false
	seenEdition := false
	seenTarget := false
	for index := 1; index < len(args); index++ {
		switch args[index] {
		case "--bin", "--lib":
			if seenKind {
				return fmt.Errorf("command.run cargo init requires exactly one project kind")
			}
			seenKind = true
		case "--name":
			if seenName || index+1 >= len(args) || !v3CargoNamePattern.MatchString(args[index+1]) {
				return fmt.Errorf("command.run cargo init requires a valid unique --name value")
			}
			seenName = true
			index++
		case "--vcs":
			if seenVCS || index+1 >= len(args) || args[index+1] != "none" {
				return fmt.Errorf("command.run cargo init permits only --vcs none")
			}
			seenVCS = true
			index++
		case "--edition":
			if seenEdition || index+1 >= len(args) || !directCodingRegisteredRustEdition(args[index+1]) {
				return fmt.Errorf("command.run cargo init requires one registered Rust edition")
			}
			seenEdition = true
			index++
		case ".":
			if seenTarget {
				return fmt.Errorf("command.run cargo init accepts one current-directory target")
			}
			seenTarget = true
		default:
			return fmt.Errorf("command.run cargo init argument %q is not allowlisted", args[index])
		}
	}
	if !seenKind || !seenName || !seenVCS || !seenEdition || !seenTarget {
		return fmt.Errorf("command.run cargo init requires kind, name, --vcs none, one registered edition, and current-directory target")
	}
	return nil
}

func directCodingRegisteredRustEdition(edition string) bool {
	for _, profile := range registeredDirectCodingProjectVersionProfiles() {
		if profile.StackID != genericRustCommandLineAdapter {
			continue
		}
		value, err := directCodingVersionComponent(profile, "rust_edition")
		if err == nil && value == edition {
			return true
		}
	}
	return false
}
