package api

import (
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/hostbridge"
)

func TestScreenMonitorPageRequestUsesCanonicalBounds(t *testing.T) {
	request := httptest.NewRequest("GET", "/v1/host/screen/monitors?limit=7&offset=14", nil)
	page, err := screenMonitorPageRequest(request, screenMonitorUIPageSize)
	if err != nil {
		t.Fatal(err)
	}
	if page.Limit != 7 || page.Offset != 14 {
		t.Fatalf("page=%+v", page)
	}
	for _, raw := range []string{"?limit=0", "?limit=01", "?offset=-1", "?offset=2&offset=3"} {
		request := httptest.NewRequest("GET", "/v1/host/screen/monitors"+raw, nil)
		if _, err := screenMonitorPageRequest(request, screenMonitorUIPageSize); err == nil {
			t.Fatalf("expected %q to fail", raw)
		}
	}
}

func TestRenderUIScreenMonitorPaginationUsesServerOffsets(t *testing.T) {
	page := hostbridge.ScreenMonitorPage{
		Limit: 10, Offset: 10, HasPrevious: true, PreviousOffset: 0,
		HasMore: true, NextOffset: 20,
	}
	html, err := renderUIScreenMonitorPagination(page)
	if err != nil {
		t.Fatal(err)
	}
	for _, expected := range []string{
		`data-action="screen#loadMonitorPage"`, `data-page-offset="0"`, `Previous monitors`,
		`data-page-offset="20"`, `Next monitors`,
	} {
		if !strings.Contains(html, expected) {
			t.Fatalf("pagination lacks %q: %s", expected, html)
		}
	}
}

func TestRenderUIScreenMonitorOptionsUsesTypedPageOnly(t *testing.T) {
	html, selected, err := renderUIScreenMonitorOptions([]hostbridge.ScreenMonitor{
		{ID: "DP-1", Name: "Desk", Width: 1920, Height: 1080},
		{ID: "HDMI-1", Name: "Main", Width: 2560, Height: 1440, Primary: true},
	})
	if err != nil {
		t.Fatal(err)
	}
	if selected != "HDMI-1" || !strings.Contains(html, `Main (2560×1440) · primary`) {
		t.Fatalf("selected=%q html=%s", selected, html)
	}
}

func TestRenderUIScreenMonitorPaginationRejectsContradictoryPage(t *testing.T) {
	if _, err := renderUIScreenMonitorPagination(hostbridge.ScreenMonitorPage{
		Offset: 10, HasPrevious: true, PreviousOffset: 10,
	}); err == nil {
		t.Fatal("expected contradictory previous offset to fail")
	}
}

func TestRenderUIProjectScreenProvidesMonitorPaginationSink(t *testing.T) {
	html := renderUIProjectScreen("/tmp/project")
	if !strings.Contains(html, `data-recyclr-sink="screen-monitor-pagination"`) {
		t.Fatalf("screen tab lacks monitor pagination sink: %s", html)
	}
}
