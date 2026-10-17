package manifest

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/gryph/omnidex/internal/modelconfig"
)

func TestManifestDefaultsReachEveryModelConsumer(t *testing.T) {
	raw, err := os.ReadFile("../../manifest.example.json")
	if err != nil {
		t.Fatal(err)
	}
	value, err := Decode(raw)
	if err != nil || value != Defaults() {
		t.Fatalf("example differs from requested defaults: %+v %v", value, err)
	}
	configuration := value.Coding()
	if configuration.Planner.Effort != "xhigh" || configuration.Reviewer.Effort != "xhigh" || configuration.Executor.Effort != "low" || configuration.LedgerParser.Effort != "low" {
		t.Fatalf("incorrect role routing: %+v", configuration)
	}
	authority, err := value.ModelAuthority()
	if err != nil {
		t.Fatal(err)
	}
	for _, field := range modelconfig.RegisteredFields() {
		if authority.Config().Get(field.Key) != value.Models.Default {
			t.Fatalf("default did not reach station %s", field.Key)
		}
	}
}

func TestPresentInvalidManifestNeverUsesOtherDefaults(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	path, err := HomePath()
	if err != nil {
		t.Fatal(err)
	}
	if _, found, err := LoadHome(); err != nil || found {
		t.Fatalf("absent manifest: found=%v error=%v", found, err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		t.Fatal(err)
	}
	for _, raw := range []string{`{`, `null`, `{"core_url":"not-a-url","models":{}}`, `{"core_url":"https://omni.worknet","models":{}}`, `{"core_url":"https://omni.worknet","core_url":"http://elsewhere"}`} {
		if err := os.WriteFile(path, []byte(raw), 0600); err != nil {
			t.Fatal(err)
		}
		if _, _, err := LoadHome(); err == nil {
			t.Fatalf("invalid manifest accepted: %s", raw)
		}
	}
	raw, _ := json.Marshal(Defaults())
	if err := os.WriteFile(path, raw, 0600); err != nil {
		t.Fatal(err)
	}
	if value, found, err := LoadHome(); err != nil || !found || value != Defaults() {
		t.Fatalf("home manifest was not consumed: %+v %v %v", value, found, err)
	}
}
