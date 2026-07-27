package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func createLocalFile(target string) (string, error) {
	target = cleanShellToken(target)
	if target == "" {
		target = "test"
	}
	if err := validateRelativePath(target); err != nil {
		return "", err
	}
	parentDir := filepath.Clean(filepath.Dir(target))
	createdParent := parentDir != "." && parentDir != ""
	existedBefore := false
	if info, err := os.Stat(target); err == nil && !info.IsDir() {
		existedBefore = true
	}
	before := captureRepoWorkingTreeSnapshot()
	lines := make([]string, 0, 4)
	if createdParent {
		if _, err := runLocalCommand([]string{"mkdir", "-p", parentDir}, localShellCommandTimeout); err != nil {
			return "", err
		}
		lines = append(lines, "Executed: mkdir -p "+parentDir)
	}
	if _, err := runLocalCommand([]string{"touch", target}, localShellCommandTimeout); err != nil {
		return "", err
	}
	abs, _ := filepath.Abs(target)
	lines = append(lines, "Executed: touch "+target)
	if existedBefore {
		lines = append(lines, "File already exists: "+abs)
	} else {
		lines = append(lines, "Created file: "+abs)
	}
	if verifyOutput, err := runLocalCommand([]string{"ls", "-l", target}, localShellCommandTimeout); err == nil {
		lines = append(lines, "Executed: ls -l "+target)
		if strings.TrimSpace(verifyOutput) != "" {
			lines = append(lines, "Verification Output:")
			lines = append(lines, strings.TrimSpace(verifyOutput))
		}
	}
	result := strings.Join(lines, "\n")
	if changeSummary := buildRepoChangeSummary(before); strings.TrimSpace(changeSummary) != "" {
		result = strings.TrimSpace(result + "\n\n" + changeSummary)
	}
	return result, nil
}

func renameLocalFile(source, target string) (string, error) {
	source = cleanShellToken(source)
	target = cleanShellToken(target)
	if source == "" {
		return "", errors.New("source file name is required")
	}
	if target == "" {
		return "", errors.New("target file name is required")
	}
	if err := validateRelativePath(source); err != nil {
		return "", err
	}
	if err := validateRelativePath(target); err != nil {
		return "", err
	}
	if _, err := os.Stat(source); err != nil {
		return "", fmt.Errorf("source file not found: %s", source)
	}
	if _, err := os.Stat(target); err == nil {
		return "", fmt.Errorf("target already exists: %s", target)
	}
	before := captureRepoWorkingTreeSnapshot()
	if _, err := runLocalCommand([]string{"mv", source, target}, localShellCommandTimeout); err != nil {
		return "", err
	}
	abs, _ := filepath.Abs(target)
	result := fmt.Sprintf("Executed: mv %s %s\nRenamed file to: %s", source, target, abs)
	if changeSummary := buildRepoChangeSummary(before); strings.TrimSpace(changeSummary) != "" {
		result = strings.TrimSpace(result + "\n\n" + changeSummary)
	}
	return result, nil
}

func runLocalSafeCommand(command string) (string, error) {
	command = strings.TrimSpace(command)
	if command == "" {
		return "", errors.New("command is required")
	}
	if strings.ContainsAny(command, "|&;<>") {
		return "", errors.New("shell operators are not allowed in local chat command mode")
	}
	if shellUnsafePattern.MatchString(command) {
		return "", errors.New("command blocked by safety policy")
	}
	args := strings.Fields(command)
	if len(args) == 0 {
		return "", errors.New("command is required")
	}
	useSudo := false
	if strings.EqualFold(strings.TrimSpace(args[0]), "sudo") {
		useSudo = true
		args = args[1:]
		if len(args) == 0 {
			return "", errors.New("missing command after sudo")
		}
		if strings.HasPrefix(args[0], "-") {
			return "", errors.New("sudo flags are not allowed in local chat mode")
		}
	}
	name := strings.ToLower(strings.TrimSpace(args[0]))
	if _, ok := allowedLocalShellCommands[name]; !ok {
		return "", fmt.Errorf("command not allowed in local chat mode: %s", name)
	}
	before := captureRepoWorkingTreeSnapshot()

	var (
		output       string
		err          error
		executedText string
	)
	if useSudo {
		if err := ensureLocalPermission(permissionKeyShellSudo, "Allow running local shell commands with sudo when elevated access is required."); err != nil {
			return "", err
		}
		output, err = runLocalSudoCommand(args, localShellCommandTimeout)
		executedText = "sudo " + strings.Join(args, " ")
	} else {
		output, err = runLocalCommand(args, localShellCommandTimeout)
		executedText = strings.Join(args, " ")
		if err != nil && shouldRetryWithSudo(args, err) {
			reason := buildSudoRetryReason(args, err)
			if permErr := ensureLocalPermission(permissionKeyShellSudo, reason); permErr != nil {
				return "", fmt.Errorf("command failed without sudo (%s); sudo retry blocked: %w", sanitizeSudoReasonText(err.Error()), permErr)
			}
			output, err = runLocalSudoCommand(args, localShellCommandTimeout)
			executedText = "sudo " + strings.Join(args, " ")
		}
	}
	if err != nil {
		return "", err
	}
	lines := []string{"Executed: " + executedText}
	if strings.TrimSpace(output) != "" {
		lines = append(lines, "Output:")
		lines = append(lines, output)
	}
	if changeSummary := buildRepoChangeSummary(before); strings.TrimSpace(changeSummary) != "" {
		lines = append(lines, "")
		lines = append(lines, changeSummary)
	}
	return strings.Join(lines, "\n"), nil
}

func shouldRetryWithSudo(args []string, err error) bool {
	if len(args) == 0 || err == nil {
		return false
	}
	name := strings.ToLower(strings.TrimSpace(args[0]))
	if name == "" || name == "sudo" {
		return false
	}
	lower := strings.ToLower(strings.TrimSpace(err.Error()))
	if lower == "" {
		return false
	}
	if !containsAnyPhrase(lower, []string{
		"permission denied",
		"operation not permitted",
		"eacces",
		"requires root",
		"must be root",
		"access denied",
	}) {
		return false
	}
	if containsAnyPhrase(lower, []string{
		"not allowed in local chat mode",
		"command not found",
		"no such file or directory",
		"timed out",
	}) {
		return false
	}
	return true
}

func buildSudoRetryReason(args []string, runErr error) string {
	cmdText := strings.TrimSpace(strings.Join(args, " "))
	if cmdText == "" {
		cmdText = "the requested command"
	}
	errText := "permission-related failure"
	if runErr != nil {
		errText = sanitizeSudoReasonText(runErr.Error())
	}
	return fmt.Sprintf("Allow retrying `%s` with sudo because it failed with: %s", cmdText, errText)
}

func sanitizeSudoReasonText(text string) string {
	clean := strings.Join(strings.Fields(strings.TrimSpace(text)), " ")
	if clean == "" {
		return "permission-related failure"
	}
	const maxChars = 220
	if len(clean) > maxChars {
		return clean[:maxChars] + "...(truncated)"
	}
	return clean
}
