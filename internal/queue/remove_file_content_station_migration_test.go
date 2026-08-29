package queue

import (
	"os"
	"strings"
	"testing"
)

func TestRemovalMigrationMakesFormerFileContentWorkKindUnowned(t *testing.T) {
	raw, err := os.ReadFile("../../migrations/114_remove_file_content_station.sql")
	if err != nil {
		t.Fatal(err)
	}
	source := strings.Join(strings.Fields(string(raw)), " ")
	if !strings.Contains(source, "CREATE OR REPLACE FUNCTION station_owns_portable_work") {
		t.Fatal("removal migration does not replace portable-work ownership")
	}
	if strings.Contains(source, "WHEN 'application_file_content'") {
		t.Fatal("removal migration still assigns the removed mapper to a station")
	}
	if !strings.Contains(source, "ELSE FALSE") {
		t.Fatal("removal migration does not explicitly reject unregistered work kinds")
	}
}
