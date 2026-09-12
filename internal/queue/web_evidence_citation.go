package queue

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/gryph/omnidex/internal/evidence"
	"github.com/gryph/omnidex/internal/webresearch"
	"github.com/jackc/pgx/v5"
)

func webCitationPosition(metadata map[string]any) (int64, int, error) {
	idText, err := objectiveMetadataString(metadata, "web_evidence_id", 19)
	if err != nil {
		return 0, 0, err
	}
	id, err := strconv.ParseInt(idText, 10, 64)
	if err != nil || id < 1 || strconv.FormatInt(id, 10) != idText {
		return 0, 0, fmt.Errorf("web citation requires a canonical positive record ID")
	}
	indexText, err := objectiveMetadataString(metadata, "web_evidence_index", 2)
	if err != nil {
		return 0, 0, err
	}
	index, err := strconv.Atoi(indexText)
	if err != nil || index < 0 || index >= 32 || strconv.Itoa(index) != indexText {
		return 0, 0, fmt.Errorf("web citation requires a canonical source index in 0..31")
	}
	return id, index, nil
}

func validateRecordedWebCitation(ctx context.Context, tx pgx.Tx, citation evidence.Record) error {
	id, index, err := webCitationPosition(citation.Metadata)
	if err != nil {
		return err
	}
	record, err := readWebEvidence(ctx, tx, citation.JobID, id)
	if err != nil {
		return err
	}
	sources, projected, err := record.Acquired.Project()
	if err != nil {
		return err
	}
	if index >= len(sources) {
		return fmt.Errorf("web citation source index exceeds the recorded selection")
	}
	source := sources[index]
	excerpt, truncated, err := webresearch.CitationExcerpt(projected[index])
	if err != nil {
		return err
	}
	if citation.SourceRef != source.URL || citation.Excerpt != excerpt ||
		citation.Metadata["capsule_id"] != string(source.ID) ||
		citation.Metadata["source_observed_at"] != source.ObservedAt.Format(time.RFC3339Nano) ||
		citation.Metadata["source_truncated"] != (source.Truncated || projected[index].Truncated || truncated) {
		return fmt.Errorf("web citation differs from its recorded source text, URL, observation, or projection")
	}
	return nil
}
