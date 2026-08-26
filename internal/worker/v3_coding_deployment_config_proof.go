package worker

import (
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strings"

	"github.com/gryph/omnidex/internal/operation"
	"github.com/gryph/omnidex/internal/queue"
)

const directCodingResolvedConfigProofSchema = "omnidex.docker-compose-resolved-config.v1"

var directCodingResolvedConfigHashPattern = regexp.MustCompile(`^[0-9a-f]{64}$`)

type directCodingResolvedConfigService struct {
	Service string `json:"service"`
	SHA256  string `json:"sha256"`
}

type directCodingResolvedConfigProof struct {
	ConfigSHA256 string
	EvidenceID   int64
}

func (s *directCodingSession) proveDirectCodingResolvedDeploymentConfig(
	root string,
	project string,
	descriptor directCodingDeploymentDescriptor,
	environment map[string]string,
	expectedServices []string,
	workspaceSHA256 string,
	secretSetSHA256 string,
	socketPath string,
) (directCodingResolvedConfigProof, error) {
	if s == nil || s.program == nil {
		return directCodingResolvedConfigProof{}, fmt.Errorf(
			"resolved Compose config proof requires one compiled project profile",
		)
	}
	result, err := s.executeDirectCodingDeploymentCommand(
		root, directCodingDeploymentConfig, s.program.VersionProfileID,
		project, descriptor, environment,
	)
	if err != nil {
		return directCodingResolvedConfigProof{}, fmt.Errorf("execute resolved Compose config proof: %w", err)
	}
	raw, err := json.Marshal(result)
	if err != nil {
		return directCodingResolvedConfigProof{}, err
	}
	if err := validateDirectCodingDeploymentEnvironmentAbsentFromText(string(raw), environment); err != nil {
		return directCodingResolvedConfigProof{}, err
	}
	if err := directCodingDeploymentCommandSucceeded(directCodingDeploymentConfig, result); err != nil {
		return directCodingResolvedConfigProof{}, err
	}
	configSHA256, services, err := directCodingResolvedConfigSHA256(
		result, expectedServices, workspaceSHA256, secretSetSHA256,
	)
	if err != nil {
		return directCodingResolvedConfigProof{}, err
	}
	if len(result.Evidence) != 1 {
		return directCodingResolvedConfigProof{}, fmt.Errorf("resolved Compose config proof produced %d evidence rows, expected one", len(result.Evidence))
	}
	namespaceProof, err := proveDirectCodingDeploymentNamespaceVacant(
		s.runtime.ctx, socketPath, project,
	)
	if err != nil {
		return directCodingResolvedConfigProof{}, fmt.Errorf(
			"prove vacant deployment Compose namespace: %w", err,
		)
	}
	record := &result.Evidence[0]
	if record.Metadata == nil {
		return directCodingResolvedConfigProof{}, fmt.Errorf(
			"resolved Compose config proof omitted evidence metadata",
		)
	}
	record.SourceType = queue.GeneratedWorkloadResolvedConfigEvidenceSource
	environmentNames := make([]string, 0, len(environment))
	for name := range environment {
		environmentNames = append(environmentNames, name)
	}
	sort.Strings(environmentNames)
	record.Metadata["resolved_config_sha256"] = configSHA256
	record.Metadata["workspace_sha256"] = workspaceSHA256
	record.Metadata["secret_set_sha256"] = secretSetSHA256
	record.Metadata["service_hashes"] = services
	record.Metadata["implicit_env_disabled"] = true
	record.Metadata["environment_names"] = environmentNames
	record.Metadata[queue.GeneratedWorkloadDeploymentNamespaceMetadataKey] = namespaceProof
	ids, err := s.persistCodeOwnedEvidenceIDs(result)
	if err != nil || len(ids) != 1 {
		return directCodingResolvedConfigProof{}, fmt.Errorf("persist resolved Compose config proof: %w", err)
	}
	return directCodingResolvedConfigProof{ConfigSHA256: configSHA256, EvidenceID: ids[0]}, nil
}

func directCodingResolvedConfigSHA256(
	result operation.Result,
	expectedServices []string,
	workspaceSHA256 string,
	secretSetSHA256 string,
) (string, []directCodingResolvedConfigService, error) {
	services, err := parseDirectCodingResolvedConfigHashes(result, expectedServices)
	if err != nil {
		return "", nil, err
	}
	canonical, err := json.Marshal(struct {
		Schema          string                              `json:"schema"`
		WorkspaceSHA256 string                              `json:"workspace_sha256"`
		SecretSetSHA256 string                              `json:"secret_set_sha256"`
		Services        []directCodingResolvedConfigService `json:"services"`
	}{directCodingResolvedConfigProofSchema, workspaceSHA256, secretSetSHA256, services})
	if err != nil {
		return "", nil, err
	}
	return directCodingDigest(string(canonical)), services, nil
}

func parseDirectCodingResolvedConfigHashes(
	result operation.Result,
	expectedServices []string,
) ([]directCodingResolvedConfigService, error) {
	if truncated, ok := result.Output["stdout_truncated"].(bool); !ok || truncated {
		return nil, fmt.Errorf("resolved Compose config output is missing or truncated")
	}
	stdout, ok := result.Output["stdout"].(string)
	if !ok || stdout == "" || len(stdout) > 4096 || strings.Contains(stdout, "\r") {
		return nil, fmt.Errorf("resolved Compose config output is invalid")
	}
	stderr, _ := result.Output["stderr"].(string)
	if strings.TrimSpace(stderr) != "" {
		return nil, fmt.Errorf("resolved Compose config emitted diagnostic output")
	}
	lines := strings.Split(strings.TrimSuffix(stdout, "\n"), "\n")
	if len(lines) != len(expectedServices) {
		return nil, fmt.Errorf("resolved Compose config service count differs from command authority")
	}
	services := make([]directCodingResolvedConfigService, len(lines))
	for index, line := range lines {
		parts := strings.Split(line, " ")
		if len(parts) != 2 || parts[0] != expectedServices[index] ||
			!directCodingResolvedConfigHashPattern.MatchString(parts[1]) {
			return nil, fmt.Errorf("resolved Compose config line %d is not canonical", index)
		}
		services[index] = directCodingResolvedConfigService{Service: parts[0], SHA256: parts[1]}
	}
	return services, nil
}
