package worker

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/netip"
	"regexp"
	"sort"
	"strconv"
)

const (
	directCodingComposePSLimit              = 1 << 20
	directCodingServiceLimit                = 16
	directCodingDeploymentObservationSchema = "omnidex.generated-service-observation.v1"
)

var directCodingContainerIDPattern = regexp.MustCompile(`^[a-f0-9]{12,64}$`)

type directCodingDeploymentObservationRequest struct {
	Project              string
	ExpectedServices     []string
	GatewayService       string
	GatewayContainerPort uint16
	BindAddress          string
	ProbeHost            string
	AdvertisedHost       string
	ReadinessPath        string
}

type directCodingObservedService struct {
	Service       string `json:"service"`
	ContainerID   string `json:"container_id"`
	ImageID       string `json:"image_id"`
	RestartPolicy string `json:"restart_policy"`
	State         string `json:"state"`
	Health        string `json:"health"`
}

type directCodingObservedEndpoint struct {
	Scheme string `json:"scheme"`
	Host   string `json:"host"`
	Port   uint16 `json:"port"`
	Path   string `json:"path"`
}

type directCodingDeploymentObservation struct {
	Schema         string                        `json:"schema"`
	Project        string                        `json:"project"`
	Services       []directCodingObservedService `json:"services"`
	Endpoint       directCodingObservedEndpoint  `json:"endpoint"`
	ServicesSHA256 string                        `json:"services_sha256"`
	EndpointSHA256 string                        `json:"endpoint_sha256"`
	SHA256         string                        `json:"sha256"`
}

type directCodingComposePSRow struct {
	ID         string
	Project    string
	Service    string
	State      string
	Health     string
	Publishers []directCodingComposePublisher
}

type directCodingComposePublisher struct {
	URL           string `json:"URL"`
	TargetPort    int    `json:"TargetPort"`
	PublishedPort int    `json:"PublishedPort"`
	Protocol      string `json:"Protocol"`
}

func observeDirectCodingDeployment(
	ctx context.Context,
	socketPath string,
	composePS []byte,
	request directCodingDeploymentObservationRequest,
) (directCodingDeploymentObservation, error) {
	return observeDirectCodingDeploymentWithReadiness(
		ctx, socketPath, composePS, request, probeDirectCodingDeploymentReadiness,
	)
}

func observeDirectCodingDeploymentWithReadiness(
	ctx context.Context,
	socketPath string,
	composePS []byte,
	request directCodingDeploymentObservationRequest,
	readiness func(context.Context, string, uint16, string) error,
) (directCodingDeploymentObservation, error) {
	inspection := func(
		ctx context.Context,
		rows []directCodingComposePSRow,
		request directCodingDeploymentObservationRequest,
	) ([]directCodingObservedService, uint16, error) {
		return inspectDirectCodingDeployment(ctx, socketPath, rows, request)
	}
	return observeDirectCodingDeploymentWithObservers(
		ctx, composePS, request, inspection, readiness,
	)
}

func observeDirectCodingDeploymentWithObservers(
	ctx context.Context,
	composePS []byte,
	request directCodingDeploymentObservationRequest,
	inspection func(
		context.Context,
		[]directCodingComposePSRow,
		directCodingDeploymentObservationRequest,
	) ([]directCodingObservedService, uint16, error),
	readiness func(context.Context, string, uint16, string) error,
) (directCodingDeploymentObservation, error) {
	if err := validateDirectCodingObservationRequest(request); err != nil {
		return directCodingDeploymentObservation{}, err
	}
	if inspection == nil || readiness == nil {
		return directCodingDeploymentObservation{}, fmt.Errorf("deployment inspection and readiness observers are required")
	}
	rows, err := parseDirectCodingComposePS(composePS, request.Project, request.ExpectedServices)
	if err != nil {
		return directCodingDeploymentObservation{}, err
	}
	services, port, err := inspection(ctx, rows, request)
	if err != nil {
		return directCodingDeploymentObservation{}, err
	}
	if err := validateDirectCodingObservedServices(services, request.ExpectedServices, port); err != nil {
		return directCodingDeploymentObservation{}, err
	}
	if err := readiness(
		ctx, request.ProbeHost, port, request.ReadinessPath,
	); err != nil {
		return directCodingDeploymentObservation{}, err
	}
	if request.AdvertisedHost != request.ProbeHost {
		if err := readiness(
			ctx, request.AdvertisedHost, port, request.ReadinessPath,
		); err != nil {
			return directCodingDeploymentObservation{}, fmt.Errorf("probe advertised deployment endpoint: %w", err)
		}
	}
	endpoint := directCodingObservedEndpoint{
		Scheme: "http", Host: request.AdvertisedHost, Port: port, Path: request.ReadinessPath,
	}
	serviceHash, err := directCodingObservationHash("services.v1", services)
	if err != nil {
		return directCodingDeploymentObservation{}, err
	}
	endpointHash, err := directCodingObservationHash("endpoint.v1", endpoint)
	if err != nil {
		return directCodingDeploymentObservation{}, err
	}
	canonical := struct {
		Schema   string                        `json:"schema"`
		Project  string                        `json:"project"`
		Services []directCodingObservedService `json:"services"`
		Endpoint directCodingObservedEndpoint  `json:"endpoint"`
	}{directCodingDeploymentObservationSchema, request.Project, services, endpoint}
	hash, err := directCodingObservationHash("observation.v1", canonical)
	if err != nil {
		return directCodingDeploymentObservation{}, err
	}
	return directCodingDeploymentObservation{
		Schema:  directCodingDeploymentObservationSchema,
		Project: request.Project, Services: services, Endpoint: endpoint,
		ServicesSHA256: serviceHash, EndpointSHA256: endpointHash, SHA256: hash,
	}, nil
}

func validateDirectCodingObservedServices(
	services []directCodingObservedService,
	expected []string,
	port uint16,
) error {
	if port == 0 || len(services) != len(expected) {
		return fmt.Errorf("deployment inspection returned incomplete service or endpoint authority")
	}
	for index, service := range services {
		if service.Service != expected[index] ||
			len(service.ContainerID) != 64 ||
			!directCodingContainerIDPattern.MatchString(service.ContainerID) ||
			!directCodingImageIDPattern.MatchString(service.ImageID) ||
			service.RestartPolicy != "unless-stopped" ||
			service.State != "running" || service.Health != "healthy" {
			return fmt.Errorf("deployment inspection returned non-canonical service %d", index)
		}
	}
	return nil
}

func validateDirectCodingObservationRequest(request directCodingDeploymentObservationRequest) error {
	if !v3DeploymentComposeProjectPattern.MatchString(request.Project) {
		return fmt.Errorf("deployment observation requires one exact Compose project")
	}
	if len(request.ExpectedServices) == 0 || len(request.ExpectedServices) > directCodingServiceLimit {
		return fmt.Errorf("deployment observation requires 1-%d expected services", directCodingServiceLimit)
	}
	previous := ""
	gateway := false
	for _, service := range request.ExpectedServices {
		if !generatedDeploymentServicePattern.MatchString(service) || service <= previous {
			return fmt.Errorf("deployment observation services must be canonical, sorted, and unique")
		}
		previous = service
		gateway = gateway || service == request.GatewayService
	}
	if !gateway || request.GatewayContainerPort == 0 {
		return fmt.Errorf("deployment observation gateway authority is incomplete")
	}
	bind, err := netip.ParseAddr(request.BindAddress)
	if err != nil || !bind.IsValid() || !bind.Is4() || bind.IsMulticast() ||
		bind.String() != request.BindAddress ||
		request.BindAddress != "127.0.0.1" && request.BindAddress != "0.0.0.0" {
		return fmt.Errorf("deployment observation bind address must be exact IPv4 loopback or all interfaces")
	}
	if err := validateDirectCodingDeploymentHost("probe", request.ProbeHost); err != nil {
		return err
	}
	if err := validateDirectCodingDeploymentHost("advertised", request.AdvertisedHost); err != nil {
		return err
	}
	if request.ReadinessPath != directCodingDeploymentReadinessPath {
		return fmt.Errorf("deployment observation readiness path is not registered")
	}
	return nil
}

var generatedDeploymentServicePattern = regexp.MustCompile(`^[a-z0-9][a-z0-9_.-]{0,62}$`)

func directCodingObservationHash(domain string, value any) (string, error) {
	encoded, err := json.Marshal(struct {
		Domain string `json:"domain"`
		Value  any    `json:"value"`
	}{domain, value})
	if err != nil {
		return "", fmt.Errorf("encode canonical deployment observation: %w", err)
	}
	digest := sha256.Sum256(encoded)
	return hex.EncodeToString(digest[:]), nil
}

func parseDirectCodingComposePS(
	output []byte,
	project string,
	expectedServices []string,
) ([]directCodingComposePSRow, error) {
	if len(output) == 0 {
		return nil, fmt.Errorf("deployment Compose observation is empty")
	}
	if len(output) > directCodingComposePSLimit {
		return nil, fmt.Errorf("deployment Compose observation exceeds the %d-byte limit", directCodingComposePSLimit)
	}
	if bytes.IndexByte(output, '\r') >= 0 || bytes.IndexByte(output, 0) >= 0 {
		return nil, fmt.Errorf("deployment Compose observation contains invalid bytes")
	}
	lines := bytes.Split(bytes.TrimSuffix(output, []byte{'\n'}), []byte{'\n'})
	rows := make([]directCodingComposePSRow, 0, len(lines))
	for index, line := range lines {
		row, err := decodeDirectCodingComposePSRow(line)
		if err != nil {
			return nil, fmt.Errorf("decode deployment Compose row %d: %w", index+1, err)
		}
		if row.Project != project {
			return nil, fmt.Errorf("deployment Compose row %d has project %q, expected %q", index+1, row.Project, project)
		}
		rows = append(rows, row)
	}
	sort.Slice(rows, func(i, j int) bool { return rows[i].Service < rows[j].Service })
	if len(rows) != len(expectedServices) {
		return nil, fmt.Errorf("deployment Compose service set has %d entries, expected %d", len(rows), len(expectedServices))
	}
	for index, expected := range expectedServices {
		if rows[index].Service != expected {
			return nil, fmt.Errorf("deployment Compose service set differs at %d", index)
		}
		if rows[index].State != "running" || rows[index].Health != "healthy" {
			return nil, fmt.Errorf("deployment Compose service %q is not running and healthy", expected)
		}
	}
	return rows, nil
}

func parseDirectCodingPublishedPort(value int) (uint16, error) {
	if value <= 0 || value > 65535 {
		return 0, fmt.Errorf("deployment published port %s is invalid", strconv.Itoa(value))
	}
	return uint16(value), nil
}
