package cognitiongauntlet

import (
	"fmt"
	"path/filepath"
	"time"

	"github.com/gryph/omnidex/internal/model"
)

type offlineLiveStaleInference struct {
	port                liveStalePort
	promotion           offlinePromotionInference
	original            model.StepAttemptAuthority
	replacement         model.StepAttemptAuthority
	originalPID         int
	replacementPID      int
	checkpoint          liveStalePortCheckpoint
	rejection           liveStalePortRejection
	stateBefore         LiveStaleDurableState
	stateAfter          LiveStaleDurableState
	replacementSealedAt time.Time
	originalResumedAt   time.Time
	originalStoppedAt   time.Time
	originalExitedAt    time.Time
	continuity          TakeoverContinuityProof
	hostSchema          string
}

func liveStaleProbeConfig(
	base OfflinePromotionConfig,
	port liveStalePort,
) (OfflinePromotionConfig, error) {
	if err := port.Validate(); err != nil {
		return OfflinePromotionConfig{}, err
	}
	base = liveStaleProbeArtifactConfig(base, port)
	if port == liveStalePolicyFinish {
		return base, base.Validate()
	}
	if err := createMatrixRunDirectories(
		base.PublicOutputDirectory, base.PrivateOutputDirectory,
	); err != nil {
		return OfflinePromotionConfig{}, err
	}
	if err := base.Validate(); err != nil {
		return OfflinePromotionConfig{}, fmt.Errorf("live stale-port config: %w", err)
	}
	return base, nil
}

func liveStaleProbeArtifactConfig(
	base OfflinePromotionConfig,
	port liveStalePort,
) OfflinePromotionConfig {
	if port == liveStalePolicyFinish {
		return base
	}
	base.PublicOutputDirectory = filepath.Join(
		base.PublicOutputDirectory, "stale-port-probes", string(port),
	)
	base.PrivateOutputDirectory = filepath.Join(
		base.PrivateOutputDirectory, "stale-port-probes", string(port),
	)
	return base
}
