package worker

import (
	"context"
	"fmt"

	"github.com/gryph/omnidex/internal/assemblyline"
	"github.com/gryph/omnidex/internal/station"
	"github.com/gryph/omnidex/internal/webresearch"
)

type routedWebEvidenceStations struct {
	relevance *webresearch.PortableStations
}

const maxObjectiveWebRelevantCandidates = 2

func newRoutedWebEvidenceStations(
	runtimeFor func(station.ID) webresearch.PortableRuntime,
) (routedWebEvidenceStations, error) {
	if runtimeFor == nil {
		return routedWebEvidenceStations{}, fmt.Errorf("web evidence portable runtime is unavailable")
	}
	relevance, err := webresearch.NewPortableStations(runtimeFor(station.WebRelevance))
	if err != nil {
		return routedWebEvidenceStations{}, err
	}
	return routedWebEvidenceStations{relevance: relevance}, nil
}

func runtimeWebPortableRuntime(
	runtime *nativeRuntimeV3,
	id station.ID,
) webresearch.PortableRuntime {
	return webresearch.PortableRuntime{
		Resolve: func(
			ctx context.Context,
			job assemblyline.PortableJob,
			validate webresearch.PortableCandidateValidator,
		) (int, error) {
			if runtime == nil || runtime.svc == nil {
				return 0, fmt.Errorf("web station %q requires runtime authority", id)
			}
			if job.Kind != assemblyline.WorkWebRelevanceRelation {
				return 0, fmt.Errorf(
					"web station %q received unsupported work kind %q", id, job.Kind,
				)
			}
			if validate == nil {
				return 0, fmt.Errorf("web station %q requires one exact decoder", id)
			}
			_, dispatches, err := runObjectivePortableRawLeafStation(
				ctx,
				runtime,
				"web_"+string(job.Kind),
				job,
				id,
				func() (string, error) {
					return objectiveStationModel(runtime, id)
				},
				func(raw string) (string, error) {
					if err := validate(raw); err != nil {
						return "", err
					}
					return raw, nil
				},
			)
			return dispatches, err
		},
	}
}

func objectiveWebEvidenceConfig() webresearch.EvidenceConfig {
	return webresearch.EvidenceConfig{
		MaxFetchCandidates: maxObjectiveWebRelevantCandidates,
		MaxProjectionBytes: 8 * 1024, MaxRelevantCandidates: maxObjectiveWebRelevantCandidates,
		CandidateSummaryBytes: 512,
	}
}
