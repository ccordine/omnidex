package cognitiongauntlet

import (
	"fmt"
	"time"

	"github.com/gryph/omnidex/internal/cognition"
)

// matrixReplayPreregistration is intentionally opaque and non-serializable.
// Its only issuer reloads the immutable Matrix schedule whose RegisteredAt
// precedes inference; replay bytes cannot construct this credential.
type matrixReplayPreregistration struct {
	preregistrationSHA256 string
	registeredAt          time.Time
	scheduleOrdinal       int
	coordinate            OfflineMatrixCase
	variant               Variant
	authority             PublicRunAuthority
	episode               cognition.EpisodeRef
	execution             matrixReplayExecutionIdentity
	fingerprint           string
}

// matrixReplayExecutionIdentity is the portion of the immutable Matrix fixed
// authority that is recorded directly in every sealed episode. These values
// are deliberately not part of PublicRunAuthority, so the preregistration
// credential must bind them separately.
type matrixReplayExecutionIdentity struct {
	omnidexCommit           string
	ledgerSchemaVersion     string
	workingSetPolicyVersion string
	projectionPolicyVersion string
}

// loadMatrixReplayPreregistration derives one opaque replay credential from
// the immutable Matrix schedule. It is deliberately package-private: the
// Matrix runner must issue every credential before starting inference, and no
// post-run caller may mint serious qualification from replay-derived values.
func loadMatrixReplayPreregistration(
	config OfflineMatrixConfig,
	caseID string,
	variant Variant,
) (matrixReplayPreregistration, error) {
	if err := config.Validate(); err != nil {
		return matrixReplayPreregistration{}, err
	}
	registration, err := LoadOfflineMatrixPreregistration(config.Paths().Preregistration)
	if err != nil {
		return matrixReplayPreregistration{}, err
	}
	registrationSHA, err := registration.SHA256()
	if err != nil || registrationSHA != config.PreregistrationSHA256 ||
		!registration.Matches(config.Plan, config.fixedAuthority()) {
		return matrixReplayPreregistration{}, fmt.Errorf("offline Matrix replay preregistration changed")
	}
	var coordinate OfflineMatrixCase
	caseCount := 0
	for _, current := range registration.Cases {
		if current.ID == caseID {
			coordinate, caseCount = current, caseCount+1
		}
	}
	scheduleOrdinal := 0
	for index, current := range matrixCoordinates(registration) {
		if current.Case.ID == caseID && current.Variant == variant {
			scheduleOrdinal = index + 1
		}
	}
	if caseCount != 1 || scheduleOrdinal == 0 {
		return matrixReplayPreregistration{}, fmt.Errorf("replay coordinate is not preregistered exactly once")
	}
	run, err := config.derivedRunConfig(coordinate, variant)
	if err != nil {
		return matrixReplayPreregistration{}, err
	}
	generated, err := generateOfflineScenario(run.Scenario)
	if err != nil {
		return matrixReplayPreregistration{}, err
	}
	paired, err := generated.pairedAuthority(
		run.Surface, run.RatGeneration, run.Repetition, run.RuntimeFingerprint,
	)
	if err != nil {
		return matrixReplayPreregistration{}, err
	}
	authority, err := NewPublicRunAuthority(paired, variant)
	if err != nil {
		return matrixReplayPreregistration{}, err
	}
	episode, err := PublicVariantEpisodeRef(authority)
	if err != nil {
		return matrixReplayPreregistration{}, err
	}
	credential := matrixReplayPreregistration{
		preregistrationSHA256: registrationSHA,
		registeredAt:          registration.RegisteredAt,
		scheduleOrdinal:       scheduleOrdinal,
		coordinate:            coordinate,
		variant:               variant,
		authority:             authority,
		episode:               episode,
		execution: matrixReplayExecutionIdentity{
			omnidexCommit:           registration.Fixed.OmnidexCommit,
			ledgerSchemaVersion:     registration.Fixed.LedgerSchemaVersion,
			workingSetPolicyVersion: registration.Fixed.WorkingSetPolicyVersion,
			projectionPolicyVersion: registration.Fixed.ProjectionPolicyVersion,
		},
	}
	credential.fingerprint, err = credential.deriveFingerprint()
	if err != nil {
		return matrixReplayPreregistration{}, err
	}
	return credential, credential.validate()
}

func (credential matrixReplayPreregistration) validate() error {
	episode, episodeErr := PublicVariantEpisodeRef(credential.authority)
	fingerprint, fingerprintErr := credential.deriveFingerprint()
	if !validDigest(credential.preregistrationSHA256) || credential.registeredAt.IsZero() ||
		credential.registeredAt.After(time.Now().UTC().Add(time.Minute)) ||
		credential.scheduleOrdinal <= 0 || credential.coordinate.ID == "" ||
		credential.variant != credential.authority.Variant ||
		credential.authority.Validate() != nil || episodeErr != nil || episode != credential.episode ||
		credential.execution.validate() != nil ||
		fingerprintErr != nil || credential.fingerprint != fingerprint {
		return fmt.Errorf("Matrix replay preregistration credential is invalid")
	}
	return nil
}

func (identity matrixReplayExecutionIdentity) validate() error {
	if !validCommitIdentity(identity.omnidexCommit) {
		return fmt.Errorf("Matrix replay Omnidex commit is invalid")
	}
	for label, value := range map[string]string{
		"Matrix replay ledger schema version":      identity.ledgerSchemaVersion,
		"Matrix replay Working Set policy version": identity.workingSetPolicyVersion,
		"Matrix replay projection policy version":  identity.projectionPolicyVersion,
	} {
		if err := requireExact(value, label, 256); err != nil {
			return err
		}
	}
	return nil
}

func (credential matrixReplayPreregistration) binds(
	authority PublicRunAuthority,
	episodeID cognition.EpisodeID,
	startedAt time.Time,
) error {
	if credential.validate() != nil || authority != credential.authority ||
		episodeID != credential.episode.ID || startedAt.Before(credential.registeredAt) {
		return fmt.Errorf("replay differs from its Matrix preregistration")
	}
	return nil
}

func (credential matrixReplayPreregistration) bindsExecution(manifest EpisodeManifest) error {
	if credential.validate() != nil ||
		manifest.OmnidexCommit != credential.execution.omnidexCommit ||
		manifest.LedgerSchemaVersion != credential.execution.ledgerSchemaVersion ||
		manifest.WorkingSetPolicyVersion != credential.execution.workingSetPolicyVersion ||
		manifest.ProjectionPolicyVersion != credential.execution.projectionPolicyVersion {
		return fmt.Errorf("replay execution identity differs from its Matrix preregistration")
	}
	return nil
}

func (credential matrixReplayPreregistration) deriveFingerprint() (string, error) {
	return digestJSON(struct {
		PreregistrationSHA256 string
		RegisteredAt          time.Time
		ScheduleOrdinal       int
		Coordinate            OfflineMatrixCase
		Variant               Variant
		Authority             PublicRunAuthority
		Episode               cognition.EpisodeRef
		Execution             matrixReplayExecutionIdentity
	}{
		credential.preregistrationSHA256,
		credential.registeredAt,
		credential.scheduleOrdinal,
		credential.coordinate,
		credential.variant,
		credential.authority,
		credential.episode,
		credential.execution,
	})
}
