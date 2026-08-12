package cognitiongauntlet

import "fmt"

// offlineExecutionAuthority is the single process/runtime authority shared by
// ordinary, Scale, Transfer, and Resume executions. Workload generation and
// private evaluation remain distinct typed rails.
type offlineExecutionAuthority struct {
	DatabaseURL             string
	OllamaEndpoint          string
	InferenceTimeoutSeconds int
	Variant                 Variant
	RatGeneration           RatGeneration
	PreparedBrainEvidence   PreparedBrainEvidenceAuthority
	RuntimeFingerprint      RuntimeFingerprint
	Budget                  RunBudget
	Repetition              int
	OmnidexCommit           string
	LedgerSchemaVersion     string
	WorkingSetPolicyVersion string
	ProjectionPolicyVersion string
}

func (authority offlineExecutionAuthority) Validate() error {
	if authority.Variant != VariantFullCognition && !executableAblation(authority.Variant) {
		return fmt.Errorf("offline execution variant is not registered")
	}
	if err := authority.Budget.ValidateFor(authority.RatGeneration); err != nil {
		return err
	}
	if err := authority.RuntimeFingerprint.Validate(); err != nil {
		return err
	}
	if _, err := loadPreparedBrainEvidenceAuthority(
		authority.PreparedBrainEvidence, authority.RatGeneration.Fixed.Brain,
	); err != nil {
		return fmt.Errorf("verify offline execution prepared Brain evidence: %w", err)
	}
	derived, err := currentRuntimeFingerprint(authority.RatGeneration.Runtime.SourceSHA256)
	if err != nil || derived != authority.RuntimeFingerprint ||
		authority.Repetition <= 0 || authority.Repetition > 10_000 ||
		authority.InferenceTimeoutSeconds <= 0 || authority.InferenceTimeoutSeconds > 24*60*60 ||
		!validCommitIdentity(authority.OmnidexCommit) {
		return fmt.Errorf("offline execution runtime authority is invalid")
	}
	if err := validateOfflineMatrixEndpoints(authority.DatabaseURL, authority.OllamaEndpoint); err != nil {
		return err
	}
	for label, value := range map[string]string{
		"Task Ledger schema version":        authority.LedgerSchemaVersion,
		"Working Set policy version":        authority.WorkingSetPolicyVersion,
		"Context Projection policy version": authority.ProjectionPolicyVersion,
	} {
		if err := requireExact(value, label, 256); err != nil {
			return err
		}
	}
	return nil
}

func (config OfflinePromotionConfig) executionAuthority() offlineExecutionAuthority {
	return offlineExecutionAuthority{
		DatabaseURL: config.DatabaseURL, OllamaEndpoint: config.OllamaEndpoint,
		InferenceTimeoutSeconds: config.InferenceTimeoutSeconds,
		Variant:                 config.Variant, RatGeneration: config.RatGeneration,
		PreparedBrainEvidence: config.PreparedBrainEvidence,
		RuntimeFingerprint: config.RuntimeFingerprint, Budget: config.Scenario.Budget(),
		Repetition: config.Repetition, OmnidexCommit: config.OmnidexCommit,
		LedgerSchemaVersion:     config.LedgerSchemaVersion,
		WorkingSetPolicyVersion: config.WorkingSetPolicyVersion,
		ProjectionPolicyVersion: config.ProjectionPolicyVersion,
	}
}
