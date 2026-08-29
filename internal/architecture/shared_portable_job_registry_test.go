package architecture

import (
	"testing"

	"github.com/gryph/omnidex/internal/assemblyline"
)

func TestSharedPortableJobRegistryContainsOnlyProductionRoutableKinds(t *testing.T) {
	for _, kind := range assemblyline.AllWorkKinds() {
		switch kind {
		case "requirement_partition_briefing", "requirement_partition_advisory",
			"requirement_partition_synthesis", "requirement_final_advisory",
			"requirement_final_synthesis":
			t.Fatalf("offline gauntlet work kind leaked into shared production registry: %s", kind)
		}
	}
}
