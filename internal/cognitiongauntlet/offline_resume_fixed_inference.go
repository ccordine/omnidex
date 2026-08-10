package cognitiongauntlet

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/gryph/omnidex/internal/model"
	"github.com/gryph/omnidex/internal/queue"
	buildversion "github.com/gryph/omnidex/internal/version"
)

type offlineResumeInference struct {
	promotion              offlinePromotionInference
	schedule               OfflineResumeSchedule
	interruptions          []takeoverInterruption
	scheduleEvidenceSHA256 string
	liveStaleProbe         *LiveStaleProbeReceipt
	liveStaleProbeSHA256   string
	liveInterruption       *OfflineResumeInterruptionReceipt
	liveProbeRuns          []offlineLiveStaleInference
}

func runOfflineResumeFixedInference(
	ctx context.Context,
	config OfflinePromotionConfig,
	schedule OfflineResumeSchedule,
	executable string,
) (offlineResumeInference, error) {
	if schedule.Kind != ResumeOneSeededKill && schedule.Kind != ResumeFiveSeededKills {
		return offlineResumeInference{}, fmt.Errorf("fixed Resume runner received another schedule")
	}
	if err := config.Validate(); err != nil {
		return offlineResumeInference{}, err
	}
	executableSHA, err := validateOfflinePromotionIdentity(
		config, executable, buildversion.Commit, buildversion.SourceSHA256,
		buildversion.MigrationsSHA256, buildversion.Version,
	)
	if err != nil {
		return offlineResumeInference{}, err
	}
	migrations, err := loadReleaseMigrationBundle(executable, buildversion.MigrationsSHA256)
	if err != nil {
		return offlineResumeInference{}, err
	}
	temporary, err := os.MkdirTemp("", "omnidex-resume-fixed-")
	if err != nil {
		return offlineResumeInference{}, err
	}
	defer os.RemoveAll(temporary)
	setup, err := startOfflineResumeExecution(
		ctx, config, executable, executableSHA, migrations, temporary,
	)
	if err != nil {
		return offlineResumeInference{}, err
	}
	defer setup.close()
	first := inferenceBoundary{Kind: inferenceBoundaryDecisions, Count: schedule.DecisionBoundaries[0]}
	before, sourcePID, killedAt, err := runKilledInferenceAtBoundary(
		ctx, config, setup.database, setup.host, setup.paths, executable, executableSHA,
		first, temporary, setup.bundle, "resume-source",
	)
	if err != nil {
		return offlineResumeInference{}, err
	}
	sourceAttempt := setup.database.attempt
	interruptions := make([]takeoverInterruption, 0, len(schedule.DecisionBoundaries))
	for index, count := range schedule.DecisionBoundaries {
		boundary := inferenceBoundary{Kind: inferenceBoundaryDecisions, Count: count}
		if before.Boundary != boundary {
			return offlineResumeInference{}, fmt.Errorf("Resume chain changed its registered boundary")
		}
		replacement, err := claimResumeReplacement(ctx, setup.database, sourceAttempt)
		if err != nil {
			return offlineResumeInference{}, err
		}
		setup.database.attempt = replacement
		if index+1 < len(schedule.DecisionBoundaries) {
			next := inferenceBoundary{
				Kind: inferenceBoundaryDecisions, Count: schedule.DecisionBoundaries[index+1],
			}
			after, stopped, replacementPID, replacementKilledAt, err := runKilledChainedReplacement(
				ctx, config, setup.database, setup.host, setup.paths, executable, executableSHA,
				before, next, temporary, setup.bundle, index+1,
			)
			if err != nil {
				return offlineResumeInference{}, err
			}
			interruptions = append(interruptions, takeoverInterruption{
				Boundary: boundary, Before: before, After: after,
				Original: sourceAttempt, Replacement: replacement,
				OriginalPID: sourcePID, ReplacementPID: replacementPID,
				OriginalDied: killedAt,
			})
			sourceAttempt, sourcePID, killedAt = replacement, replacementPID, replacementKilledAt
			before = stopped
			continue
		}
		after, replacementPID, replacementExitedAt, err := runFinalReplacementInference(
			ctx, config, setup.database, setup.host, setup.paths, executable, executableSHA,
			boundary, temporary, setup.bundle, before, "resume-final",
		)
		if err != nil {
			return offlineResumeInference{}, err
		}
		interruptions = append(interruptions, takeoverInterruption{
			Boundary: boundary, Before: before, After: after,
			Original: sourceAttempt, Replacement: replacement,
			OriginalPID: sourcePID, ReplacementPID: replacementPID,
			OriginalDied: killedAt,
		})
		return finishOfflineResumeExecution(
			ctx, setup, schedule, interruptions, replacementPID, replacementExitedAt,
		)
	}
	return offlineResumeInference{}, fmt.Errorf("fixed Resume schedule had no interruption")
}

func claimResumeReplacement(
	ctx context.Context,
	database *offlinePromotionDatabase,
	previous model.StepAttemptAuthority,
) (model.StepAttemptAuthority, error) {
	reclaimCtx, cancel := resumeReclaimContext(ctx)
	defer cancel()
	return waitForReplacementClaim(reclaimCtx, database, previous)
}

func resumeReclaimContext(ctx context.Context) (context.Context, context.CancelFunc) {
	return context.WithTimeout(ctx, queue.StepAttemptLeaseDuration+30*time.Second)
}
