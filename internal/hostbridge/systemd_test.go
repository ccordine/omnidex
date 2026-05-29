package hostbridge

import (
	"strings"
	"testing"
)

func TestRenderServiceEnvFileDefaults(t *testing.T) {
	body := RenderServiceEnvFile("", "")
	if !strings.Contains(body, "HOST_AGENT_LISTEN=0.0.0.0:8091") {
		t.Fatalf("expected docker-friendly default listen address, got:\n%s", body)
	}
	if strings.Contains(body, "HOST_AGENT_TOKEN=") && !strings.Contains(body, "# HOST_AGENT_TOKEN=") {
		t.Fatalf("expected commented token line when unset, got:\n%s", body)
	}
}

func TestRenderServiceEnvFileWithToken(t *testing.T) {
	body := RenderServiceEnvFile("127.0.0.1:9000", "secret")
	if !strings.Contains(body, "HOST_AGENT_LISTEN=127.0.0.1:9000") {
		t.Fatalf("missing listen override: %s", body)
	}
	if !strings.Contains(body, "HOST_AGENT_TOKEN=secret") {
		t.Fatalf("missing token: %s", body)
	}
}

func TestRenderSystemdUnit(t *testing.T) {
	unit := RenderSystemdUnit("/usr/local/bin/omni", "/home/test/.config/omni/host-bridge.env")
	for _, want := range []string{
		"ExecStart=/usr/local/bin/omni host serve",
		"EnvironmentFile=-/home/test/.config/omni/host-bridge.env",
		"WantedBy=default.target",
		"Restart=on-failure",
	} {
		if !strings.Contains(unit, want) {
			t.Fatalf("unit missing %q:\n%s", want, unit)
		}
	}
}
