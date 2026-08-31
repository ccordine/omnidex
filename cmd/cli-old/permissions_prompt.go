package main

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"sort"
	"strings"
)

func promptPermissionDecision(key, reason, storePath string) (bool, error) {
	reason = strings.TrimSpace(reason)
	description := strings.TrimSpace(knownPermissionDescriptions[key])
	if fn := currentPermissionPromptFunc(); fn != nil {
		return fn(key, reason, storePath, description)
	}

	reader, writer, closer, err := openPermissionPromptIO()
	if err != nil {
		return false, fmt.Errorf("permission prompt unavailable for %s: %w (grant manually with `omni permissions grant %s`)", key, err, key)
	}
	if closer != nil {
		defer closer.Close()
	}

	for {
		fmt.Fprintln(writer, "permission required:")
		fmt.Fprintf(writer, "  key: %s\n", key)
		if description != "" {
			fmt.Fprintf(writer, "  description: %s\n", description)
		}
		if reason != "" {
			fmt.Fprintf(writer, "  reason: %s\n", reason)
		}
		fmt.Fprintf(writer, "  store: %s\n", storePath)
		fmt.Fprint(writer, "allow and save this permission? [y/n]: ")
		if err := writer.Flush(); err != nil {
			return false, err
		}

		line, err := reader.ReadString('\n')
		if err != nil {
			return false, err
		}
		switch strings.ToLower(strings.TrimSpace(line)) {
		case "y", "yes":
			return true, nil
		case "n", "no":
			return false, nil
		default:
			fmt.Fprintln(writer, "please answer y or n")
			_ = writer.Flush()
		}
	}
}

func openPermissionPromptIO() (*bufio.Reader, *bufio.Writer, *os.File, error) {
	if tty, err := os.OpenFile("/dev/tty", os.O_RDWR, 0); err == nil {
		return bufio.NewReader(tty), bufio.NewWriter(tty), tty, nil
	}

	if isCharDevice(os.Stdin) && isCharDevice(os.Stdout) {
		return bufio.NewReader(os.Stdin), bufio.NewWriter(os.Stdout), nil, nil
	}
	return nil, nil, nil, errors.New("no interactive terminal available")
}

func isCharDevice(file *os.File) bool {
	if file == nil {
		return false
	}
	info, err := file.Stat()
	if err != nil {
		return false
	}
	return (info.Mode() & os.ModeCharDevice) != 0
}

func runPermissions(args []string) {
	subcommand := "list"
	rest := args
	if len(args) > 0 {
		subcommand = strings.ToLower(strings.TrimSpace(args[0]))
		rest = args[1:]
	}

	pm := getPermissionManager()
	switch subcommand {
	case "", "list", "show":
		path, entries, err := pm.List()
		if err != nil {
			die(err.Error())
		}
		printPermissionEntries(path, entries)
		return
	case "path":
		path, _, err := pm.List()
		if err != nil {
			die(err.Error())
		}
		fmt.Println(path)
		return
	case "grant", "allow":
		if len(rest) < 1 {
			die("permissions grant requires <key>")
		}
		key := strings.TrimSpace(rest[0])
		reason := ""
		if len(rest) > 1 {
			reason = strings.TrimSpace(strings.Join(rest[1:], " "))
		}
		if err := pm.Set(key, true, reason); err != nil {
			die(err.Error())
		}
		fmt.Printf("permission granted: %s\n", key)
		return
	case "deny":
		if len(rest) < 1 {
			die("permissions deny requires <key>")
		}
		key := strings.TrimSpace(rest[0])
		reason := ""
		if len(rest) > 1 {
			reason = strings.TrimSpace(strings.Join(rest[1:], " "))
		}
		if err := pm.Set(key, false, reason); err != nil {
			die(err.Error())
		}
		fmt.Printf("permission denied: %s\n", key)
		return
	case "unset", "remove", "delete":
		if len(rest) < 1 {
			die("permissions unset requires <key>")
		}
		key := strings.TrimSpace(rest[0])
		if err := pm.Unset(key); err != nil {
			die(err.Error())
		}
		fmt.Printf("permission removed: %s\n", key)
		return
	case "reset", "clear":
		if err := pm.Reset(); err != nil {
			die(err.Error())
		}
		fmt.Println("all saved permissions cleared")
		return
	case "help":
		printPermissionsHelp()
		return
	default:
		die("unknown permissions command. use `omni permissions help`")
	}
}

func printPermissionsHelp() {
	fmt.Println("permissions commands:")
	fmt.Println("  permissions list")
	fmt.Println("  permissions path")
	fmt.Println("  permissions grant <key> [reason]")
	fmt.Println("  permissions deny <key> [reason]")
	fmt.Println("  permissions unset <key>")
	fmt.Println("  permissions reset")
	fmt.Println("")
	fmt.Println("known permission keys:")
	keys := sortedKnownPermissionKeys()
	for _, key := range keys {
		fmt.Printf("  %s  %s\n", key, knownPermissionDescriptions[key])
	}
}

func printPermissionEntries(path string, entries map[string]permissionDecision) {
	fmt.Printf("permissions_store=%s\n", path)
	if len(entries) == 0 {
		fmt.Println("no saved permissions")
		return
	}

	keys := make([]string, 0, len(entries))
	for key := range entries {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	for _, key := range keys {
		decision := entries[key]
		state := "deny"
		if decision.Allowed {
			state = "allow"
		}
		line := fmt.Sprintf("%s=%s", key, state)
		if strings.TrimSpace(decision.UpdatedAt) != "" {
			line += " updated_at=" + strings.TrimSpace(decision.UpdatedAt)
		}
		if strings.TrimSpace(decision.Reason) != "" {
			line += " reason=" + strings.TrimSpace(decision.Reason)
		}
		if description := strings.TrimSpace(knownPermissionDescriptions[key]); description != "" {
			line += " description=" + description
		}
		fmt.Println(line)
	}
}

func sortedKnownPermissionKeys() []string {
	keys := make([]string, 0, len(knownPermissionDescriptions))
	for key := range knownPermissionDescriptions {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}
