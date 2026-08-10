package cognitiongauntlet

import (
	"fmt"
	"path/filepath"
)

type inferenceProcessMode string
type inferenceBoundaryKind string

const (
	inferenceRunToTerminal        inferenceProcessMode  = "run_to_terminal"
	inferenceStopBeforeNextCall   inferenceProcessMode  = "stop_before_next_call"
	inferenceProbeStalePort       inferenceProcessMode  = "probe_stale_runtime_port"
	inferenceRecoverStalePort     inferenceProcessMode  = "recover_stale_runtime_port"
	inferenceRecordResumeBaseline inferenceProcessMode  = "record_resume_baseline"
	inferenceBoundaryActions      inferenceBoundaryKind = "successful_actions"
	inferenceBoundaryDecisions    inferenceBoundaryKind = "policy_decisions"
)

type inferenceBoundary struct {
	Kind  inferenceBoundaryKind `json:"kind"`
	Count uint32                `json:"count"`
}

type inferenceProcessControl struct {
	Mode                   inferenceProcessMode `json:"mode"`
	StopBoundary           inferenceBoundary    `json:"stop_boundary"`
	CheckpointPath         string               `json:"checkpoint_path"`
	ResumeCheckpointPath   string               `json:"resume_checkpoint_path"`
	ResumeVerificationPath string               `json:"resume_verification_path"`
	ResumeBoundary         inferenceBoundary    `json:"resume_boundary"`
	ProbePort              liveStalePort        `json:"probe_port"`
	ProbeCheckpointPath    string               `json:"probe_checkpoint_path"`
	ProbeRejectionPath     string               `json:"probe_rejection_path"`
	GeneratePausePath      string               `json:"generate_pause_path"`
	BaselineDirectory      string               `json:"baseline_directory"`
}

func resumeBaselineInferenceControl(
	directory string,
) (inferenceProcessControl, error) {
	control := inferenceProcessControl{
		Mode: inferenceRecordResumeBaseline, BaselineDirectory: directory,
	}
	return control, control.Validate()
}

func terminalInferenceControl() inferenceProcessControl {
	return inferenceProcessControl{Mode: inferenceRunToTerminal}
}

func checkpointInferenceControl(
	afterSuccessfulActions uint32,
	checkpointPath string,
) (inferenceProcessControl, error) {
	control := inferenceProcessControl{
		Mode:           inferenceStopBeforeNextCall,
		StopBoundary:   inferenceBoundary{Kind: inferenceBoundaryActions, Count: afterSuccessfulActions},
		CheckpointPath: checkpointPath,
	}
	return control, control.Validate()
}

func replacementInferenceControl(
	afterSuccessfulActions uint32,
	checkpointPath string,
	resumeCheckpointPath string,
) (inferenceProcessControl, error) {
	return newReplacementInferenceControl(
		inferenceBoundary{Kind: inferenceBoundaryActions, Count: afterSuccessfulActions},
		checkpointPath, resumeCheckpointPath,
	)
}

func newReplacementInferenceControl(
	resumeBoundary inferenceBoundary,
	checkpointPath string,
	resumeCheckpointPath string,
) (inferenceProcessControl, error) {
	control := inferenceProcessControl{
		Mode: inferenceRunToTerminal, ResumeCheckpointPath: resumeCheckpointPath,
		ResumeVerificationPath: checkpointPath,
		ResumeBoundary:         resumeBoundary,
	}
	return control, control.Validate()
}

func chainedReplacementInferenceControl(
	afterSuccessfulActions uint32,
	checkpointPath string,
	resumeCheckpointPath string,
	resumeVerificationPath string,
	resumeBoundary inferenceBoundary,
) (inferenceProcessControl, error) {
	return newChainedReplacementInferenceControl(
		inferenceBoundary{Kind: inferenceBoundaryActions, Count: afterSuccessfulActions},
		checkpointPath, resumeCheckpointPath, resumeVerificationPath, resumeBoundary,
	)
}

func newChainedReplacementInferenceControl(
	stopBoundary inferenceBoundary,
	checkpointPath string,
	resumeCheckpointPath string,
	resumeVerificationPath string,
	resumeBoundary inferenceBoundary,
) (inferenceProcessControl, error) {
	control := inferenceProcessControl{
		Mode:           inferenceStopBeforeNextCall,
		StopBoundary:   stopBoundary,
		CheckpointPath: checkpointPath, ResumeCheckpointPath: resumeCheckpointPath,
		ResumeVerificationPath: resumeVerificationPath, ResumeBoundary: resumeBoundary,
	}
	return control, control.Validate()
}

func decisionCheckpointInferenceControl(
	afterPolicyDecisions uint32,
	checkpointPath string,
) (inferenceProcessControl, error) {
	control := inferenceProcessControl{
		Mode:           inferenceStopBeforeNextCall,
		StopBoundary:   inferenceBoundary{Kind: inferenceBoundaryDecisions, Count: afterPolicyDecisions},
		CheckpointPath: checkpointPath,
	}
	return control, control.Validate()
}

func liveStalePortInferenceControl(
	port liveStalePort,
	checkpointPath string,
	rejectionPath string,
	generatePausePath string,
) (inferenceProcessControl, error) {
	control := inferenceProcessControl{
		Mode: inferenceProbeStalePort, ProbePort: port,
		ProbeCheckpointPath: checkpointPath, ProbeRejectionPath: rejectionPath,
		GeneratePausePath: generatePausePath,
	}
	return control, control.Validate()
}

func (control inferenceProcessControl) Validate() error {
	switch control.Mode {
	case inferenceRunToTerminal:
		if control.StopBoundary != (inferenceBoundary{}) || control.CheckpointPath != "" {
			return fmt.Errorf("terminal inference control cannot carry a pause boundary")
		}
	case inferenceStopBeforeNextCall:
		if control.StopBoundary.Validate() != nil || control.CheckpointPath == "" ||
			filepath.Clean(control.CheckpointPath) != control.CheckpointPath {
			return fmt.Errorf("checkpoint inference control requires one exact positive boundary and path")
		}
	case inferenceProbeStalePort:
		if control.StopBoundary != (inferenceBoundary{}) || control.CheckpointPath != "" ||
			control.ProbePort.Validate() != nil || control.ProbeCheckpointPath == "" ||
			control.ProbeRejectionPath == "" ||
			filepath.Clean(control.ProbeCheckpointPath) != control.ProbeCheckpointPath ||
			filepath.Clean(control.ProbeRejectionPath) != control.ProbeRejectionPath ||
			control.ProbeCheckpointPath == control.ProbeRejectionPath {
			return fmt.Errorf("live stale-port inference control requires exact distinct private outputs")
		}
		if control.ProbePort == liveStalePolicyFinish {
			if control.GeneratePausePath == "" ||
				filepath.Clean(control.GeneratePausePath) != control.GeneratePausePath ||
				control.GeneratePausePath == control.ProbeCheckpointPath ||
				control.GeneratePausePath == control.ProbeRejectionPath {
				return fmt.Errorf("policy-finish probe requires an exact live-generation boundary")
			}
		} else if control.GeneratePausePath != "" {
			return fmt.Errorf("nonpolicy stale-port probe cannot carry a generation boundary")
		}
	case inferenceRecoverStalePort:
		if control.StopBoundary != (inferenceBoundary{}) || control.CheckpointPath != "" ||
			control.ProbePort.Validate() != nil || control.ProbeCheckpointPath != "" ||
			control.ProbeRejectionPath != "" || control.GeneratePausePath != "" {
			return fmt.Errorf("live stale-port recovery requires only one registered port")
		}
	case inferenceRecordResumeBaseline:
		if control.StopBoundary != (inferenceBoundary{}) || control.CheckpointPath != "" ||
			control.BaselineDirectory == "" ||
			filepath.Clean(control.BaselineDirectory) != control.BaselineDirectory {
			return fmt.Errorf("Resume baseline control requires one exact private directory")
		}
	default:
		return fmt.Errorf("offline inference process mode %q is not registered", control.Mode)
	}
	if (control.ResumeCheckpointPath == "") != (control.ResumeVerificationPath == "") ||
		(control.ResumeCheckpointPath == "") != (control.ResumeBoundary == (inferenceBoundary{})) {
		return fmt.Errorf("inference resume requires exact source and verification paths")
	}
	if control.ResumeCheckpointPath != "" &&
		filepath.Clean(control.ResumeCheckpointPath) != control.ResumeCheckpointPath {
		return fmt.Errorf("checkpoint inference resume path must be exact")
	}
	if control.Mode != inferenceProbeStalePort && control.Mode != inferenceRecoverStalePort &&
		(control.ProbePort != "" || control.ProbeCheckpointPath != "" ||
			control.ProbeRejectionPath != "" || control.GeneratePausePath != "") {
		return fmt.Errorf("ordinary inference control cannot carry stale-port outputs")
	}
	if control.Mode != inferenceRecordResumeBaseline && control.BaselineDirectory != "" {
		return fmt.Errorf("ordinary inference control cannot carry a Resume baseline directory")
	}
	if control.ResumeVerificationPath != "" &&
		filepath.Clean(control.ResumeVerificationPath) != control.ResumeVerificationPath {
		return fmt.Errorf("checkpoint inference verification path must be exact")
	}
	if control.CheckpointPath != "" &&
		(control.CheckpointPath == control.ResumeCheckpointPath ||
			control.CheckpointPath == control.ResumeVerificationPath) {
		return fmt.Errorf("checkpoint inference cannot overwrite its resume evidence")
	}
	if control.ResumeCheckpointPath != "" &&
		control.ResumeCheckpointPath == control.ResumeVerificationPath {
		return fmt.Errorf("checkpoint inference cannot overwrite its source evidence")
	}
	if control.Mode == inferenceStopBeforeNextCall && control.ResumeCheckpointPath != "" &&
		control.StopBoundary.Kind == control.ResumeBoundary.Kind &&
		control.StopBoundary.Count <= control.ResumeBoundary.Count {
		return fmt.Errorf("chained inference checkpoint must advance beyond its resume boundary")
	}
	return nil
}

func liveStalePortRecoveryControl(port liveStalePort) (inferenceProcessControl, error) {
	control := inferenceProcessControl{Mode: inferenceRecoverStalePort, ProbePort: port}
	return control, control.Validate()
}

func (boundary inferenceBoundary) Validate() error {
	if boundary.Count == 0 ||
		(boundary.Kind != inferenceBoundaryActions && boundary.Kind != inferenceBoundaryDecisions) {
		return fmt.Errorf("inference boundary is not registered")
	}
	return nil
}
