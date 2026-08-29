package omni

import (
	"context"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/model"
)

func TestBuildDBSchemaMemorySnapshotSummarizesAndFingerprintsSchema(t *testing.T) {
	schema := []DBSchemaTable{{
		Schema: "public",
		Name:   "memory_chunks",
		Columns: []DBSchemaColumn{
			{Name: "content", DataType: "text"},
			{Name: "id", DataType: "bigint"},
		},
	}}
	snapshot := BuildDBSchemaMemorySnapshot("omnidex", schema)
	for _, want := range []string{
		"schema_fingerprint=" + snapshot.Fingerprint,
		"- public.memory_chunks",
		"table:public-memory-chunks",
	} {
		if !strings.Contains(snapshot.Content+"\n"+strings.Join(snapshot.Tags, "\n"), want) {
			t.Fatalf("snapshot missing %q", want)
		}
	}
	changed := BuildDBSchemaMemorySnapshot("omnidex", []DBSchemaTable{{
		Schema: "public", Name: "memory_chunks",
		Columns: []DBSchemaColumn{{Name: "source", DataType: "text"}},
	}})
	if changed.Fingerprint == snapshot.Fingerprint {
		t.Fatal("schema fingerprint did not change")
	}
}

func TestStoreDBSchemaMemorySnapshotWritesReferenceMemory(t *testing.T) {
	writer := &fakeDBSchemaMemoryWriter{}
	record, snapshot, err := StoreDBSchemaMemorySnapshot(context.Background(), writer,
		model.MemoryScope{ProjectID: 1, ChannelID: "channel-one"}, "omnidex", []DBSchemaTable{{
			Schema: "public", Name: "jobs", Columns: []DBSchemaColumn{{Name: "id", DataType: "bigint"}},
		}})
	if err != nil {
		t.Fatal(err)
	}
	if record.ID != 1 || writer.input.Kind != model.MemoryKindReference ||
		!strings.Contains(writer.input.Content, snapshot.Fingerprint) ||
		len(writer.input.Categories) != 1 || writer.input.Categories[0] != model.MemoryCategoryDatabase {
		t.Fatalf("record=%+v writer=%+v", record, writer)
	}
}

type fakeDBSchemaMemoryWriter struct {
	input model.MemoryInput
}

func (w *fakeDBSchemaMemoryWriter) AddMemory(_ context.Context, input model.MemoryInput) (MemoryRecord, error) {
	w.input = input
	return memoryRecordFromInput(1, input)
}
