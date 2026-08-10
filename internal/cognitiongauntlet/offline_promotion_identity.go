package cognitiongauntlet

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"

	"github.com/gryph/omnidex/internal/cognition"
	"github.com/gryph/omnidex/internal/cognitionpolicy"
	"github.com/gryph/omnidex/internal/cognitionstate"
	"github.com/gryph/omnidex/internal/contextbuilder"
	"github.com/gryph/omnidex/internal/llm"
	"github.com/gryph/omnidex/internal/workingset"
)

func validateOfflinePromotionIdentity(
	config OfflinePromotionConfig,
	executable string,
	embeddedCommit string,
	embeddedSourceSHA256 string,
	embeddedMigrationsSHA256 string,
	embeddedVersion string,
) (string, error) {
	return validateOfflineExecutionIdentity(
		config.executionAuthority(), executable, embeddedCommit, embeddedSourceSHA256,
		embeddedMigrationsSHA256, embeddedVersion,
	)
}

func validateOfflineExecutionIdentity(
	authority offlineExecutionAuthority,
	executable string,
	embeddedCommit string,
	embeddedSourceSHA256 string,
	embeddedMigrationsSHA256 string,
	embeddedVersion string,
) (string, error) {
	if err := authority.Validate(); err != nil {
		return "", err
	}
	if !validCommitIdentity(embeddedCommit) || authority.OmnidexCommit != embeddedCommit {
		return "", fmt.Errorf("offline promotion commit does not match the embedded build commit")
	}
	if !validDigest(embeddedSourceSHA256) ||
		authority.RatGeneration.Runtime.SourceSHA256 != embeddedSourceSHA256 {
		return "", fmt.Errorf("offline promotion source digest does not match the embedded build source")
	}
	if !validDigest(embeddedMigrationsSHA256) ||
		authority.RatGeneration.Runtime.MigrationsSHA256 != embeddedMigrationsSHA256 {
		return "", fmt.Errorf("offline promotion migration digest does not match the embedded build")
	}
	if err := requireExact(embeddedVersion, "embedded runtime version", 256); err != nil ||
		authority.RatGeneration.Runtime.Version != embeddedVersion {
		return "", fmt.Errorf("offline promotion runtime version does not match the embedded build")
	}
	executableSHA, err := executableSHA256(executable)
	if err != nil {
		return "", err
	}
	if authority.RatGeneration.Runtime.ExecutableSHA256 != executableSHA {
		return "", fmt.Errorf("offline promotion executable does not match the frozen runtime candidate")
	}
	expected, err := currentRuntimeFingerprint(embeddedSourceSHA256)
	if err != nil {
		return "", err
	}
	if authority.RuntimeFingerprint != expected {
		return "", fmt.Errorf("offline promotion runtime fingerprint is not code-derived")
	}
	return executableSHA, nil
}

func validateCurrentProcessIdentity(
	expectedExecutableSHA256 string,
	expectedCommit string,
	expectedSourceSHA256 string,
	embeddedCommit string,
	embeddedSourceSHA256 string,
) error {
	if !validDigest(expectedExecutableSHA256) || !validCommitIdentity(expectedCommit) ||
		!validDigest(expectedSourceSHA256) || expectedCommit != embeddedCommit ||
		expectedSourceSHA256 != embeddedSourceSHA256 {
		return fmt.Errorf("offline cognition child build identity is invalid")
	}
	executable, err := os.Executable()
	if err != nil {
		return fmt.Errorf("resolve offline cognition child executable: %w", err)
	}
	actual, err := executableSHA256(executable)
	if err != nil {
		return err
	}
	if actual != expectedExecutableSHA256 {
		return fmt.Errorf("offline cognition child executable digest changed")
	}
	return nil
}

func currentRuntimeFingerprint(sourceSHA256 string) (RuntimeFingerprint, error) {
	if !validDigest(sourceSHA256) {
		return RuntimeFingerprint{}, fmt.Errorf("runtime source digest is invalid")
	}
	renderer, err := digestJSON([]string{
		contextbuilder.ProjectionSchemaV1, contextbuilder.RendererJSONV1,
		contextbuilder.TokenEstimatorV1, cognitionpolicy.EnvelopeSchemaV2,
		cognitionpolicy.RendererVersionV2,
	})
	if err != nil {
		return RuntimeFingerprint{}, err
	}
	retention, err := digestJSON([]string{
		workingset.WorkingSetSchemaV1, workingset.WorkingSetCommandSchemaV1,
		cognitionstate.EntryMappingSchemaV1, cognitionstate.AttentionPlanSchemaV1,
		cognitionstate.ObservationRetentionSchemaV1,
	})
	if err != nil {
		return RuntimeFingerprint{}, err
	}
	obligations, err := digestJSON([]string{
		cognition.ObligationGraphSchemaV1, cognition.ObligationIdentitySchemaV1,
		cognition.ObligationMaterializationSchemaV1, cognition.CompletionAuthoritySchemaV1,
	})
	if err != nil {
		return RuntimeFingerprint{}, err
	}
	prompt, err := digestJSON([]string{
		llm.MinimalGeneratePrompt, cognitionpolicy.DecisionSchemaProtocolV1,
		cognitionpolicy.EnvelopeSchemaV2, cognitionpolicy.RendererVersionV2,
	})
	if err != nil {
		return RuntimeFingerprint{}, err
	}
	return RuntimeFingerprint{
		ProductionSourceSHA256: sourceSHA256, RendererSHA256: renderer,
		RetentionPolicySHA256: retention, ObligationPolicySHA256: obligations,
		PromptSHA256: prompt,
	}, nil
}

func executableSHA256(path string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", fmt.Errorf("open offline promotion executable: %w", err)
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil || !info.Mode().IsRegular() || info.Size() <= 0 {
		return "", fmt.Errorf("offline promotion executable is not a nonempty regular file")
	}
	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return "", fmt.Errorf("hash offline promotion executable: %w", err)
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
}
