package queue

import (
	"crypto/sha256"
	"fmt"
	"net/netip"
	"path"
	"regexp"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

const (
	maxGeneratedDeploymentServices = 16
	maxGeneratedDeploymentSecrets  = 16
)

var (
	generatedDeploymentNamePattern = regexp.MustCompile(`^[a-z][a-z0-9_.-]{0,63}$`)
	generatedDeploymentVersion     = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.+-]{0,63}$`)
	generatedDeploymentProject     = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]{0,62}$`)
	generatedDeploymentService     = regexp.MustCompile(`^[a-z0-9][a-z0-9_.-]{0,62}$`)
	generatedDeploymentSecret      = regexp.MustCompile(`^[A-Z][A-Z0-9_]{0,127}$`)
	generatedDeploymentCode        = regexp.MustCompile(`^[a-z][a-z0-9_]{0,63}$`)
	generatedDeploymentDNSHost     = regexp.MustCompile(
		`^[a-z0-9](?:[a-z0-9.-]{0,251}[a-z0-9])?$`,
	)
)

func validateGeneratedWorkloadDeploymentCommand(command GeneratedWorkloadDeploymentCommand) error {
	authority := command.Authority
	if authority.JobID <= 0 || authority.Generation <= 0 || authority.StepID <= 0 ||
		authority.ProjectID <= 0 {
		return fmt.Errorf("generated deployment requires positive stable job, generation, step, and project authority")
	}
	for _, digest := range []struct{ name, value string }{
		{name: "deployment intent job", value: command.DeploymentIntentJobID},
		{name: "deployment intent response", value: command.DeploymentIntentResponseSHA256},
		{name: "workspace", value: command.WorkspaceSHA256},
		{name: "source snapshot", value: command.SourceSnapshotSHA256},
		{name: "Compose file", value: command.ComposeFileSHA256},
		{name: "structural config", value: command.ConfigSHA256},
		{name: "secret set", value: command.SecretSetSHA256},
	} {
		if !validSHA256Digest(digest.value) {
			return fmt.Errorf("generated deployment %s SHA-256 is invalid", digest.name)
		}
	}
	if command.ConfigSHA256 == command.ComposeFileSHA256 {
		return fmt.Errorf("generated deployment requires a distinct resolved Compose config proof")
	}
	if command.Disposition != GeneratedWorkloadDeploymentPersistCurrentHost {
		return fmt.Errorf("generated deployment disposition %q has no current-host authority", command.Disposition)
	}
	if !generatedDeploymentNamePattern.MatchString(command.AdapterID) ||
		!generatedDeploymentNamePattern.MatchString(command.ProfileID) {
		return fmt.Errorf("generated deployment adapter and profile identities are invalid")
	}
	if !generatedDeploymentVersion.MatchString(command.AdapterVersion) ||
		!generatedDeploymentVersion.MatchString(command.ProfileVersion) {
		return fmt.Errorf("generated deployment adapter and profile versions are invalid")
	}
	if !validSHA256ID(command.ComposeFileID, "file_") {
		return fmt.Errorf("generated deployment Compose file identity is invalid")
	}
	if !generatedDeploymentProject.MatchString(command.ComposeProject) {
		return fmt.Errorf("generated deployment Compose project identity is invalid")
	}
	if command.BindHost != GeneratedWorkloadDeploymentBindLoopback &&
		command.BindHost != GeneratedWorkloadDeploymentBindAllInterfaces {
		return fmt.Errorf("generated deployment bind-host authority %q is unsupported", command.BindHost)
	}
	switch command.EndpointPortAuthority {
	case GeneratedWorkloadDeploymentPortAllocate:
		if command.EndpointPort != 0 {
			return fmt.Errorf("generated deployment allocated port authority requires zero requested port")
		}
	case GeneratedWorkloadDeploymentPortFixed:
		if command.EndpointPort == 0 {
			return fmt.Errorf("generated deployment fixed port authority requires a positive requested port")
		}
	default:
		return fmt.Errorf("generated deployment endpoint port authority %q is unsupported", command.EndpointPortAuthority)
	}
	if err := validateGeneratedDeploymentEndpoint(
		command.EndpointScheme, command.EndpointHost, command.EndpointPath,
	); err != nil {
		return err
	}
	if err := validateGeneratedDeploymentStrings(
		"service", command.Services, 1, maxGeneratedDeploymentServices, generatedDeploymentService,
	); err != nil {
		return err
	}
	if err := validateGeneratedDeploymentStrings(
		"required secret name", command.RequiredSecretNames, 0,
		maxGeneratedDeploymentSecrets, generatedDeploymentSecret,
	); err != nil {
		return err
	}
	if command.PriorDeploymentID != "" &&
		!validSHA256ID(command.PriorDeploymentID, "generated_workload_deployment_") {
		return fmt.Errorf("generated deployment prior operation identity is invalid")
	}
	return nil
}

func validateGeneratedDeploymentEndpoint(scheme, host, endpointPath string) error {
	if scheme != "http" && scheme != "https" {
		return fmt.Errorf("generated deployment endpoint scheme %q is unsupported", scheme)
	}
	if host == "" || host != strings.TrimSpace(host) || len(host) > 253 ||
		!utf8.ValidString(host) || strings.ContainsAny(host, "\x00\r\n/@:[]") {
		return fmt.Errorf("generated deployment endpoint host is invalid")
	}
	if address, err := netip.ParseAddr(host); err == nil {
		if !address.IsValid() || address.IsUnspecified() || address.IsMulticast() || address.String() != host {
			return fmt.Errorf("generated deployment endpoint address is not canonical")
		}
	} else if !generatedDeploymentDNSHost.MatchString(host) || strings.Contains(host, "..") {
		return fmt.Errorf("generated deployment endpoint DNS host is invalid")
	}
	if endpointPath == "" || len(endpointPath) > 512 || endpointPath[0] != '/' ||
		path.Clean(endpointPath) != endpointPath || strings.ContainsAny(endpointPath, "?#\\\x00\r\n") ||
		strings.IndexFunc(endpointPath, unicode.IsSpace) >= 0 {
		return fmt.Errorf("generated deployment endpoint path is not canonical")
	}
	return nil
}

func validateGeneratedDeploymentStrings(
	label string,
	values []string,
	minimum, maximum int,
	pattern *regexp.Regexp,
) error {
	if len(values) < minimum || len(values) > maximum {
		return fmt.Errorf("generated deployment requires %d-%d %ss", minimum, maximum, label)
	}
	previous := ""
	for index, value := range values {
		if !pattern.MatchString(value) {
			return fmt.Errorf("generated deployment %s %d is invalid", label, index)
		}
		if index > 0 && value <= previous {
			return fmt.Errorf("generated deployment %ss must be sorted and unique", label)
		}
		previous = value
	}
	return nil
}

func validateGeneratedWorkloadDeploymentTransition(
	transition GeneratedWorkloadDeploymentTransition,
) error {
	switch transition.State {
	case GeneratedWorkloadDeploymentApplying:
		if transition.Code != "" || transition.DetailSHA256 != "" {
			return fmt.Errorf("generated deployment applying transition cannot contain failure detail")
		}
	case GeneratedWorkloadDeploymentFailed,
		GeneratedWorkloadDeploymentIndeterminate,
		GeneratedWorkloadDeploymentRolledBack:
		if !generatedDeploymentCode.MatchString(transition.Code) ||
			!validSHA256Digest(transition.DetailSHA256) {
			return fmt.Errorf("generated deployment %s transition requires a typed code and detail SHA-256", transition.State)
		}
	default:
		return fmt.Errorf("generated deployment transition target %q is unavailable", transition.State)
	}
	return nil
}

func validateGeneratedDeploymentTime(name string, value time.Time) error {
	if value.IsZero() || value.Location() != time.UTC || value.Nanosecond()%1000 != 0 {
		return fmt.Errorf("generated deployment receipt %s must be nonzero UTC microsecond time", name)
	}
	return nil
}

func generatedDeploymentSHA(value string) string {
	digest := sha256.Sum256([]byte(value))
	return fmt.Sprintf("%x", digest[:])
}
