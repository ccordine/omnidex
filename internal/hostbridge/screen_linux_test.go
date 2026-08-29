//go:build linux

package hostbridge

import (
	"strings"
	"testing"
)

func TestScanHyprlandMonitorsStreamsBoundedPage(t *testing.T) {
	raw := `[
		{"id":1,"name":"DP-1","width":1920,"height":1080,"x":0,"y":0,"focused":false},
		{"id":2,"name":"HDMI-A-1","width":2560,"height":1440,"x":1920,"y":0,"focused":true},
		{"id":3,"name":"DP-2","width":1280,"height":720,"x":4480,"y":0,"focused":false}
	]`
	page, found, err := collectScreenMonitorPage(ScreenMonitorPageRequest{Limit: 1, Offset: 1}, "hyprland-grim", func(visit func(ScreenMonitor) bool) (bool, int, error) {
		return scanHyprlandMonitors(strings.NewReader(raw), visit)
	})
	if err != nil {
		t.Fatal(err)
	}
	if !found || len(page.Monitors) != 1 || page.Monitors[0].ID != "HDMI-A-1" || !page.Monitors[0].Primary || !page.HasMore {
		t.Fatalf("page=%+v", page)
	}
}

func TestScanXRandRMonitorsStreamsBoundedPage(t *testing.T) {
	raw := "Monitors: 3\n 0: +DP-1 1920/500x1080/300+0+0  DP-1\n 1: +*HDMI-A-1 2560/600x1440/340+1920+0 HDMI-A-1\n 2: +DP-2 1280/400x720/200+4480+0 DP-2\n"
	page, found, err := collectScreenMonitorPage(ScreenMonitorPageRequest{Limit: 2}, "x11", func(visit func(ScreenMonitor) bool) (bool, int, error) {
		return scanXRandRMonitors(strings.NewReader(raw), visit)
	})
	if err != nil {
		t.Fatal(err)
	}
	if !found || len(page.Monitors) != 2 || page.Monitors[1].ID != "HDMI-A-1" || !page.Monitors[1].Primary || !page.HasMore {
		t.Fatalf("page=%+v", page)
	}
}
