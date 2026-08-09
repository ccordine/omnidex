package worker

import (
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/assemblyline"
)

func TestProductionTransportRejectsEveryOfflineAdvisoryProtocolWorkKind(t *testing.T) {
	t.Parallel()

	for _, kind := range []assemblyline.WorkKind{
		assemblyline.WorkRequirementBriefing,
		assemblyline.WorkRequirementAdvisory,
		assemblyline.WorkRequirementSynthesis,
		assemblyline.WorkRequirementFinalAdvisory,
		assemblyline.WorkRequirementFinalSynthesis,
		assemblyline.WorkRetrievalBriefing,
		assemblyline.WorkRetrievalAdvisory,
		assemblyline.WorkRetrievalSynthesis,
	} {
		if err := rejectOfflineExperimentJob(kind); err == nil || !strings.Contains(err.Error(), "offline advisory experiment") {
			t.Fatalf("kind=%q error=%v", kind, err)
		}
	}
	for _, kind := range []assemblyline.WorkKind{
		assemblyline.WorkRequirementPartition,
		assemblyline.WorkRepositoryRetrieval,
	} {
		if err := rejectOfflineExperimentJob(kind); err != nil {
			t.Fatalf("production work %q rejected: %v", kind, err)
		}
	}
}
