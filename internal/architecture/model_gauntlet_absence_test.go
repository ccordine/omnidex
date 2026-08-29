package architecture

import (
	"os"
	"path/filepath"
	"testing"
)

func TestOfflineModelGauntletRuntimeAndCLITransportAreAbsent(t *testing.T) {
	root := filepath.Clean(filepath.Join("..", ".."))
	for _, path := range []string{
		filepath.Join(root, "internal", "modelgauntlet"),
		filepath.Join(root, "docs", "MODEL_GAUNTLETS.md"),
	} {
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Fatalf("retired model gauntlet surface must be absent: %s", path)
		}
	}
}
