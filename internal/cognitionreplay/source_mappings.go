package cognitionreplay

import (
	"fmt"
	"reflect"
	"sort"
)

func deriveStructuralMappings(
	sources []SourceRecord,
	events []Event,
) ([]SourceMapping, error) {
	return deriveMappings(sources, events, StructuralMappingSchemaV1,
		func(string) (MappingDisposition, error) { return MappingStructuralOpaque, nil })
}

func deriveMappings(
	sources []SourceRecord,
	events []Event,
	mappingSchema string,
	disposition func(string) (MappingDisposition, error),
) ([]SourceMapping, error) {
	kindByOrdinal := make(map[uint64]string, len(sources))
	eventKinds := make(map[string]map[EventKind]struct{})
	for _, source := range sources {
		kindByOrdinal[source.Ordinal] = source.Kind
		if _, exists := eventKinds[source.Kind]; !exists {
			eventKinds[source.Kind] = make(map[EventKind]struct{})
		}
	}
	for _, event := range events {
		for _, ref := range event.Sources {
			kind, exists := kindByOrdinal[ref.Ordinal]
			if !exists || kind != ref.Kind {
				return nil, fmt.Errorf("replay mapping cites an unknown source kind")
			}
			eventKinds[kind][event.Kind] = struct{}{}
		}
	}
	kinds := make([]string, 0, len(eventKinds))
	for kind := range eventKinds {
		kinds = append(kinds, kind)
	}
	sort.Strings(kinds)
	result := make([]SourceMapping, len(kinds))
	for index, kind := range kinds {
		mapped := eventKinds[kind]
		if len(mapped) == 0 {
			return nil, fmt.Errorf("replay source kind %q has no derived event", kind)
		}
		mappedKinds := make([]EventKind, 0, len(mapped))
		for eventKind := range mapped {
			mappedKinds = append(mappedKinds, eventKind)
		}
		sort.Slice(mappedKinds, func(left, right int) bool { return mappedKinds[left] < mappedKinds[right] })
		mappingDisposition, err := disposition(kind)
		if err != nil {
			return nil, err
		}
		result[index] = SourceMapping{SourceKind: kind, MappingSchema: mappingSchema,
			Disposition: mappingDisposition, EventKinds: mappedKinds}
	}
	return result, nil
}

func equalSourceMappings(left, right []SourceMapping) bool {
	return reflect.DeepEqual(left, right)
}
