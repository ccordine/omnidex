package worker

import (
	"testing"

	"github.com/gryph/omnidex/internal/assemblyline"
	"github.com/gryph/omnidex/internal/queue"
)

func portableWorkerTestMaxOutputTokens(
	t testing.TB,
	job assemblyline.PortableJob,
	contextTokens int,
) int {
	t.Helper()
	maximum, err := queue.ExpectedPortableStationMaxOutputTokens(job, contextTokens)
	if err != nil {
		t.Fatal(err)
	}
	return maximum
}
