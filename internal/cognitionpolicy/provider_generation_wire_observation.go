package cognitionpolicy

import (
	"fmt"
	"time"

	"github.com/gryph/omnidex/internal/llm"
)

type providerIdentityObservationWire struct {
	Schema                 providerGenerationWireBytes `json:"schema"`
	ObservedYear           int                         `json:"observed_year"`
	ObservedMonth          int                         `json:"observed_month"`
	ObservedDay            int                         `json:"observed_day"`
	ObservedHour           int                         `json:"observed_hour"`
	ObservedMinute         int                         `json:"observed_minute"`
	ObservedSecond         int                         `json:"observed_second"`
	ObservedNanosecond     int                         `json:"observed_nanosecond"`
	ObservedAt             providerGenerationWireBytes `json:"observed_at"`
	ObservedLocation       providerGenerationWireBytes `json:"observed_location"`
	ObservedOffsetSeconds  int                         `json:"observed_offset_seconds"`
	AttestationSHA256      providerGenerationWireBytes `json:"attestation_sha256"`
	VersionBodySHA256      providerGenerationWireBytes `json:"version_body_sha256"`
	InstalledBodySHA256    providerGenerationWireBytes `json:"installed_body_sha256"`
	TokenizerRequestSHA256 providerGenerationWireBytes `json:"tokenizer_request_sha256"`
	TokenizerBodySHA256    providerGenerationWireBytes `json:"tokenizer_body_sha256"`
	PreloadBodySHA256      providerGenerationWireBytes `json:"preload_body_sha256"`
	RunnerBodySHA256       providerGenerationWireBytes `json:"runner_body_sha256"`
	PreloadMethod          providerGenerationWireBytes `json:"preload_method"`
	PreloadEndpoint        providerGenerationWireBytes `json:"preload_endpoint"`
	PreloadRequestSHA256   providerGenerationWireBytes `json:"preload_request_sha256"`
	ChallengeSHA256        providerGenerationWireBytes `json:"challenge_sha256"`
	EvidenceSchema         providerGenerationWireBytes `json:"evidence_schema"`
	EvidenceID             providerGenerationWireBytes `json:"evidence_id"`
	EvidenceSHA256         providerGenerationWireBytes `json:"evidence_sha256"`
	EvidenceBytes          int                         `json:"evidence_bytes"`
	ObservationSHA256      providerGenerationWireBytes `json:"observation_sha256"`
}

func encodeProviderObservationWire(
	observation llm.ProviderIdentityObservation,
	field func(string) providerGenerationWireBytes,
) providerIdentityObservationWire {
	_, offset := observation.ObservedAt.Zone()
	return providerIdentityObservationWire{
		Schema: field(observation.Schema), ObservedYear: observation.ObservedAt.Year(),
		ObservedMonth: int(observation.ObservedAt.Month()), ObservedDay: observation.ObservedAt.Day(),
		ObservedHour: observation.ObservedAt.Hour(), ObservedMinute: observation.ObservedAt.Minute(),
		ObservedSecond: observation.ObservedAt.Second(), ObservedNanosecond: observation.ObservedAt.Nanosecond(),
		ObservedAt:       field(observation.ObservedAt.Format(time.RFC3339Nano)),
		ObservedLocation: field(observation.ObservedAt.Location().String()), ObservedOffsetSeconds: offset,
		AttestationSHA256:      field(observation.AttestationSHA256),
		VersionBodySHA256:      field(observation.VersionBodySHA256),
		InstalledBodySHA256:    field(observation.InstalledBodySHA256),
		TokenizerRequestSHA256: field(observation.TokenizerRequestSHA256),
		TokenizerBodySHA256:    field(observation.TokenizerBodySHA256),
		PreloadBodySHA256:      field(observation.PreloadBodySHA256),
		RunnerBodySHA256:       field(observation.RunnerBodySHA256),
		PreloadMethod:          field(observation.PreloadMethod), PreloadEndpoint: field(observation.PreloadEndpoint),
		PreloadRequestSHA256: field(observation.PreloadRequestSHA256),
		ChallengeSHA256:      field(observation.ChallengeSHA256),
		EvidenceSchema:       field(observation.Evidence.Schema), EvidenceID: field(observation.Evidence.ID),
		EvidenceSHA256: field(observation.Evidence.SHA256), EvidenceBytes: observation.Evidence.Bytes,
		ObservationSHA256: field(observation.ObservationSHA256),
	}
}

func decodeProviderObservationWire(
	wire providerIdentityObservationWire,
) (llm.ProviderIdentityObservation, bool, error) {
	fields := []providerGenerationWireBytes{
		wire.Schema, wire.ObservedAt, wire.ObservedLocation, wire.AttestationSHA256,
		wire.VersionBodySHA256, wire.InstalledBodySHA256, wire.TokenizerRequestSHA256,
		wire.TokenizerBodySHA256, wire.PreloadBodySHA256, wire.RunnerBodySHA256,
		wire.PreloadMethod, wire.PreloadEndpoint, wire.PreloadRequestSHA256,
		wire.ChallengeSHA256, wire.EvidenceSchema, wire.EvidenceID,
		wire.EvidenceSHA256, wire.ObservationSHA256,
	}
	values := make([]string, len(fields))
	complete := true
	timeFieldsComplete := true
	for index, field := range fields {
		raw, exact, err := field.exact(maxProviderGenerationMetadataCaptureBytes)
		if err != nil {
			return llm.ProviderIdentityObservation{}, false, err
		}
		complete = complete && exact
		if index == 1 || index == 2 {
			timeFieldsComplete = timeFieldsComplete && exact
		}
		if exact {
			values[index] = string(raw)
		}
	}
	var observedAt time.Time
	if timeFieldsComplete {
		location := time.FixedZone(values[2], wire.ObservedOffsetSeconds)
		if values[2] == "UTC" && wire.ObservedOffsetSeconds == 0 {
			location = time.UTC
		}
		observedAt = time.Date(
			wire.ObservedYear, time.Month(wire.ObservedMonth), wire.ObservedDay,
			wire.ObservedHour, wire.ObservedMinute, wire.ObservedSecond,
			wire.ObservedNanosecond, location,
		)
		if values[1] != observedAt.Format(time.RFC3339Nano) {
			return llm.ProviderIdentityObservation{}, false,
				fmt.Errorf("provider observation time wire changed")
		}
	}
	if !complete {
		return llm.ProviderIdentityObservation{}, false, nil
	}
	return llm.ProviderIdentityObservation{
		Schema: values[0], ObservedAt: observedAt, AttestationSHA256: values[3],
		VersionBodySHA256: values[4], InstalledBodySHA256: values[5],
		TokenizerRequestSHA256: values[6], TokenizerBodySHA256: values[7],
		PreloadBodySHA256: values[8], RunnerBodySHA256: values[9],
		PreloadMethod: values[10], PreloadEndpoint: values[11],
		PreloadRequestSHA256: values[12], ChallengeSHA256: values[13],
		Evidence: llm.ProviderIdentityEvidenceRef{
			Schema: values[14], ID: values[15], SHA256: values[16], Bytes: wire.EvidenceBytes,
		},
		ObservationSHA256: values[17],
	}, true, nil
}
