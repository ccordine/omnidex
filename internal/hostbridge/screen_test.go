package hostbridge

import (
	"net/http/httptest"
	"testing"
)

func TestScreenMonitorPageRequestRejectsInvalidBounds(t *testing.T) {
	for _, raw := range []string{
		"?limit=0", "?limit=101", "?limit=01", "?offset=-1", "?offset=1.0", "?offset=1&offset=2",
	} {
		r := httptest.NewRequest("GET", "/v1/screen/monitors"+raw, nil)
		if _, err := screenMonitorPageRequest(r); err == nil {
			t.Fatalf("expected %q to fail", raw)
		}
	}
}

func TestCollectScreenMonitorPageRetainsOnlyLimitPlusOne(t *testing.T) {
	monitors := []ScreenMonitor{
		{ID: "one", Name: "one"}, {ID: "two", Name: "two"}, {ID: "three", Name: "three"},
		{ID: "four", Name: "four"}, {ID: "five", Name: "five"},
	}
	visited := 0
	page, found, err := collectScreenMonitorPage(ScreenMonitorPageRequest{Limit: 2, Offset: 2}, "test", func(visit func(ScreenMonitor) bool) (bool, int, error) {
		for index, monitor := range monitors {
			visited++
			if !visit(monitor) {
				return false, index + 1, nil
			}
		}
		return true, len(monitors), nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if !found || visited != 5 || len(page.Monitors) != 2 || page.Monitors[0].ID != "three" || !page.HasMore || page.NextOffset != 4 || !page.HasPrevious || page.PreviousOffset != 0 {
		t.Fatalf("page=%+v found=%t visited=%d", page, found, visited)
	}
}

func TestParseScreenInt(t *testing.T) {
	if parseScreenInt("15", 12) != 15 {
		t.Fatal("expected 15")
	}
	if parseScreenInt("", 12) != 12 {
		t.Fatal("expected fallback")
	}
}
