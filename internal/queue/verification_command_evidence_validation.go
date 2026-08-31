package queue

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
	"unicode/utf8"
)

const (
	maxVerificationCommandArguments   = 128
	maxVerificationCommandArrayBytes  = 64 * 1024
	maxVerificationCommandStreamBytes = 1024 * 1024
)

var verificationEnvironmentName = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

func normalizeVerificationCommandEvidence(
	record VerificationCommandEvidence,
) (VerificationCommandEvidence, error) {
	if err := validateStepAttemptAuthority(record.Authority); err != nil {
		return VerificationCommandEvidence{}, err
	}
	if record.ID != 0 || record.Status != "" || record.DurationNanos != 0 ||
		record.ArgvSHA256 != "" || record.EnvironmentSHA256 != "" ||
		record.StdinSHA256 != "" || record.StdoutSHA256 != "" ||
		record.StderrSHA256 != "" || !record.CreatedAt.IsZero() {
		return VerificationCommandEvidence{}, fmt.Errorf("new verification command evidence contains database-derived fields")
	}
	if !registeredVerificationCommandPhase(record.Phase) || record.Ordinal < 1 {
		return VerificationCommandEvidence{}, fmt.Errorf("verification command evidence requires a registered phase and positive ordinal")
	}
	if err := validateVerificationCommandArgv(record.Argv); err != nil {
		return VerificationCommandEvidence{}, err
	}
	if err := validateVerificationEnvironment(record.Environment); err != nil {
		return VerificationCommandEvidence{}, err
	}
	if !filepath.IsAbs(record.WorkingDirectory) || filepath.Clean(record.WorkingDirectory) != record.WorkingDirectory ||
		len(record.WorkingDirectory) > 4096 || !validVerificationText(record.WorkingDirectory) {
		return VerificationCommandEvidence{}, fmt.Errorf("verification working directory must be one canonical absolute path")
	}
	if record.StartedAt.Location() != time.UTC || record.FinishedAt.Location() != time.UTC ||
		record.StartedAt.IsZero() ||
		record.FinishedAt.Before(record.StartedAt) ||
		record.StartedAt.Nanosecond()%1000 != 0 || record.FinishedAt.Nanosecond()%1000 != 0 {
		return VerificationCommandEvidence{}, fmt.Errorf("verification timestamps must be canonical PostgreSQL-precision UTC")
	}
	if len(record.Stdin) > maxVerificationCommandStreamBytes ||
		len(record.Stdout) > maxVerificationCommandStreamBytes ||
		len(record.Stderr) > maxVerificationCommandStreamBytes {
		return VerificationCommandEvidence{}, fmt.Errorf("verification command stream exceeds the 1 MiB evidence bound")
	}
	if (!record.StdoutComplete || !record.StderrComplete) && record.LaunchError == "" {
		return VerificationCommandEvidence{}, fmt.Errorf("incomplete verification output requires one terminal launch error")
	}
	if err := validateLLMCallError(record.LaunchError); err != nil {
		return VerificationCommandEvidence{}, fmt.Errorf("verification launch error: %w", err)
	}
	if err := validateLLMCallError(record.ObservationError); err != nil {
		return VerificationCommandEvidence{}, fmt.Errorf("verification observation error: %w", err)
	}
	if (record.ExitCode == nil) != (record.LaunchError != "") {
		return VerificationCommandEvidence{}, fmt.Errorf("verification command requires either an exit code or one launch error")
	}
	if record.ExitCode != nil && (*record.ExitCode < 0 || *record.ExitCode > 255) {
		return VerificationCommandEvidence{}, fmt.Errorf("verification command exit code is outside the portable process range")
	}
	if !optionalVerificationWorkspaceSHA(record.WorkspaceSHA256Before) ||
		!optionalVerificationWorkspaceSHA(record.WorkspaceSHA256After) ||
		(record.WorkspaceSHA256After != "" && record.WorkspaceSHA256Before == "") {
		return VerificationCommandEvidence{}, fmt.Errorf("verification workspace identities must be one optional exact before/after pair")
	}
	if verificationHostPhase(record.Phase) && record.WorkspaceSHA256Before == "" {
		return VerificationCommandEvidence{}, fmt.Errorf("host verification command requires authoritative before/after workspace identities")
	}
	if record.ObservationError != "" {
		if record.WorkspaceSHA256Before == "" || record.WorkspaceSHA256After != "" {
			return VerificationCommandEvidence{}, fmt.Errorf("verification observation failure requires one before identity and no unobserved after identity")
		}
	} else if (record.WorkspaceSHA256Before == "") != (record.WorkspaceSHA256After == "") {
		return VerificationCommandEvidence{}, fmt.Errorf("verification workspace identities must be one exact pair")
	}

	if record.Environment == nil {
		record.Environment = []string{}
	}
	argvJSON, _ := json.Marshal(record.Argv)
	environmentJSON, _ := json.Marshal(record.Environment)
	record.Argv = append([]string(nil), record.Argv...)
	record.Environment = append([]string{}, record.Environment...)
	record.StdinPresent = record.Stdin != nil
	if record.StdinPresent {
		exactStdin := make([]byte, len(record.Stdin))
		copy(exactStdin, record.Stdin)
		record.Stdin = exactStdin
	}
	exactStdout := make([]byte, len(record.Stdout))
	copy(exactStdout, record.Stdout)
	record.Stdout = exactStdout
	exactStderr := make([]byte, len(record.Stderr))
	copy(exactStderr, record.Stderr)
	record.Stderr = exactStderr
	record.DurationNanos = record.FinishedAt.Sub(record.StartedAt).Nanoseconds()
	record.ArgvSHA256 = llmEvidenceSHA256(argvJSON)
	record.EnvironmentSHA256 = llmEvidenceSHA256(environmentJSON)
	if record.StdinPresent {
		record.StdinSHA256 = llmEvidenceSHA256(record.Stdin)
	}
	record.StdoutSHA256 = llmEvidenceSHA256(record.Stdout)
	record.StderrSHA256 = llmEvidenceSHA256(record.Stderr)
	switch {
	case record.ObservationError != "":
		record.Status = VerificationCommandObservationFailed
	case record.WorkspaceSHA256Before != "" &&
		record.WorkspaceSHA256Before != record.WorkspaceSHA256After:
		record.Status = VerificationCommandWorkspaceChanged
	case record.LaunchError != "":
		record.Status = VerificationCommandLaunchFailed
	case *record.ExitCode == 0:
		record.Status = VerificationCommandSucceeded
	default:
		record.Status = VerificationCommandExitFailed
	}
	return record, nil
}

func validateVerificationCommandArgv(argv []string) error {
	if len(argv) < 1 || len(argv) > maxVerificationCommandArguments || argv[0] == "" {
		return fmt.Errorf("verification command requires 1..%d argv values", maxVerificationCommandArguments)
	}
	raw, err := json.Marshal(argv)
	if err != nil || len(raw) > maxVerificationCommandArrayBytes {
		return fmt.Errorf("verification argv exceeds its exact evidence bound")
	}
	for _, value := range argv {
		if !validVerificationText(value) {
			return fmt.Errorf("verification argv contains invalid text")
		}
	}
	return nil
}

func validateVerificationEnvironment(environment []string) error {
	if len(environment) > 64 || !sort.StringsAreSorted(environment) {
		return fmt.Errorf("verification environment overrides must be bounded and sorted")
	}
	raw, err := json.Marshal(environment)
	if err != nil || len(raw) > maxVerificationCommandArrayBytes {
		return fmt.Errorf("verification environment exceeds its exact evidence bound")
	}
	prior := ""
	for _, entry := range environment {
		name, _, found := strings.Cut(entry, "=")
		if !found || !verificationEnvironmentName.MatchString(name) ||
			!validVerificationText(entry) || name == prior {
			return fmt.Errorf("verification environment contains an invalid or duplicate override")
		}
		prior = name
	}
	return nil
}

func registeredVerificationCommandPhase(phase VerificationCommandPhase) bool {
	switch phase {
	case VerificationIsolatedInstall, VerificationIsolatedImplementation,
		VerificationIsolatedTask, VerificationIsolatedFinal,
		VerificationHostInstall, VerificationHostFinal, VerificationHostCleanup:
		return true
	default:
		return false
	}
}

func verificationHostPhase(phase VerificationCommandPhase) bool {
	return phase == VerificationHostInstall || phase == VerificationHostFinal ||
		phase == VerificationHostCleanup
}

func optionalVerificationWorkspaceSHA(value string) bool {
	return value == "" || exactLowerSHA256(value)
}

func validVerificationText(value string) bool {
	return utf8.ValidString(value) && !strings.ContainsRune(value, 0)
}
