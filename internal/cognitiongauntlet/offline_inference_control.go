package cognitiongauntlet

import (
	"fmt"
	"path/filepath"
)

type inferenceProcessMode string

const (
	inferenceRunToTerminal      inferenceProcessMode = "run_to_terminal"
	inferenceStopBeforeNextCall inferenceProcessMode = "stop_before_next_call"
)

type inferenceProcessControl struct {
	Mode                   inferenceProcessMode `json:"mode"`
	AfterSuccessfulActions uint32               `json:"after_successful_actions"`
	CheckpointPath         string               `json:"checkpoint_path"`
	ResumeCheckpointPath   string               `json:"resume_checkpoint_path"`
}

func terminalInferenceControl() inferenceProcessControl {
	return inferenceProcessControl{Mode: inferenceRunToTerminal}
}

func checkpointInferenceControl(
	afterSuccessfulActions uint32,
	checkpointPath string,
) (inferenceProcessControl, error) {
	control := inferenceProcessControl{
		Mode: inferenceStopBeforeNextCall, AfterSuccessfulActions: afterSuccessfulActions,
		CheckpointPath: checkpointPath,
	}
	return control, control.Validate()
}

func replacementInferenceControl(
	afterSuccessfulActions uint32,
	checkpointPath string,
	resumeCheckpointPath string,
) (inferenceProcessControl, error) {
	control := inferenceProcessControl{
		Mode: inferenceStopBeforeNextCall, AfterSuccessfulActions: afterSuccessfulActions,
		CheckpointPath: checkpointPath, ResumeCheckpointPath: resumeCheckpointPath,
	}
	return control, control.Validate()
}

func (control inferenceProcessControl) Validate() error {
	switch control.Mode {
	case inferenceRunToTerminal:
		if control.AfterSuccessfulActions != 0 || control.CheckpointPath != "" ||
			control.ResumeCheckpointPath != "" {
			return fmt.Errorf("terminal inference control cannot carry a pause boundary")
		}
	case inferenceStopBeforeNextCall:
		if control.AfterSuccessfulActions == 0 || control.CheckpointPath == "" ||
			filepath.Clean(control.CheckpointPath) != control.CheckpointPath {
			return fmt.Errorf("checkpoint inference control requires one exact positive boundary and path")
		}
		if control.ResumeCheckpointPath != "" &&
			filepath.Clean(control.ResumeCheckpointPath) != control.ResumeCheckpointPath {
			return fmt.Errorf("checkpoint inference resume path must be exact")
		}
		if control.CheckpointPath == control.ResumeCheckpointPath {
			return fmt.Errorf("checkpoint inference cannot overwrite its resume evidence")
		}
	default:
		return fmt.Errorf("offline inference process mode %q is not registered", control.Mode)
	}
	return nil
}
