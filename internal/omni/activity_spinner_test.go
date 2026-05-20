package omni

import (
	"testing"
	"time"
)

func TestFormatActivityElapsed(t *testing.T) {
	cases := []struct {
		elapsed time.Duration
		want    string
	}{
		{elapsed: 0, want: "0s"},
		{elapsed: 1500 * time.Millisecond, want: "2s"},
		{elapsed: 65 * time.Second, want: "1m05s"},
	}
	for _, tc := range cases {
		if got := formatActivityElapsed(tc.elapsed); got != tc.want {
			t.Fatalf("formatActivityElapsed(%s) = %q, want %q", tc.elapsed, got, tc.want)
		}
	}
}
