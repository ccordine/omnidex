package architecture

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestModelGauntletCannotRouteProductionWork(t *testing.T) {
	repositoryRoot := filepath.Clean(filepath.Join("..", ".."))
	for _, sourceRoot := range []string{"internal", "cmd"} {
		root := filepath.Join(repositoryRoot, sourceRoot)
		err := walkProductionGo(root, func(path string, raw []byte) error {
			relative, err := filepath.Rel(repositoryRoot, path)
			if err != nil {
				return err
			}
			if strings.HasPrefix(relative, filepath.Join("internal", "modelgauntlet")) ||
				strings.HasPrefix(relative, filepath.Join("cmd", "cli")) {
				return nil
			}
			if strings.Contains(string(raw), "github.com/gryph/omnidex/internal/modelgauntlet") {
				t.Fatalf("production source %s imports the offline model gauntlet", relative)
			}
			return nil
		})
		if err != nil {
			t.Fatalf("scan %s production source: %v", sourceRoot, err)
		}
	}
}
