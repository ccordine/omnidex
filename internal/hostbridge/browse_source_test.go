package hostbridge

import (
	"os"
	"strings"
	"testing"
)

func TestBrowseProductionDoesNotUseUnboundedReadDir(t *testing.T) {
	raw, err := os.ReadFile("browse.go")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), "os.ReadDir(") {
		t.Fatal("browse.go must not load an entire directory before pagination")
	}
	if !strings.Contains(string(raw), ".ReadDir(browseReadChunkSize)") {
		t.Fatal("browse.go must scan directories in an explicit bounded chunk")
	}
}
