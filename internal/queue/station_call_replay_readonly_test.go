package queue

import (
	"os"
	"strings"
	"testing"
)

func TestStationCallReplayReadPathDoesNotRequestRowLocks(t *testing.T) {
	source, err := os.ReadFile("station_call_replay.go")
	if err != nil {
		t.Fatalf("read replay source: %v", err)
	}
	if strings.Contains(string(source), "FOR SHARE") {
		t.Fatal("station replay read path must not request a row lock in a read-only transaction")
	}
	if strings.Contains(string(source), "loadStationCallOpeningTx") ||
		strings.Contains(string(source), "loadStationGapOpeningTx") {
		t.Fatal("station replay read path must not reuse the locking production loaders")
	}
}
