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
		) (webresearch.SemanticCallReceipt, error) {
			if runtime == nil || runtime.svc == nil {
				return webresearch.SemanticCallReceipt{}, fmt.Errorf("web station %q requires runtime authority", id)
			}
			if job.Kind != assemblyline.WorkWebRelevanceRelation {
				return webresearch.SemanticCallReceipt{}, fmt.Errorf(
					"web station %q received unsupported work kind %q", id, job.Kind,
				)
			}
			if validate == nil {
				return webresearch.SemanticCallReceipt{}, fmt.Errorf("web station %q requires one exact decoder", id)
			}
			_, receipt, err := runObjectivePortableRawLeafStation(
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
				func(string) error { return nil },
			)
			return webresearch.SemanticCallReceipt{
				Calls: receipt.Calls, Reused: receipt.Reused,
			}, err
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

func cloneWebParagraphs(items []webresearch.GroundedParagraph) []webresearch.GroundedParagraph {
	cloned := make([]webresearch.GroundedParagraph, len(items))
	for index, item := range items {
		cloned[index] = item
		cloned[index].EvidenceIDs = append([]webresearch.EvidenceID(nil), item.EvidenceIDs...)
	}
	return cloned
}
