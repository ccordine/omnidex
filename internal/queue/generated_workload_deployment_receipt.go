package queue

import (
	"fmt"
)

func validateGeneratedWorkloadDeploymentReceipt(
	command GeneratedWorkloadDeploymentCommand,
	receipt GeneratedWorkloadDeploymentReceipt,
	identity generatedWorkloadDeploymentIdentity,
) error {
	if err := validateGeneratedWorkloadDeploymentCommand(command); err != nil {
		return err
	}
	if receipt.Schema != GeneratedWorkloadDeploymentReceiptV2 {
		return fmt.Errorf("generated deployment receipt schema is invalid")
	}
	if receipt.OperationID != identity.OperationID ||
		receipt.ConfigSHA256 != command.ConfigSHA256 ||
		receipt.ComposeProject != command.ComposeProject ||
		receipt.EndpointScheme != command.EndpointScheme ||
		receipt.EndpointHost != command.EndpointHost ||
		receipt.EndpointPath != command.EndpointPath ||
		receipt.PriorDeploymentID != command.PriorDeploymentID {
		return fmt.Errorf("generated deployment receipt disagrees with immutable command authority")
	}
	if err := validateGeneratedDeploymentEndpoint(
		receipt.EndpointScheme, receipt.EndpointHost, receipt.EndpointPath,
	); err != nil {
		return err
	}
	if receipt.EndpointPort == 0 ||
		command.EndpointPortAuthority == GeneratedWorkloadDeploymentPortFixed &&
			receipt.EndpointPort != command.EndpointPort {
		return fmt.Errorf("generated deployment receipt endpoint port disagrees with allocation authority")
	}
	if err := validateGeneratedDeploymentTime("applied_at", receipt.AppliedAt); err != nil {
		return err
	}
	if err := validateGeneratedDeploymentTime("observed_at", receipt.ObservedAt); err != nil {
		return err
	}
	if receipt.ObservedAt.Before(receipt.AppliedAt) {
		return fmt.Errorf("generated deployment receipt observation predates application")
	}
	if len(receipt.Services) != len(command.Services) {
		return fmt.Errorf("generated deployment receipt service set is incomplete")
	}
	for index, service := range receipt.Services {
		if service.Service != command.Services[index] {
			return fmt.Errorf("generated deployment receipt services must match exact sorted command authority")
		}
		if !validSHA256Digest(service.ContainerID) {
			return fmt.Errorf("generated deployment service %q container identity is invalid", service.Service)
		}
		if !validSHA256ID(service.ImageDigest, "sha256:") {
			return fmt.Errorf("generated deployment service %q image digest is invalid", service.Service)
		}
		switch service.RestartPolicy {
		case GeneratedWorkloadDeploymentRestartNo,
			GeneratedWorkloadDeploymentRestartAlways,
			GeneratedWorkloadDeploymentRestartOnFailure,
			GeneratedWorkloadDeploymentRestartUnlessStopped:
		default:
			return fmt.Errorf("generated deployment service %q restart policy is invalid", service.Service)
		}
		if service.State != "running" || service.Health != "healthy" {
			return fmt.Errorf("generated deployment service %q is not running and healthy", service.Service)
		}
	}
	if !validSHA256ID(
		receipt.WorkspaceVerificationReceiptID, "generated_workload_verification_",
	) {
		return fmt.Errorf("generated deployment workspace verification receipt identity is invalid")
	}
	if err := validateGeneratedDeploymentEvidenceIDs(receipt.ExecutionEvidenceIDs, 6, 9); err != nil {
		return fmt.Errorf("generated deployment execution receipt: %w", err)
	}
	if err := validateGeneratedDeploymentEvidenceIDs(receipt.ObservationEvidenceIDs, 2, 2); err != nil {
		return fmt.Errorf("generated deployment observation receipt: %w", err)
	}
	return nil
}

func canonicalGeneratedWorkloadDeploymentReceipt(
	command GeneratedWorkloadDeploymentCommand,
	receipt GeneratedWorkloadDeploymentReceipt,
	identity generatedWorkloadDeploymentIdentity,
) (string, string, error) {
	if err := validateGeneratedWorkloadDeploymentReceipt(command, receipt, identity); err != nil {
		return "", "", err
	}
	encoded, err := canonicalGeneratedDeploymentJSON(receipt)
	if err != nil {
		return "", "", fmt.Errorf("encode canonical generated deployment receipt: %w", err)
	}
	if len(encoded) > 32768 {
		return "", "", fmt.Errorf("canonical generated deployment receipt exceeds 32768 bytes")
	}
	return encoded, generatedDeploymentSHA(encoded), nil
}
