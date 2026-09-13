package queue

import (
	"fmt"
	"regexp"
)

var verificationContainerID = regexp.MustCompile(`^[0-9a-f]{64}$`)
var verificationContainerImageID = regexp.MustCompile(`^sha256:[0-9a-f]{64}$`)

func validateVerificationContainerEvidence(record VerificationCommandEvidence) error {
	if record.WorkingDirectory != "/workspace" {
		return fmt.Errorf("verification requires the exact Docker working directory")
	}
	if record.ContainerNetworkEnabled != nil && *record.ContainerNetworkEnabled &&
		record.Phase != VerificationIsolatedInstall && record.Phase != VerificationHostInstall {
		return fmt.Errorf("verification networking is restricted to dependency acquisition")
	}
	if record.ContainerID == "" {
		if record.ContainerImageID != "" || record.ContainerExecID != "" || record.ContainerNetworkEnabled != nil || record.LaunchError == "" {
			return fmt.Errorf("verification container evidence lacks its container identity")
		}
		return nil
	}
	if !verificationContainerID.MatchString(record.ContainerID) || !verificationContainerImageID.MatchString(record.ContainerImageID) ||
		(record.ContainerExecID != "" && !verificationContainerID.MatchString(record.ContainerExecID)) ||
		(record.ContainerExecID != "" && record.ContainerNetworkEnabled == nil) ||
		(record.ContainerExecID == "" && record.LaunchError == "") {
		return fmt.Errorf("verification container evidence requires exact image, container, and execution identities")
	}
	return nil
}
