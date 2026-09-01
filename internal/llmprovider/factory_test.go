package llmprovider

import (
	"strings"
	"testing"
	"time"

	"github.com/gryph/omnidex/internal/config"
	"github.com/gryph/omnidex/internal/llmprovider/catalog"
)

func TestProviderRequestTimeoutEnforcesThirtyMinuteMaximum(t *testing.T) {
	definition := catalog.Definition{ID: "test-provider"}
	for _, test := range []struct {
		name    string
		value   string
		want    time.Duration
		wantErr string
	}{
		{name: "shorter", value: "12m", want: 12 * time.Minute},
		{name: "maximum", value: "30m", want: 30 * time.Minute},
		{name: "over maximum", value: "30m1s", wantErr: "must not exceed 30m0s"},
		{name: "zero", value: "0s", wantErr: "must be positive"},
		{name: "invalid", value: "eventually", wantErr: "must be a duration"},
	} {
		t.Run(test.name, func(t *testing.T) {
			got, err := providerRequestTimeout(config.Config{RequestTimeout: test.value}, definition)
			if test.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), test.wantErr) {
					t.Fatalf("error=%v, want substring %q", err, test.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if got != test.want {
				t.Fatalf("timeout=%s, want %s", got, test.want)
			}
		})
	}
}
