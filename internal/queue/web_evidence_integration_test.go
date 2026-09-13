package queue

import (
	"context"
	"encoding/json"
	"maps"
	"reflect"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/evidence"
	"github.com/gryph/omnidex/internal/model"
)

func TestWebEvidenceRecordsHTTPValuesAndVerifiesCitations(t *testing.T) {
	databaseURL := evidenceDatabaseURL(t)
	for _, fixture := range []struct{ title, content string }{
		{"Temperature observations", "The recorded temperature is twelve degrees."},
		{"Archive reading notes", strings.Repeat("The catalogue records the original edition. ", 90)},
	} {
		t.Run(fixture.title, func(t *testing.T) {
			ctx := context.Background()
			pool, repository := freshEvidenceRepository(t, databaseURL)
			_, job := enqueueChatCodingPipelineFixture(t, repository, "Explain the fetched source.")
			claim, err := repository.ClaimNextStep(ctx, "web-evidence-test")
			if err != nil || claim == nil || claim.Job.ID != job.ID {
				t.Fatalf("claim = %#v, error = %v", claim, err)
			}
			httpFixture := newWebEvidenceHTTPFixture(t, fixture.title, fixture.content)
			acquired := httpFixture.acquire(t)
			if httpFixture.requests.Load() != 2 || acquired.Discovery.Query != fixture.title ||
				acquired.Fetch.Documents[0].Content != fixture.title+" "+strings.TrimSpace(fixture.content) ||
				acquired.Fetch.Documents[0].Truncated {
				t.Fatalf("actual requests and response text differ: %#v", acquired)
			}
			stored, err := repository.RecordWebEvidence(ctx, job.ID, acquired)
			if err != nil {
				t.Fatal(err)
			}
			loaded, err := repository.GetWebEvidence(ctx, job.ID, stored.ID)
			if err != nil || !reflect.DeepEqual(loaded, stored) {
				t.Fatalf("web observations did not round-trip: error=%v\nwant=%#v\ngot=%#v", err, stored, loaded)
			}
			if _, err := repository.GetWebEvidence(ctx, job.ID+1000, stored.ID); err == nil {
				t.Fatal("another job read the acquired source")
			}
			second, err := repository.RecordWebEvidence(ctx, job.ID, httpFixture.acquire(t))
			if err != nil || second.ID == stored.ID {
				t.Fatalf("an equal second fetch was collapsed into a content receipt: %#v / %v", second, err)
			}
			citation := webEvidenceCitation(t, stored, claim.Step.ID)
			operationID, err := model.NewLifecycleOperationID()
			if err != nil {
				t.Fatal(err)
			}
			completion := CompleteStepEvidenceCommand{CompleteStepCommand: CompleteStepCommand{
				OperationID: operationID, Authority: claim.Authority, StepID: claim.Step.ID,
				Output: "The source returned the recorded observation.", ContextKey: "objective_result",
			}, Evidence: []evidence.Record{citation}}
			for _, mutation := range []struct {
				name  string
				apply func(*evidence.Record)
			}{
				{"excerpt", func(c *evidence.Record) { c.Excerpt = "An invented source excerpt." }},
				{"source", func(c *evidence.Record) { c.SourceRef = "https://unrelated.example/document" }},
				{"time", func(c *evidence.Record) { c.Metadata["source_observed_at"] = "2026-01-01T00:00:00Z" }},
				{"truncation", func(c *evidence.Record) { c.Metadata["source_truncated"] = !c.Metadata["source_truncated"].(bool) }},
				{"missing record", func(c *evidence.Record) { c.Metadata["web_evidence_id"] = "999999999" }},
				{"source index", func(c *evidence.Record) { c.Metadata["web_evidence_index"] = "1" }},
				{"capsule", func(c *evidence.Record) { c.Metadata["capsule_id"] = "document_2" }},
				{"obsolete hash metadata", func(c *evidence.Record) { c.Metadata["source_sha256"] = strings.Repeat("a", 64) }},
			} {
				invalid := completion
				changed := citation
				changed.Metadata = maps.Clone(citation.Metadata)
				mutation.apply(&changed)
				invalid.Evidence = []evidence.Record{changed}
				if err := repository.CompleteStepWithEvidence(ctx, invalid); err == nil {
					t.Fatalf("completion accepted changed %s", mutation.name)
				}
			}
			var evidenceCount int
			if err := pool.QueryRow(ctx, "SELECT count(*) FROM evidence WHERE job_id=$1", job.ID).Scan(&evidenceCount); err != nil || evidenceCount != 0 {
				t.Fatalf("failed citation persisted partial completion: count=%d, error=%v", evidenceCount, err)
			}
			if err := repository.CompleteStepWithEvidence(ctx, completion); err != nil {
				t.Fatal(err)
			}
			if err := repository.CompleteStepWithEvidence(ctx, completion); err != nil {
				t.Fatalf("exact completion replay: %v", err)
			}
			var records int
			if err := pool.QueryRow(ctx, "SELECT count(*) FROM web_evidence WHERE job_id=$1", job.ID).Scan(&records); err != nil || records != 2 || httpFixture.requests.Load() != 4 {
				t.Fatalf("completion added acquisition work: records=%d, requests=%d, error=%v", records, httpFixture.requests.Load(), err)
			}
			encoded, err := json.Marshal(stored)
			if err != nil || strings.Contains(strings.ToLower(string(encoded)), "sha256") {
				t.Fatalf("web evidence retained a hash: %s / %v", encoded, err)
			}
		})
	}
}
