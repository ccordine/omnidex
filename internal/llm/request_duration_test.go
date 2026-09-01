package llm

import (
	"testing"
	"time"
)

func TestBoundedModelRequestDurationEnforcesHardCeiling(t *testing.T) {
	for _, test := range []struct {
		name      string
		requested time.Duration
		want      time.Duration
	}{
		{name: "shorter", requested: 12 * time.Minute, want: 12 * time.Minute},
		{name: "ceiling", requested: 30 * time.Minute, want: 30 * time.Minute},
		{name: "over ceiling", requested: 31 * time.Minute, want: 30 * time.Minute},
		{name: "zero cannot disable timeout", requested: 0, want: 30 * time.Minute},
		{name: "negative cannot disable timeout", requested: -time.Second, want: 30 * time.Minute},
	} {
		t.Run(test.name, func(t *testing.T) {
			if got := BoundedModelRequestDuration(test.requested); got != test.want {
				t.Fatalf("BoundedModelRequestDuration(%s)=%s, want %s", test.requested, got, test.want)
			}
		})
	}
}
