package objectiveworkload_test

import (
	"context"
	"testing"

	"github.com/gryph/omnidex/internal/cognitionreference/objectiveworkload"
)

func compileOne(t *testing.T) objectiveworkload.Workload {
	t.Helper()
	authority := "Build a dashboard."
	station := &scriptedPartitionStation{steps: newPartitionScript(authority, "dashboard")}
	result, err := objectiveworkload.Compile(
		context.Background(), authority, station,
		objectiveworkload.CompileLimits{MaxStationCalls: 8},
	)
	if err != nil {
		t.Fatal(err)
	}
	return result.Workload
}
