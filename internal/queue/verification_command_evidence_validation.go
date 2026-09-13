package queue

import (
	"encoding/json"
	"fmt"
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
		!record.CreatedAt.IsZero() {
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
	if err := validateVerificationContainerEvidence(record); err != nil {
		return VerificationCommandEvidence{}, err
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
	if record.Environment == nil {
		record.Environment = []string{}
	}
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
	switch {
	case record.ObservationError != "":
		record.Status = VerificationCommandObservationFailed
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
		VerificationHostInstall, VerificationHostFinal:
		return true
	default:
		return false
	}
}

func validVerificationText(value string) bool {
	return utf8.ValidString(value) && !strings.ContainsRune(value, 0)
}
