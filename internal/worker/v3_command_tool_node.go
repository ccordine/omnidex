package worker

import (
	"fmt"
	"path/filepath"
	"strconv"
	"strings"
)

func directCodingNodeSandboxPrefix() []string {
	return []string{
		"--permission",
		"--allow-fs-read=.",
		"--disable-proto=throw",
	}
}

func javaScriptNodeTestArgs() []string {
	args := directCodingNodeSandboxPrefix()
	return append(args, "--test-isolation=none", "--test")
}

func javaScriptNodeCheckArgs(artifactPath string) []string {
	args := directCodingNodeSandboxPrefix()
	return append(args, "--check", artifactPath)
}

func typeScriptScopeInspectorNodeArgs(
	artifactPath string,
	line int,
	column int,
	blockStartLine int,
	blockEndLine int,
) []string {
	args := directCodingNodeSandboxPrefix()
	return append(
		args,
		directCodingTypeScriptScopeInspectorFile,
		artifactPath,
		strconv.Itoa(line),
		strconv.Itoa(column),
		strconv.Itoa(blockStartLine),
		strconv.Itoa(blockEndLine),
	)
}

func validateV3Node(args []string) error {
	if slicesEqualStrings(args, javaScriptNodeTestArgs()) {
		return nil
	}
	if len(args) == len(directCodingNodeSandboxPrefix())+2 &&
		slicesEqualStrings(args[:len(args)-2], directCodingNodeSandboxPrefix()) &&
		args[len(args)-2] == "--check" && javascriptArtifactPath(args[len(args)-1]) {
		return nil
	}
	if validTypeScriptScopeInspectorNodeArgs(args) {
		return nil
	}
	return fmt.Errorf(
		"command.run permits node only through the code-owned permission sandbox for tests, one JavaScript syntax check, or the exact TypeScript scope inspector",
	)
}

func validTypeScriptScopeInspectorNodeArgs(args []string) bool {
	prefix := directCodingNodeSandboxPrefix()
	if len(args) != len(prefix)+6 || !slicesEqualStrings(args[:len(prefix)], prefix) {
		return false
	}
	command := args[len(prefix):]
	if command[0] != directCodingTypeScriptScopeInspectorFile ||
		!typeScriptScopeInspectorArtifactPath(command[1]) {
		return false
	}
	coordinates := make([]int, 4)
	for index, raw := range command[2:] {
		value, err := strconv.Atoi(raw)
		if err != nil || value < 1 || strconv.Itoa(value) != raw {
			return false
		}
		coordinates[index] = value
	}
	line, blockStartLine, blockEndLine := coordinates[0], coordinates[2], coordinates[3]
	return blockEndLine >= blockStartLine && line >= blockStartLine && line <= blockEndLine
}

func typeScriptScopeInspectorArtifactPath(value string) bool {
	clean, err := normalizeDirectCodingPath(value)
	if err != nil || clean != value {
		return false
	}
	switch strings.ToLower(filepath.Ext(clean)) {
	case ".ts", ".tsx":
		return true
	default:
		return false
	}
}
