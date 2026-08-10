package queue

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"testing"

	"github.com/gryph/omnidex/internal/workingset"
)

func TestPostgresCognitionTraceSchemaAuthorityIsExactAndImmutable(t *testing.T) {
	_, _, pool := openWorkingSetDatabase(t)
	tx, err := pool.Begin(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(context.Background())
	if err := requireCognitionTraceSchemaAuthorityTx(t.Context(), tx); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(t.Context()); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(t.Context(), `
		UPDATE cognition_trace_schema_authority
		SET trace_schema='omnidex.cognition-trace-authority.v1'
		WHERE singleton=TRUE
	`); err == nil {
		t.Fatal("durable cognition trace schema authority was mutable")
	}
}

func TestPostgresSealedTraceRejectsActiveEpisode(t *testing.T) {
	fixture := startTaskGenerationRetirementFixture(t, "active-sealed-trace")
	_, err := fixture.Repository.ReadCognitionSealedTrace(
		fixture.Context, fixture.EpisodeID, CognitionTracePageRequest{Limit: 1},
	)
	if !errors.Is(err, ErrCognitionConflict) {
		t.Fatalf("active trace read error=%v", err)
	}
}

func assertSealedWorkingSetTrace(
	t *testing.T,
	fixture taskGenerationRetirementFixture,
	seal CognitionTerminalSeal,
) {
	t.Helper()
	offset := 0
	var start, final *CognitionTraceWorkingSetSnapshot
	events := make([]CognitionTraceWorkingSetEvent, 0)
	seen := 0
	for {
		page, err := fixture.Repository.ReadCognitionSealedTrace(
			fixture.Context, fixture.EpisodeID,
			CognitionTracePageRequest{Offset: offset, Limit: 3},
		)
		if err != nil {
			t.Fatal(err)
		}
		if page.TraceSHA256 != seal.TraceSHA256 || page.Seal.TraceSHA256 != seal.TraceSHA256 ||
			page.EpisodeStartedAt.IsZero() || page.SealedAt.IsZero() || page.TotalRecords < 3 {
			t.Fatalf("sealed trace header=%+v", page)
		}
		seen += len(page.Records)
		for _, record := range page.Records {
			switch record.Kind {
			case "working_set_snapshot":
				var payload CognitionTraceWorkingSetSnapshot
				if err := json.Unmarshal(record.Payload, &payload); err != nil {
					t.Fatal(err)
				}
				if payload.Point == "episode_start" {
					copy := payload
					start = &copy
				} else if payload.Point == "terminal" {
					copy := payload
					final = &copy
				}
			case "working_set_event":
				var payload CognitionTraceWorkingSetEvent
				if err := json.Unmarshal(record.Payload, &payload); err != nil {
					t.Fatal(err)
				}
				events = append(events, payload)
			}
		}
		if page.NextOffset < 0 {
			if seen != page.TotalRecords {
				t.Fatalf("sealed trace records=%d want %d", seen, page.TotalRecords)
			}
			break
		}
		offset = page.NextOffset
	}
	if start == nil || final == nil || len(events) == 0 ||
		final.Snapshot.Version != seal.WorkingSetVersion {
		t.Fatalf("Working Set trace start=%+v final=%+v events=%d", start, final, len(events))
	}
	replayed, err := workingset.Restore(start.Snapshot)
	if err != nil {
		t.Fatal(err)
	}
	for _, supplied := range events {
		command, err := workingset.DecodeCommand(supplied.Event.CommandKind, supplied.Event.Command)
		if err != nil {
			t.Fatal(err)
		}
		actual, err := replayed.Apply(command)
		if err != nil || !reflect.DeepEqual(actual, supplied.Event) {
			t.Fatalf("Working Set trace replay actual=%+v supplied=%+v error=%v", actual, supplied.Event, err)
		}
	}
	if !reflect.DeepEqual(replayed.Snapshot(), final.Snapshot) {
		t.Fatalf("Working Set terminal snapshot diverged after replay")
	}
}
