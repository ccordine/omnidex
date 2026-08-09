package worker

import (
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/assemblyline"
)

func TestOrdinaryTransportRejectsEveryAdvisoryWorkKind(t *testing.T) {
	t.Parallel()

	for _, kind := range []assemblyline.WorkKind{
		assemblyline.WorkRequirementAdvisory,
		assemblyline.WorkRequirementFinalAdvisory,
		assemblyline.WorkRetrievalAdvisory,
	} {
		if err := rejectOrdinaryAdvisoryJob(kind); err == nil || !strings.Contains(err.Error(), "native advisory transport") {
			t.Fatalf("kind=%q error=%v", kind, err)
		}
	}
	if err := rejectOrdinaryAdvisoryJob(assemblyline.WorkRequirementBriefing); err != nil {
		t.Fatalf("structured work rejected: %v", err)
	}
}

func TestProductionAdvisoryTransportAcceptsOnlyApprovedRequirementStation(t *testing.T) {
	t.Parallel()

	if err := validateProductionAdvisoryJob(assemblyline.WorkRequirementAdvisory); err != nil {
		t.Fatalf("requirement advisory rejected: %v", err)
	}
	if err := validateProductionAdvisoryJob(assemblyline.WorkRetrievalAdvisory); err == nil || !strings.Contains(err.Error(), "rejects work kind") {
		t.Fatalf("retrieval advisory error=%v", err)
	}
	if err := validateProductionAdvisoryJob(assemblyline.WorkRequirementFinalAdvisory); err == nil || !strings.Contains(err.Error(), "rejects work kind") {
		t.Fatalf("experimental final advisory error=%v", err)
	}
}
