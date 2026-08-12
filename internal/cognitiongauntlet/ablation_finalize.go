package cognitiongauntlet

import (
	"fmt"
	"time"
)

func finalizeAblationEpisode(
	episodePath string,
	evidencePath string,
	startedAt time.Time,
	publicAuthority PublicRunAuthority,
	recorder *EpisodeRecorder,
	state *ablationState,
	journal *ablationCallJournal,
	execution ablationExecution,
	bootstrap RuntimeBrainBootstrapEvidenceAuthority,
	activation RuntimeProviderActivationEvidenceAuthority,
) (SealedEpisode, AblationEvidenceAuthority, error) {
	if execution.Terminal == nil || execution.TerminalCause == nil ||
		execution.Terminal.Validate() != nil || execution.TerminalCause.Validate() != nil {
		return SealedEpisode{}, AblationEvidenceAuthority{},
			fmt.Errorf("ablation finalizer lacks its exact pending terminal")
	}
	artifact, evidenceAuthority, err := prepareAblationEvidence(
		evidencePath, publicAuthority, state, journal,
		*execution.Terminal, *execution.TerminalCause, execution.ContextBudget,
		bootstrap, activation,
	)
	if err != nil {
		return SealedEpisode{}, AblationEvidenceAuthority{}, err
	}
	if filepathBase(evidencePath) != evidenceAuthority.File {
		return SealedEpisode{}, AblationEvidenceAuthority{},
			fmt.Errorf("ablation evidence path differs from frozen basename")
	}
	if err := sealAblationEvidence(evidencePath, artifact, evidenceAuthority); err != nil {
		return SealedEpisode{}, AblationEvidenceAuthority{}, err
	}
	if _, err := loadAblationEvidence(evidencePath, evidenceAuthority); err != nil {
		return SealedEpisode{}, AblationEvidenceAuthority{}, err
	}
	if err := appendAblationEvidenceTrace(recorder, evidenceAuthority); err != nil {
		return SealedEpisode{}, AblationEvidenceAuthority{}, err
	}
	if err := appendPendingAblationTerminal(recorder, execution); err != nil {
		return SealedEpisode{}, AblationEvidenceAuthority{}, err
	}
	sealed, err := recorder.Seal(
		episodePath, startedAt, time.Now().UTC(), execution.Revision, execution.Outcome,
		execution.Resources, execution.Memory, execution.Planning, RecoveryMetrics{},
	)
	if err != nil {
		return SealedEpisode{}, AblationEvidenceAuthority{}, err
	}
	sealed, err = LoadSealedEpisode(episodePath)
	if err != nil {
		return SealedEpisode{}, AblationEvidenceAuthority{}, err
	}
	if err := verifySealedAblationEvidence(
		episodePath, evidencePath, sealed, publicAuthority,
	); err != nil {
		return SealedEpisode{}, AblationEvidenceAuthority{}, err
	}
	return sealed, evidenceAuthority, nil
}

func verifySealedAblationEvidence(
	episodePath string,
	evidencePath string,
	sealed SealedEpisode,
	publicAuthority PublicRunAuthority,
) error {
	if episodePath == evidencePath {
		return fmt.Errorf("episode and ablation evidence share one output path")
	}
	expected, err := NewAblationEvidenceExpectation(publicAuthority, sealed)
	if err != nil {
		return err
	}
	_, err = VerifyAblationEvidenceFor(evidencePath, sealed, expected)
	return err
}

func filepathBase(path string) string {
	for index := len(path) - 1; index >= 0; index-- {
		if path[index] == '/' {
			return path[index+1:]
		}
	}
	return path
}
