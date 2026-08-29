package worker

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
)

const directCodingComposePSRowLimit = 128 << 10

type directCodingComposePSDocument struct {
	Command      string                         `json:"Command"`
	CreatedAt    string                         `json:"CreatedAt"`
	ExitCode     int                            `json:"ExitCode"`
	Health       string                         `json:"Health"`
	ID           string                         `json:"ID"`
	Image        string                         `json:"Image"`
	Labels       string                         `json:"Labels"`
	LocalVolumes string                         `json:"LocalVolumes"`
	Mounts       string                         `json:"Mounts"`
	Name         string                         `json:"Name"`
	Names        string                         `json:"Names"`
	Networks     string                         `json:"Networks"`
	Ports        string                         `json:"Ports"`
	Project      string                         `json:"Project"`
	Publishers   []directCodingComposePublisher `json:"Publishers"`
	RunningFor   string                         `json:"RunningFor"`
	Service      string                         `json:"Service"`
	Size         string                         `json:"Size"`
	State        string                         `json:"State"`
	Status       string                         `json:"Status"`
}

func decodeDirectCodingComposePSRow(line []byte) (directCodingComposePSRow, error) {
	if len(line) == 0 || len(line) > directCodingComposePSRowLimit ||
		!bytes.Equal(line, bytes.TrimSpace(line)) {
		return directCodingComposePSRow{}, fmt.Errorf("row is empty, padded, or exceeds its byte limit")
	}
	decoder := json.NewDecoder(bytes.NewReader(line))
	decoder.DisallowUnknownFields()
	var document directCodingComposePSDocument
	if err := decoder.Decode(&document); err != nil {
		return directCodingComposePSRow{}, err
	}
	if err := requireDirectCodingJSONEOF(decoder); err != nil {
		return directCodingComposePSRow{}, err
	}
	if !directCodingContainerIDPattern.MatchString(document.ID) {
		return directCodingComposePSRow{}, fmt.Errorf("container identity is invalid")
	}
	if !generatedDeploymentServicePattern.MatchString(document.Service) {
		return directCodingComposePSRow{}, fmt.Errorf("service identity is invalid")
	}
	for index, publisher := range document.Publishers {
		if publisher.TargetPort < 0 || publisher.TargetPort > 65535 ||
			publisher.PublishedPort < 0 || publisher.PublishedPort > 65535 ||
			(publisher.Protocol != "tcp" && publisher.Protocol != "udp") {
			return directCodingComposePSRow{}, fmt.Errorf("publisher %d is invalid", index)
		}
		if publisher.PublishedPort == 0 && publisher.URL != "" {
			return directCodingComposePSRow{}, fmt.Errorf("publisher %d has a host without a published port", index)
		}
	}
	return directCodingComposePSRow{
		ID: document.ID, Project: document.Project, Service: document.Service,
		State: document.State, Health: document.Health, Publishers: document.Publishers,
	}, nil
}

func requireDirectCodingJSONEOF(decoder *json.Decoder) error {
	var trailing json.RawMessage
	err := decoder.Decode(&trailing)
	if err == io.EOF {
		return nil
	}
	if err != nil {
		return fmt.Errorf("decode trailing JSON: %w", err)
	}
	return fmt.Errorf("multiple JSON values are forbidden")
}
