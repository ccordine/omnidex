package cognitiongauntlet

import (
	"fmt"
	"math"
	"sort"

	"github.com/gryph/omnidex/internal/queue"
)

func semanticWorkingSetProjectionPoints(
	records []queue.CognitionSealedTraceRecord,
) ([]queue.CognitionProjectionWorkingVersion, error) {
	maximums := make(map[int64]int64)
	for _, record := range records {
		if record.Kind != "runtime_snapshot" {
			continue
		}
		var value semanticRuntimeSnapshot
		if err := decodeProductionPayload(
			record.Payload, &value, "Working Set projection chronology",
		); err != nil {
			return nil, err
		}
		if record.CallOrdinal < 1 || value.ContextProjection.WorkingSetVersion > math.MaxInt64 {
			return nil, fmt.Errorf("invalid Working Set projection chronology")
		}
		version := int64(value.ContextProjection.WorkingSetVersion)
		if prior, exists := maximums[record.CallOrdinal]; !exists || version > prior {
			maximums[record.CallOrdinal] = version
		}
	}
	ordinals := make([]int64, 0, len(maximums))
	for ordinal := range maximums {
		ordinals = append(ordinals, ordinal)
	}
	sort.Slice(ordinals, func(i, j int) bool { return ordinals[i] < ordinals[j] })
	points := make([]queue.CognitionProjectionWorkingVersion, 0, len(ordinals))
	for _, ordinal := range ordinals {
		points = append(points, queue.CognitionProjectionWorkingVersion{
			CallOrdinal: ordinal, WorkingVersion: maximums[ordinal],
		})
	}
	probe := queue.CognitionSealedTraceRecord{Kind: "working_set_snapshot", Phase: 1}
	if err := queue.VerifyCognitionWorkingSetTraceTuple(
		probe, queue.CognitionWorkingSetTraceEpisodeStart, 0, points,
	); err != nil {
		return nil, err
	}
	return points, nil
}

func (state *semanticReplayState) verifyWorkingSetTuple(
	record queue.CognitionSealedTraceRecord,
	point queue.CognitionWorkingSetTracePoint,
	version uint64,
) error {
	if state.workingSetTupleErr != nil {
		return state.workingSetTupleErr
	}
	return queue.VerifyCognitionWorkingSetTraceTuple(
		record, point, version, state.workingSetProjectionPoints,
	)
}
