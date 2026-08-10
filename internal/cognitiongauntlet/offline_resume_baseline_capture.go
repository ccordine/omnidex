package cognitiongauntlet

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
)

func recordResumeBaselineCheckpoint(
	ctx context.Context,
	control inferenceProcessControl,
	execution publicFullCognitionExecution,
	decisionBoundary uint32,
) error {
	if control.Mode != inferenceRecordResumeBaseline {
		return fmt.Errorf("Resume baseline capture requires its exact process mode")
	}
	preCall, err := CaptureSemanticPreCallCheckpoint(
		ctx, execution.components.repository, execution.episode.ID,
		episodeAttemptAuthority(execution),
	)
	if err != nil {
		return err
	}
	checkpoint := ResumeBaselineCheckpoint{
		DecisionBoundary: decisionBoundary, PreCall: preCall,
	}
	if err := checkpoint.Validate(); err != nil {
		return err
	}
	path := filepath.Join(
		control.BaselineDirectory, fmt.Sprintf("%08d.json", decisionBoundary),
	)
	return sealScenarioArtifact(path, checkpoint, "Resume baseline checkpoint")
}

func loadResumeBaselineCheckpoints(
	directory string,
	maximum int,
) ([]ResumeBaselineCheckpoint, error) {
	if directory == "" || filepath.Clean(directory) != directory || maximum < 1 {
		return nil, fmt.Errorf("Resume baseline checkpoint directory authority is invalid")
	}
	info, err := os.Lstat(directory)
	if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return nil, fmt.Errorf("Resume baseline checkpoint directory is unavailable")
	}
	entries, err := os.ReadDir(directory)
	if err != nil {
		return nil, err
	}
	if len(entries) == 0 || len(entries) > maximum+1 {
		return nil, fmt.Errorf("Resume baseline checkpoint count is outside the frozen budget")
	}
	result := make([]ResumeBaselineCheckpoint, len(entries))
	for index, entry := range entries {
		name := fmt.Sprintf("%08d.json", index)
		if entry.Name() != name || entry.Type()&os.ModeSymlink != 0 || entry.IsDir() {
			return nil, fmt.Errorf("Resume baseline checkpoint entry %d is not exact", index+1)
		}
		if err := loadStrictJSONFile(
			filepath.Join(directory, name), &result[index], "Resume baseline checkpoint",
		); err != nil {
			return nil, err
		}
		if result[index].DecisionBoundary != uint32(index) || result[index].Validate() != nil {
			return nil, fmt.Errorf("Resume baseline checkpoint entry %d changed authority", index+1)
		}
	}
	return result, nil
}
