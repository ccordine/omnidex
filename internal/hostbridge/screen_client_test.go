package hostbridge

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestClientScreenMonitorsBindsExactPage(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("limit") != "2" || r.URL.Query().Get("offset") != "4" {
			t.Fatalf("query=%s", r.URL.RawQuery)
		}
		_ = json.NewEncoder(w).Encode(ScreenMonitorPage{
			Monitors: []ScreenMonitor{{ID: "DP-1", Name: "DP-1", Width: 1920, Height: 1080}},
			Backend:  "x11", StreamPath: "/v1/screen/mjpeg", Limit: 2, Offset: 4,
			HasPrevious: true, PreviousOffset: 2,
		})
	}))
	t.Cleanup(server.Close)
	page, err := NewClient(server.URL, "", 0).ScreenMonitors(context.Background(), ScreenMonitorPageRequest{Limit: 2, Offset: 4})
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Monitors) != 1 || page.Monitors[0].ID != "DP-1" || !page.HasPrevious {
		t.Fatalf("page=%+v", page)
	}
}
