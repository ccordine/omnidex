package worker

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime"
	"net"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"
)

const (
	directCodingDockerInspectLimit   = 1 << 20
	directCodingDockerObserveTimeout = 8 * time.Second
)

type directCodingDockerInspection struct {
	ID     string `json:"Id"`
	Image  string `json:"Image"`
	Config *struct {
		Labels map[string]string `json:"Labels"`
	} `json:"Config"`
	HostConfig *struct {
		RestartPolicy struct {
			Name string `json:"Name"`
		} `json:"RestartPolicy"`
	} `json:"HostConfig"`
	State *struct {
		Status  string `json:"Status"`
		Running bool   `json:"Running"`
		Health  *struct {
			Status string `json:"Status"`
		} `json:"Health"`
	} `json:"State"`
	NetworkSettings *struct {
		Ports map[string][]directCodingDockerPortBinding `json:"Ports"`
	} `json:"NetworkSettings"`
}

type directCodingDockerPortBinding struct {
	HostIP   string `json:"HostIp"`
	HostPort string `json:"HostPort"`
}

func inspectDirectCodingDeployment(
	parent context.Context,
	socketPath string,
	rows []directCodingComposePSRow,
	request directCodingDeploymentObservationRequest,
) ([]directCodingObservedService, uint16, error) {
	if parent == nil {
		return nil, 0, fmt.Errorf("deployment Docker observation requires a context")
	}
	if err := validateV3DockerSocket(socketPath); err != nil {
		return nil, 0, err
	}
	if err := validateV3DockerDaemon(parent, socketPath); err != nil {
		return nil, 0, err
	}
	ctx, cancel := context.WithTimeout(parent, directCodingDockerObserveTimeout)
	defer cancel()
	transport := directCodingDockerUnixTransport(socketPath)
	defer transport.CloseIdleConnections()
	client := &http.Client{Transport: transport}
	return inspectDirectCodingDeploymentWithClient(ctx, client, rows, request)
}

func inspectDirectCodingDeploymentWithClient(
	ctx context.Context,
	client *http.Client,
	rows []directCodingComposePSRow,
	request directCodingDeploymentObservationRequest,
) ([]directCodingObservedService, uint16, error) {
	if ctx == nil || client == nil {
		return nil, 0, fmt.Errorf("deployment Docker inspection requires context and client authority")
	}
	services := make([]directCodingObservedService, 0, len(rows))
	var gatewayPort uint16
	for _, row := range rows {
		inspection, err := inspectDirectCodingContainer(ctx, client, row.ID)
		if err != nil {
			return nil, 0, fmt.Errorf("inspect deployment service %q: %w", row.Service, err)
		}
		service, port, err := validateDirectCodingContainerInspection(row, inspection, request)
		if err != nil {
			return nil, 0, fmt.Errorf("validate deployment service %q: %w", row.Service, err)
		}
		if row.Service == request.GatewayService {
			gatewayPort = port
		}
		services = append(services, service)
	}
	if gatewayPort == 0 {
		return nil, 0, fmt.Errorf("deployment gateway has no observed published port")
	}
	return services, gatewayPort, nil
}

func directCodingDockerUnixTransport(socketPath string) *http.Transport {
	return &http.Transport{
		DisableCompression: true,
		DisableKeepAlives:  true,
		DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
			var dialer net.Dialer
			return dialer.DialContext(ctx, "unix", socketPath)
		},
		MaxResponseHeaderBytes: 16 << 10,
	}
}

func inspectDirectCodingContainer(
	ctx context.Context,
	client *http.Client,
	containerID string,
) (directCodingDockerInspection, error) {
	if !directCodingContainerIDPattern.MatchString(containerID) {
		return directCodingDockerInspection{}, fmt.Errorf("container identity is invalid")
	}
	request, err := http.NewRequestWithContext(
		ctx, http.MethodGet,
		"http://docker.local/containers/"+url.PathEscape(containerID)+"/json", nil,
	)
	if err != nil {
		return directCodingDockerInspection{}, fmt.Errorf("construct Docker inspection request: %w", err)
	}
	response, err := client.Do(request)
	if err != nil {
		return directCodingDockerInspection{}, fmt.Errorf("execute Docker inspection request: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return directCodingDockerInspection{}, fmt.Errorf("Docker inspection returned HTTP status %d", response.StatusCode)
	}
	mediaType, _, err := mime.ParseMediaType(response.Header.Get("Content-Type"))
	if err != nil || mediaType != "application/json" {
		return directCodingDockerInspection{}, fmt.Errorf("Docker inspection returned a non-JSON content type")
	}
	limited := &io.LimitedReader{R: response.Body, N: directCodingDockerInspectLimit + 1}
	decoder := json.NewDecoder(limited)
	var inspection directCodingDockerInspection
	if err := decoder.Decode(&inspection); err != nil {
		if limited.N <= 0 {
			return directCodingDockerInspection{}, fmt.Errorf("Docker inspection exceeds the %d-byte limit", directCodingDockerInspectLimit)
		}
		return directCodingDockerInspection{}, fmt.Errorf("decode Docker inspection: %w", err)
	}
	if err := requireDirectCodingJSONEOF(decoder); err != nil {
		if limited.N <= 0 {
			return directCodingDockerInspection{}, fmt.Errorf("Docker inspection exceeds the %d-byte limit", directCodingDockerInspectLimit)
		}
		return directCodingDockerInspection{}, err
	}
	return inspection, nil
}

func validateDirectCodingContainerInspection(
	row directCodingComposePSRow,
	inspection directCodingDockerInspection,
	request directCodingDeploymentObservationRequest,
) (directCodingObservedService, uint16, error) {
	if len(inspection.ID) != 64 || !directCodingContainerIDPattern.MatchString(inspection.ID) ||
		!strings.HasPrefix(inspection.ID, row.ID) {
		return directCodingObservedService{}, 0, fmt.Errorf("container identity disagrees with Compose observation")
	}
	if inspection.Config == nil || inspection.HostConfig == nil ||
		inspection.State == nil || inspection.NetworkSettings == nil {
		return directCodingObservedService{}, 0, fmt.Errorf("Docker inspection omits required container authority")
	}
	labels := inspection.Config.Labels
	if labels["com.docker.compose.project"] != request.Project ||
		labels["com.docker.compose.service"] != row.Service {
		return directCodingObservedService{}, 0, fmt.Errorf("Compose project or service labels disagree")
	}
	if !directCodingImageIDPattern.MatchString(inspection.Image) ||
		labels["com.docker.compose.image"] != inspection.Image {
		return directCodingObservedService{}, 0, fmt.Errorf("container image is not one immutable Compose image identity")
	}
	if !inspection.State.Running || inspection.State.Status != "running" ||
		inspection.State.Health == nil || inspection.State.Health.Status != "healthy" {
		return directCodingObservedService{}, 0, fmt.Errorf("container is not running and healthy")
	}
	if inspection.HostConfig.RestartPolicy.Name != "unless-stopped" {
		return directCodingObservedService{}, 0, fmt.Errorf("container does not have the persistent restart policy")
	}
	port, err := validateDirectCodingObservedBindings(row, inspection.NetworkSettings.Ports, request)
	if err != nil {
		return directCodingObservedService{}, 0, err
	}
	return directCodingObservedService{
		Service: row.Service, ContainerID: inspection.ID, ImageID: inspection.Image,
		RestartPolicy: inspection.HostConfig.RestartPolicy.Name,
		State:         inspection.State.Status, Health: inspection.State.Health.Status,
	}, port, nil
}

var directCodingImageIDPattern = regexp.MustCompile(`^sha256:[a-f0-9]{64}$`)

func validateDirectCodingObservedBindings(
	row directCodingComposePSRow,
	ports map[string][]directCodingDockerPortBinding,
	request directCodingDeploymentObservationRequest,
) (uint16, error) {
	wantKey := strconv.Itoa(int(request.GatewayContainerPort)) + "/tcp"
	var observed uint16
	for key, bindings := range ports {
		if len(bindings) == 0 {
			continue
		}
		if row.Service != request.GatewayService || key != wantKey || len(bindings) != 1 {
			return 0, fmt.Errorf("service has an unregistered published port")
		}
		binding := bindings[0]
		parsed, err := strconv.Atoi(binding.HostPort)
		if err != nil || strconv.Itoa(parsed) != binding.HostPort {
			return 0, fmt.Errorf("gateway published port is not canonical")
		}
		observed, err = parseDirectCodingPublishedPort(parsed)
		if err != nil || binding.HostIP != request.BindAddress {
			return 0, fmt.Errorf("gateway published binding disagrees with server authority")
		}
	}
	return validateDirectCodingComposePublishers(row, request, observed)
}

func validateDirectCodingComposePublishers(
	row directCodingComposePSRow,
	request directCodingDeploymentObservationRequest,
	observed uint16,
) (uint16, error) {
	published := make([]directCodingComposePublisher, 0, 1)
	for _, candidate := range row.Publishers {
		if candidate.PublishedPort > 0 {
			published = append(published, candidate)
		}
	}
	if row.Service != request.GatewayService {
		if len(published) != 0 || observed != 0 {
			return 0, fmt.Errorf("non-gateway service exposes a published port")
		}
		return 0, nil
	}
	if len(published) != 1 || observed == 0 {
		return 0, fmt.Errorf("gateway requires one published port")
	}
	candidate := published[0]
	if candidate.URL != request.BindAddress || candidate.TargetPort != int(request.GatewayContainerPort) ||
		candidate.PublishedPort != int(observed) || candidate.Protocol != "tcp" {
		return 0, fmt.Errorf("gateway Compose publisher disagrees with Docker inspection")
	}
	return observed, nil
}
