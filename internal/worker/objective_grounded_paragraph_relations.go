package worker

import (
	"context"

	"github.com/gryph/omnidex/internal/assemblyline"
	"github.com/gryph/omnidex/internal/station"
)

func runGroundedParagraphRelevance(
	ctx context.Context, runtime *nativeRuntimeV3, stationID station.ID,
	resolveModel func() (string, error), input assemblyline.GroundedParagraphRelevanceInput,
) (assemblyline.GroundedParagraphRelevance, int, error) {
	job, err := assemblyline.NewGroundedParagraphRelevanceJob(input)
	if err != nil {
		return "", 0, err
	}
	return runObjectivePortableRawLeafStation(
		ctx, runtime, "grounded_paragraph_relevance", job, stationID, resolveModel,
		func(raw string) (assemblyline.GroundedParagraphRelevance, error) {
			return assemblyline.DecodeGroundedParagraphRelevance(input, raw)
		},
	)
}

func runGroundedParagraphSupport(
	ctx context.Context, runtime *nativeRuntimeV3, stationID station.ID,
	resolveModel func() (string, error), input assemblyline.GroundedParagraphSupportInput,
) (assemblyline.GroundedParagraphSupport, int, error) {
	job, err := assemblyline.NewGroundedParagraphSupportJob(input)
	if err != nil {
		return "", 0, err
	}
	return runObjectivePortableRawLeafStation(
		ctx, runtime, "grounded_paragraph_support", job, stationID, resolveModel,
		func(raw string) (assemblyline.GroundedParagraphSupport, error) {
			return assemblyline.DecodeGroundedParagraphSupport(input, raw)
		},
	)
}
