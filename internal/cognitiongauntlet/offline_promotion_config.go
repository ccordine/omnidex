package cognitiongauntlet

import (
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const OfflinePromotionConfigSchemaV1 = "omnidex.offline-cognition-promotion-config.v1"

type OfflinePromotionConfig struct {
	Schema                  string              `json:"schema"`
	DatabaseURL             string              `json:"database_url"`
	OllamaEndpoint          string              `json:"ollama_endpoint"`
	InferenceTimeoutSeconds int                 `json:"inference_timeout_seconds"`
	Scenario                OfflineScenarioSpec `json:"scenario"`
	Variant                 Variant             `json:"variant"`
	Surface                 Surface             `json:"surface"`
	RatGeneration           RatGeneration       `json:"rat_generation"`
	RuntimeFingerprint      RuntimeFingerprint  `json:"runtime_fingerprint"`
	Repetition              int                 `json:"repetition"`
	PublicOutputDirectory   string              `json:"public_output_directory"`
	PrivateOutputDirectory  string              `json:"private_output_directory"`
	OmnidexCommit           string              `json:"omnidex_commit,omitempty"`
	LedgerSchemaVersion     string              `json:"ledger_schema_version"`
	WorkingSetPolicyVersion string              `json:"working_set_policy_version"`
	ProjectionPolicyVersion string              `json:"projection_policy_version"`
}

type OfflinePromotionPaths struct {
	PublicBundle  string
	Episode       string
	PrivateOracle string
	Evaluation    string
	Receipt       string
}

func (config OfflinePromotionConfig) Validate() error {
	if config.Schema != OfflinePromotionConfigSchemaV1 || config.DatabaseURL == "" {
		return fmt.Errorf("offline cognition promotion configuration is invalid")
	}
	if err := config.Scenario.Validate(); err != nil {
		return err
	}
	if config.Variant != VariantFullCognition && !executableAblation(config.Variant) {
		return fmt.Errorf("offline cognition promotion variant %q is not executable", config.Variant)
	}
	if config.Variant == VariantRawShell && config.Surface != SurfaceFilesystem {
		return fmt.Errorf("offline raw-shell promotion requires the filesystem surface")
	}
	if _, err := config.Surface.Version(); err != nil {
		return err
	}
	if err := config.RatGeneration.Validate(); err != nil {
		return err
	}
	budget := config.Scenario.Budget()
	if err := budget.ValidateFor(config.RatGeneration); err != nil {
		return err
	}
	if _, err := productionBrain(
		config.RatGeneration, budget.Station.MaxOutputTokens,
	); err != nil {
		return err
	}
	if err := config.RuntimeFingerprint.Validate(); err != nil {
		return err
	}
	derivedRuntime, err := currentRuntimeFingerprint(config.RatGeneration.Runtime.SourceSHA256)
	if err != nil || config.RuntimeFingerprint != derivedRuntime ||
		config.RuntimeFingerprint.ProductionSourceSHA256 != config.RatGeneration.Runtime.SourceSHA256 {
		return fmt.Errorf("offline cognition promotion runtime authority is not code-derived")
	}
	if config.Repetition <= 0 || config.Repetition > 10_000 ||
		config.InferenceTimeoutSeconds <= 0 || config.InferenceTimeoutSeconds > int((24*time.Hour)/time.Second) {
		return fmt.Errorf("offline cognition promotion repetition or timeout is invalid")
	}
	parsed, err := url.Parse(config.OllamaEndpoint)
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" {
		return fmt.Errorf("offline cognition promotion Ollama endpoint is invalid")
	}
	database, err := url.Parse(config.DatabaseURL)
	if err != nil || (database.Scheme != "postgres" && database.Scheme != "postgresql") ||
		database.Host == "" {
		return fmt.Errorf("offline cognition promotion database URL is invalid")
	}
	if err := validateOfflineOutputDirectories(
		config.PublicOutputDirectory, config.PrivateOutputDirectory,
	); err != nil {
		return err
	}
	for label, value := range map[string]string{
		"Task Ledger schema version":        config.LedgerSchemaVersion,
		"Working Set policy version":        config.WorkingSetPolicyVersion,
		"Context Projection policy version": config.ProjectionPolicyVersion,
	} {
		if err := requireExact(value, label, 256); err != nil {
			return err
		}
	}
	if !validCommitIdentity(config.OmnidexCommit) {
		return fmt.Errorf("offline cognition promotion Omnidex commit is invalid")
	}
	return validateOfflineOutputTargets(config.Paths())
}

func validateOfflineOutputDirectories(publicDirectory, privateDirectory string) error {
	for _, directory := range []string{publicDirectory, privateDirectory} {
		info, err := os.Lstat(directory)
		resolved, resolveErr := filepath.EvalSymlinks(directory)
		if err != nil || resolveErr != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 ||
			!filepath.IsAbs(directory) || filepath.Clean(directory) != directory || resolved != directory {
			return fmt.Errorf("offline cognition promotion directory %q is unavailable or inexact", directory)
		}
	}
	publicInfo, _ := os.Stat(publicDirectory)
	privateInfo, _ := os.Stat(privateDirectory)
	if os.SameFile(publicInfo, privateInfo) {
		return fmt.Errorf("offline cognition promotion requires separate public and private output directories")
	}
	if pathContains(publicDirectory, privateDirectory) ||
		pathContains(privateDirectory, publicDirectory) {
		return fmt.Errorf("offline cognition output directories cannot contain each other")
	}
	if err := validatePrivateOutputDirectory(privateDirectory, privateInfo); err != nil {
		return err
	}
	return nil
}

func (config OfflinePromotionConfig) Paths() OfflinePromotionPaths {
	return OfflinePromotionPaths{
		PublicBundle:  filepath.Join(config.PublicOutputDirectory, "inference-bootstrap.json"),
		Episode:       filepath.Join(config.PublicOutputDirectory, "sealed-episode.json"),
		PrivateOracle: filepath.Join(config.PrivateOutputDirectory, "private-oracle.json"),
		Evaluation:    filepath.Join(config.PrivateOutputDirectory, "evaluation.json"),
		Receipt:       filepath.Join(config.PrivateOutputDirectory, "promotion-receipt.json"),
	}
}

func validateOfflineOutputTargets(paths OfflinePromotionPaths) error {
	for _, path := range []string{
		paths.PublicBundle, paths.Episode, paths.PrivateOracle, paths.Evaluation, paths.Receipt,
	} {
		if _, err := os.Lstat(path); !os.IsNotExist(err) {
			return fmt.Errorf("offline cognition promotion output %q already exists or is inaccessible", path)
		}
	}
	return nil
}

func pathContains(parent, candidate string) bool {
	relative, err := filepath.Rel(parent, candidate)
	if err != nil || relative == "." {
		return relative == "."
	}
	return relative != ".." && !filepath.IsAbs(relative) &&
		!strings.HasPrefix(relative, ".."+string(filepath.Separator))
}
