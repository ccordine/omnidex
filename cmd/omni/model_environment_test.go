package main

import (
	"bytes"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/modelconfig"
)

func TestModelEnvironmentUsesCompleteRuntimeRegistry(t *testing.T) {
	var output bytes.Buffer
	if err := writeModelEnvironment(&output, "native-model"); err != nil {
		t.Fatal(err)
	}
	for _, field := range modelconfig.RegisteredFields() {
		for _, key := range field.EnvKeys {
			if !strings.Contains(output.String(), key+"=native-model\n") {
				t.Fatalf("missing routing field %s", key)
			}
		}
	}
	for _, model := range []string{"", "model\nINJECTED=yes", "model name"} {
		if err := writeModelEnvironment(&output, model); err == nil {
			t.Fatal("unsafe environment value accepted")
		}
	}
}
