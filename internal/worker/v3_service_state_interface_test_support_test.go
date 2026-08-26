package worker

import (
	"testing"

	"github.com/gryph/omnidex/internal/assemblyline"
)

func bindTestServiceStateInterfaces(
	t *testing.T,
	workload assemblyline.FrozenApplicationWorkload,
	capabilities directCodingCapabilityGraph,
	plan directCodingServiceStatePlan,
	fields []assemblyline.ApplicationServiceStateField,
) directCodingServiceStatePlan {
	t.Helper()
	components, err := directCodingServiceStateComponents(workload, capabilities, plan)
	if err != nil {
		t.Fatal(err)
	}
	plan.Interfaces = make([]directCodingServiceStateInterfaceBinding, 0, len(components))
	plan.InterfaceByTask = make(map[string]string)
	for _, component := range components {
		result := assemblyline.ApplicationServiceStateInterfaceResult{
			Schema: assemblyline.ApplicationServiceStateInterfaceSchemaV1,
			Fields: append([]assemblyline.ApplicationServiceStateField(nil), fields...),
		}
		if err := result.ValidateFor(component.Input); err != nil {
			t.Fatal(err)
		}
		plan.Interfaces = append(plan.Interfaces, directCodingServiceStateInterfaceBinding{
			ID: component.ID, TaskIDs: append([]string(nil), component.TaskIDs...),
			Input: component.Input, Result: result,
		})
		for _, taskID := range component.TaskIDs {
			plan.InterfaceByTask[taskID] = component.ID
		}
	}
	return plan
}

func testIntegerServiceStateField(name string) []assemblyline.ApplicationServiceStateField {
	return []assemblyline.ApplicationServiceStateField{{
		Name: name, Kind: assemblyline.ApplicationServiceStateInteger,
		RecordFields: []assemblyline.ApplicationServiceStateRecordField{},
	}}
}
