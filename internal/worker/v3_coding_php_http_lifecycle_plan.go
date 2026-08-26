package worker

import (
	"fmt"
	"sort"
	"strings"

	"github.com/gryph/omnidex/internal/assemblyline"
)

type phpServiceHTTPLifecycle struct {
	Interface directCodingServiceStateInterfaceBinding
	Writer    phpServiceFeatureBinding
	Reader    phpServiceFeatureBinding
}

type phpServiceHTTPLifecyclePlan struct {
	Lifecycles []phpServiceHTTPLifecycle
	Blockers   []string
}

func derivePHPServiceHTTPLifecyclePlan(
	workload assemblyline.FrozenApplicationWorkload,
	capabilities directCodingCapabilityGraph,
	state directCodingServiceStatePlan,
	bindings []phpServiceFeatureBinding,
) (phpServiceHTTPLifecyclePlan, error) {
	if err := state.ValidateInterfacesFor(workload, capabilities); err != nil {
		return phpServiceHTTPLifecyclePlan{}, fmt.Errorf(
			"validate HTTP lifecycle state authority: %w", err,
		)
	}
	byTask := make(map[string]phpServiceFeatureBinding, len(bindings))
	for _, binding := range bindings {
		if _, duplicate := byTask[binding.TaskID]; duplicate {
			return phpServiceHTTPLifecyclePlan{}, fmt.Errorf(
				"HTTP lifecycle bindings repeat task %s", binding.TaskID,
			)
		}
		byTask[binding.TaskID] = binding
	}
	plan := phpServiceHTTPLifecyclePlan{}
	for _, stateInterface := range state.Interfaces {
		candidates := phpServiceHTTPLifecycleCandidates(
			stateInterface, state, capabilities, byTask,
		)
		if len(candidates) == 0 {
			plan.Blockers = append(plan.Blockers, fmt.Sprintf(
				"durable state interface %s has no mechanically verifiable cross-endpoint lifecycle: require a POST or PUT endpoint with a type-preserving state payload and a directly dependent parameter-free GET endpoint with observable response media",
				stateInterface.ID,
			))
			continue
		}
		sort.Slice(candidates, func(left, right int) bool {
			leftID := candidates[left].Writer.TaskID + "\x00" + candidates[left].Reader.TaskID
			rightID := candidates[right].Writer.TaskID + "\x00" + candidates[right].Reader.TaskID
			return leftID < rightID
		})
		plan.Lifecycles = append(plan.Lifecycles, candidates[0])
	}
	return plan, nil
}

func phpServiceHTTPLifecycleCandidates(
	stateInterface directCodingServiceStateInterfaceBinding,
	state directCodingServiceStatePlan,
	capabilities directCodingCapabilityGraph,
	byTask map[string]phpServiceFeatureBinding,
) []phpServiceHTTPLifecycle {
	if !phpServiceStateInterfaceHasObservableSentinel(stateInterface.Result) {
		return nil
	}
	candidates := make([]phpServiceHTTPLifecycle, 0)
	for _, writerTaskID := range stateInterface.TaskIDs {
		writer, exists := byTask[writerTaskID]
		if !exists || !phpServiceHTTPStateWriter(writer, state, stateInterface.Result) {
			continue
		}
		for _, readerTaskID := range stateInterface.TaskIDs {
			reader, exists := byTask[readerTaskID]
			if !exists || !phpServiceHTTPStateReader(reader) ||
				!phpServiceDirectCapability(capabilities[reader.RequirementID], writer.RequirementID) {
				continue
			}
			candidates = append(candidates, phpServiceHTTPLifecycle{
				Interface: stateInterface, Writer: writer, Reader: reader,
			})
		}
	}
	return candidates
}

func phpServiceHTTPStateWriter(
	binding phpServiceFeatureBinding,
	state directCodingServiceStatePlan,
	stateShape assemblyline.ApplicationServiceStateInterfaceResult,
) bool {
	if !binding.HasEndpoint || state.ByTask[binding.TaskID] !=
		assemblyline.ApplicationServiceStateCrossRequestAuthorityRequired {
		return false
	}
	if binding.Endpoint.Method != assemblyline.ApplicationServiceEndpointPOST &&
		binding.Endpoint.Method != assemblyline.ApplicationServiceEndpointPUT {
		return false
	}
	return phpServiceHTTPStatePayloadPreservesKinds(binding.Endpoint.RequestMedia, stateShape)
}

func phpServiceHTTPStateReader(binding phpServiceFeatureBinding) bool {
	return binding.HasEndpoint && binding.Endpoint.Method == assemblyline.ApplicationServiceEndpointGET &&
		binding.Endpoint.RequestMedia == assemblyline.ApplicationServiceEndpointMediaNone &&
		binding.Endpoint.ResponseMedia != assemblyline.ApplicationServiceEndpointMediaNone &&
		len(phpServiceRouteParameters(binding.Endpoint.RouteTemplate)) == 0
}

func phpServiceHTTPStatePayloadPreservesKinds(
	media assemblyline.ApplicationServiceEndpointMedia,
	stateShape assemblyline.ApplicationServiceStateInterfaceResult,
) bool {
	if media == assemblyline.ApplicationServiceEndpointJSON {
		return true
	}
	if media != assemblyline.ApplicationServiceEndpointForm {
		return false
	}
	for _, field := range stateShape.Fields {
		if field.Kind == assemblyline.ApplicationServiceStateString ||
			field.Kind == assemblyline.ApplicationServiceStateStringList {
			continue
		}
		if field.Kind != assemblyline.ApplicationServiceStateRecordList {
			return false
		}
		for _, nested := range field.RecordFields {
			if nested.Kind != assemblyline.ApplicationServiceStateString {
				return false
			}
		}
	}
	return true
}

func phpServiceStateInterfaceHasObservableSentinel(
	stateShape assemblyline.ApplicationServiceStateInterfaceResult,
) bool {
	for _, field := range stateShape.Fields {
		switch field.Kind {
		case assemblyline.ApplicationServiceStateString,
			assemblyline.ApplicationServiceStateInteger,
			assemblyline.ApplicationServiceStateNumber,
			assemblyline.ApplicationServiceStateStringList,
			assemblyline.ApplicationServiceStateIntegerList,
			assemblyline.ApplicationServiceStateNumberList:
			return true
		case assemblyline.ApplicationServiceStateRecordList:
			for _, nested := range field.RecordFields {
				if nested.Kind != assemblyline.ApplicationServiceStateBoolean {
					return true
				}
			}
		}
	}
	return false
}

func phpServiceDirectCapability(
	capabilities []directCodingCapabilityBinding,
	providerRequirementID string,
) bool {
	for _, capability := range capabilities {
		if capability.RequirementID == providerRequirementID {
			return true
		}
	}
	return false
}

func phpServiceHTTPLifecycleBlockerSource(blockers []string) string {
	if len(blockers) == 0 {
		return ""
	}
	return "throw new RuntimeException(" + phpSingleQuoted(strings.Join(blockers, "; ")) + ");\n"
}
