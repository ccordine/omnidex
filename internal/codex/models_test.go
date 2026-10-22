package codex

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestModelCatalogPreservesNativeOptionsAndActualModelHandle(t *testing.T) {
	var first nativeModelPage
	if err := json.Unmarshal([]byte(`{"data":[{"id":"picker-identity","model":"future-native-model","displayName":"Future model","defaultReasoningEffort":"ultra","supportedReasoningEfforts":[{"reasoningEffort":"ultra","description":"Deepest"},{"reasoningEffort":"new-effort","description":"New"}],"serviceTiers":[{"id":"priority","name":"Fast","description":"Priority processing"}],"defaultServiceTier":"priority"}],"nextCursor":"next-page"}`), &first); err != nil {
		t.Fatal(err)
	}
	calls := 0
	models, err := readModelCatalog(func(cursor *string) (nativeModelPage, error) {
		calls++
		if calls == 1 {
			if cursor != nil {
				t.Fatal("first request supplied a cursor")
			}
			return first, nil
		}
		if cursor == nil || *cursor != "next-page" {
			t.Fatal("catalog pagination discarded the native cursor")
		}
		return nativeModelPage{Data: []nativeModel{{Model: "another-model", DisplayName: "Another", DefaultReasoningEffort: "high"}}}, nil
	})
	if err != nil || len(models) != 2 || calls != 2 {
		t.Fatalf("catalog = %+v, calls=%d, err=%v", models, calls, err)
	}
	model := models[0]
	if model.ID != "future-native-model" || model.DefaultEffort != "ultra" || model.Efforts[1].Value != "new-effort" || model.ServiceTiers[0].ID != "priority" || model.DefaultServiceTier == nil || *model.DefaultServiceTier != "priority" {
		t.Fatalf("native model options were replaced or lost: %+v", model)
	}
}

func TestModelCatalogRejectsCursorCyclesAndDuplicateModelHandles(t *testing.T) {
	for _, cycle := range []bool{true, false} {
		cursor := "repeated"
		_, err := readModelCatalog(func(*string) (nativeModelPage, error) {
			page := nativeModelPage{NextCursor: &cursor}
			if !cycle {
				page.Data = []nativeModel{{Model: "same", DisplayName: "Same", DefaultReasoningEffort: "high"}}
			}
			return page, nil
		})
		if err == nil {
			t.Fatal("nonconvergent or ambiguous catalog accepted")
		}
	}
}

func TestInstalledVersionAndUpdateBindingValidation(t *testing.T) {
	for _, output := range []string{"other-cli 1.0", "codex-cli 1.0\nsecret=value", "codex-cli"} {
		if _, err := codexVersion(output); err == nil {
			t.Fatal("foreign or malformed installation response accepted")
		}
	}
	version, err := codexVersion("codex-cli 0.162.1\n")
	if err != nil || version != "0.162.1" {
		t.Fatalf("version=%q err=%v", version, err)
	}
	prefix := filepath.Join(string(filepath.Separator), "existing", "node")
	root := filepath.Join(prefix, "lib", "node_modules", "@openai", "codex")
	actual, err := existingNpmPrefix(root)
	if err != nil || actual != prefix {
		t.Fatalf("existing prefix=%q err=%v", actual, err)
	}
	if _, err := existingNpmPrefix(filepath.Join(prefix, "some-project", "node_modules", "@openai", "codex")); err == nil {
		t.Fatal("project-local package authorized a global installation update")
	}
	t.Setenv("CODEX_MANAGED_PACKAGE_ROOT", "/foreign")
	t.Setenv("npm_config_prefix", "/foreign")
	environment := codexUpdateBinding{packageRoot: root, prefix: prefix}.environment()
	for _, value := range environment {
		if strings.Contains(value, "/foreign") {
			t.Fatal("inherited update authority escaped the existing installation")
		}
	}
}

func TestInstalledCodexCatalogAndVersion(t *testing.T) {
	command := os.Getenv("OMNI_TEST_CODEX_COMMAND")
	if command == "" {
		t.Skip("installed Codex catalog qualification not selected")
	}
	client, err := New(command, ".")
	if err != nil {
		t.Fatal(err)
	}
	installation, err := client.Installation(context.Background())
	if err != nil || installation.Version == "" {
		t.Fatalf("installation=%+v err=%v", installation, err)
	}
	models, err := client.Models(context.Background())
	if err != nil || len(models) == 0 {
		t.Fatalf("native catalog entries=%d err=%v", len(models), err)
	}
	t.Logf("actual installed Codex version=%s update_available=%t models=%d first_model=%s first_efforts=%+v first_service_tiers=%+v", installation.Version, installation.CanUpdate, len(models), models[0].ID, models[0].Efforts, models[0].ServiceTiers)
}
