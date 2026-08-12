package cognitiongauntlet

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)

const maxOfflineChildOutputBytes = 64 * 1024

func newGeneratorProcessConfig(
	config OfflinePromotionConfig,
	paths OfflinePromotionPaths,
	hostScenarioPath string,
	privateOracleCredential string,
	executableSHA256 string,
) generatorProcessConfig {
	return generatorProcessConfig{
		Schema: generatorProcessConfigSchemaV1, Scenario: config.Scenario, Variant: config.Variant,
		Surface:       config.Surface,
		RatGeneration: config.RatGeneration, RuntimeFingerprint: config.RuntimeFingerprint,
		Repetition: config.Repetition, PublicBundlePath: paths.PublicBundle,
		HostScenarioPath: hostScenarioPath, PrivateOraclePath: paths.PrivateOracle,
		PrivateOracleCredential: privateOracleCredential,
		ExecutableSHA256:        executableSHA256,
		SourceSHA256:            config.RatGeneration.Runtime.SourceSHA256,
		OmnidexCommit:           config.OmnidexCommit,
	}
}

func newInferenceProcessConfig(
	config OfflinePromotionConfig,
	database *offlinePromotionDatabase,
	host *offlinePromotionHost,
	paths OfflinePromotionPaths,
	executableSHA256 string,
	privateOracleCredential string,
) inferenceProcessConfig {
	return newInferenceProcessConfigForExecution(
		config.executionAuthority(), database, host, paths,
		executableSHA256, privateOracleCredential,
	)
}

func newInferenceProcessConfigForExecution(
	authority offlineExecutionAuthority,
	database *offlinePromotionDatabase,
	host *offlinePromotionHost,
	paths OfflinePromotionPaths,
	executableSHA256 string,
	privateOracleCredential string,
) inferenceProcessConfig {
	process := inferenceProcessConfig{
		Schema:      inferenceProcessConfigSchemaV3,
		DatabaseURL: database.inferenceURL, DatabaseSchema: database.schema,
		EnvironmentURL: host.baseURL, EnvironmentToken: host.token,
		OllamaEndpoint: authority.OllamaEndpoint, TimeoutSeconds: authority.InferenceTimeoutSeconds,
		PublicBundlePath:       paths.PublicBundle,
		PublicOutputDirectory:  filepath.Dir(paths.PublicBundle),
		PrivateOutputDirectory: filepath.Dir(paths.PrivateOracle),
		EpisodePath:            paths.Episode,
		EvidencePath:           paths.Evidence,
		Attempt:                database.attempt, ExecutableSHA256: executableSHA256,
		SourceSHA256:            authority.RatGeneration.Runtime.SourceSHA256,
		OmnidexCommit:           authority.OmnidexCommit,
		LedgerSchemaVersion:     authority.LedgerSchemaVersion,
		WorkingSetPolicyVersion: authority.WorkingSetPolicyVersion,
		ProjectionPolicyVersion: authority.ProjectionPolicyVersion,
		Control:                 terminalInferenceControl(),
	}
	if authority.Variant == VariantOracleEvidence {
		process.ContaminatedOraclePath = paths.PrivateOracle
		process.ContaminatedOracleGrant = privateOracleCredential
	}
	return process
}

func newEvaluatorProcessConfig(
	config OfflinePromotionConfig,
	paths OfflinePromotionPaths,
	privateOracleCredential string,
	executableSHA256 string,
) evaluatorProcessConfig {
	return newEvaluatorProcessConfigForExecution(
		config.executionAuthority(), paths, privateOracleCredential, executableSHA256,
	)
}

func newEvaluatorProcessConfigForExecution(
	authority offlineExecutionAuthority,
	paths OfflinePromotionPaths,
	privateOracleCredential string,
	executableSHA256 string,
) evaluatorProcessConfig {
	return evaluatorProcessConfig{
		Schema: evaluatorProcessConfigSchemaV1, PrivateOraclePath: paths.PrivateOracle,
		PrivateOracleCredential: privateOracleCredential,
		PublicBundlePath:        paths.PublicBundle,
		EpisodePath:             paths.Episode, EvaluationPath: paths.Evaluation,
		ExecutableSHA256: executableSHA256,
		SourceSHA256:     authority.RatGeneration.Runtime.SourceSHA256,
		OmnidexCommit:    authority.OmnidexCommit,
	}
}

func runOfflineChild(
	ctx context.Context,
	executable string,
	phase string,
	configPath string,
	expectedExecutableSHA256 string,
) (int, error) {
	child, err := startOfflineChild(
		ctx, executable, phase, configPath, expectedExecutableSHA256,
	)
	if err != nil {
		return 0, err
	}
	return child.wait()
}

type offlineChildProcess struct {
	command *exec.Cmd
	phase   string
	output  *boundedProcessOutput
}

func startOfflineChild(
	ctx context.Context,
	executable string,
	phase string,
	configPath string,
	expectedExecutableSHA256 string,
) (*offlineChildProcess, error) {
	if !registeredOfflineChildPhase(phase) {
		return nil, fmt.Errorf("offline cognition child phase %q is not registered", phase)
	}
	actualSHA256, err := executableSHA256(executable)
	if err != nil {
		return nil, err
	}
	if actualSHA256 != expectedExecutableSHA256 {
		return nil, fmt.Errorf("offline cognition child executable changed before %s", phase)
	}
	command, err := offlineChildCommand(ctx, executable, phase, configPath)
	if err != nil {
		return nil, err
	}
	output := &boundedProcessOutput{}
	command.Stdout, command.Stderr = output, output
	if err := command.Start(); err != nil {
		return nil, err
	}
	return &offlineChildProcess{command: command, phase: phase, output: output}, nil
}

func (child *offlineChildProcess) pid() int {
	if child == nil || child.command == nil || child.command.Process == nil {
		return 0
	}
	return child.command.Process.Pid
}

func (child *offlineChildProcess) signal(signal os.Signal) error {
	if child == nil || child.command == nil || child.command.Process == nil || signal == nil {
		return fmt.Errorf("offline cognition child signal authority is invalid")
	}
	return child.command.Process.Signal(signal)
}

func (child *offlineChildProcess) wait() (int, error) {
	pid := child.pid()
	if pid == 0 {
		return 0, fmt.Errorf("offline cognition child process was not started")
	}
	if err := child.command.Wait(); err != nil {
		return pid, fmt.Errorf(
			"offline cognition %s process failed: %w: %s", child.phase, err, child.output.String(),
		)
	}
	return pid, nil
}

func offlineChildCommand(
	ctx context.Context,
	executable string,
	phase string,
	configPath string,
) (*exec.Cmd, error) {
	if ctx == nil || !registeredOfflineChildPhase(phase) ||
		executable == "" || configPath == "" {
		return nil, fmt.Errorf("offline cognition child command authority is invalid")
	}
	command := exec.CommandContext(ctx, executable, phase, "--process-config", configPath)
	command.Env = []string{"LANG=C.UTF-8"}
	command.Stdin = nil
	return command, nil
}

func registeredOfflineChildPhase(phase string) bool {
	switch phase {
	case "generate", "generate-scale", "host", "infer", "evaluate", "evaluate-scale":
		return true
	default:
		return false
	}
}

type boundedProcessOutput struct {
	buffer bytes.Buffer
	total  int
}

func (output *boundedProcessOutput) Write(value []byte) (int, error) {
	original := len(value)
	output.total += original
	remaining := maxOfflineChildOutputBytes - output.buffer.Len()
	if remaining > 0 {
		if len(value) > remaining {
			value = value[:remaining]
		}
		_, _ = output.buffer.Write(value)
	}
	return original, nil
}

func (output *boundedProcessOutput) String() string {
	if output.total <= maxOfflineChildOutputBytes {
		return output.buffer.String()
	}
	return output.buffer.String() + " [child output truncated]"
}
