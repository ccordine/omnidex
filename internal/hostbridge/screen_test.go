package hostbridge

import "testing"

func TestPickScreenMonitor(t *testing.T) {
	monitors := []ScreenMonitor{
		{ID: "DP-1", Name: "DP-1", Primary: false},
		{ID: "HDMI-A-1", Name: "HDMI-A-1", Primary: true},
	}

	got, err := pickScreenMonitor(monitors, "")
	if err != nil {
		t.Fatalf("pick primary: %v", err)
	}
	if got.Name != "HDMI-A-1" {
		t.Fatalf("primary=%q", got.Name)
	}

	got, err = pickScreenMonitor(monitors, "DP-1")
	if err != nil {
		t.Fatalf("pick by id: %v", err)
	}
	if got.Name != "DP-1" {
		t.Fatalf("picked=%q", got.Name)
	}

	_, err = pickScreenMonitor(monitors, "missing")
	if err == nil {
		t.Fatal("expected missing monitor error")
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
