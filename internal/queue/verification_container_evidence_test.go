package queue

import (
	"strings"
	"testing"
)

func TestVerificationContainerEvidenceRequiresActualExecutionAuthority(t *testing.T) {
	record := verificationCommandFixture()
	zero := 0
	record.ExitCode = &zero
	record.ContainerID = strings.Repeat("a", 64)
	record.ContainerImageID = "sha256:" + strings.Repeat("b", 64)
	record.ContainerExecID = strings.Repeat("c", 64)
	networkEnabled := false
	record.ContainerNetworkEnabled = &networkEnabled
	if _, err := normalizeVerificationCommandEvidence(record); err != nil {
		t.Fatal(err)
	}
	for name, change := range map[string]func(*VerificationCommandEvidence){
		"missing container":           func(r *VerificationCommandEvidence) { r.ContainerID = "" },
		"tag instead of image":        func(r *VerificationCommandEvidence) { r.ContainerImageID = "node:22" },
		"missing execution":           func(r *VerificationCommandEvidence) { r.ContainerExecID = "" },
		"missing network observation": func(r *VerificationCommandEvidence) { r.ContainerNetworkEnabled = nil },
		"malformed execution":         func(r *VerificationCommandEvidence) { r.ContainerExecID = "process" },
		"host working directory":      func(r *VerificationCommandEvidence) { r.WorkingDirectory = "/tmp/source" },
		"network outside acquisition": func(r *VerificationCommandEvidence) { enabled := true; r.ContainerNetworkEnabled = &enabled },
		"removed host cleanup phase":  func(r *VerificationCommandEvidence) { r.Phase = "host_cleanup" },
		"native process result": func(r *VerificationCommandEvidence) {
			r.ContainerID = ""
			r.ContainerImageID = ""
			r.ContainerExecID = ""
			r.ContainerNetworkEnabled = nil
		},
	} {
		t.Run(name, func(t *testing.T) {
			candidate := record
			change(&candidate)
			if _, err := normalizeVerificationCommandEvidence(candidate); err == nil {
				t.Fatal("inexact Docker authority accepted")
			}
		})
	}
	record.ContainerExecID = ""
	record.ExitCode = nil
	record.LaunchError = "Docker exec creation failed"
	if result, err := normalizeVerificationCommandEvidence(record); err != nil || result.Status != VerificationCommandLaunchFailed {
		t.Fatalf("observed launch failure = %+v, %v", result, err)
	}
	record.ContainerID, record.ContainerImageID = "", ""
	record.ContainerNetworkEnabled = nil
	if _, err := normalizeVerificationCommandEvidence(record); err != nil {
		t.Fatalf("pre-container failure was hidden: %v", err)
	}
}
