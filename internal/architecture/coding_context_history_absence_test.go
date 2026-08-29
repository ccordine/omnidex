package architecture

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestCodingContextHasNoHistoricalOrMemoryAuthorityChannel(t *testing.T) {
	t.Parallel()
	root := filepath.Clean(filepath.Join("..", "..", "internal"))
	forbidden := []string{
		"AdditionalAuthority",
		"MemoryAuthorities",
		"ObjectiveMemoryAuthority",
		"ApplicationContextAcceptedMemory",
		"ApplicationContextMemoryAuthority",
	}
	walkProductionSource(t, root, func(path, source string) {
		for _, identifier := range forbidden {
			if strings.Contains(source, identifier) {
				t.Errorf("production source %s retains coding history authority %q", path, identifier)
			}
		}
	})
}
