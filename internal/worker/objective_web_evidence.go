package worker

import (
	"fmt"
	"slices"

	"github.com/gryph/omnidex/internal/queue"
	"github.com/gryph/omnidex/internal/webresearch"
)

func projectObjectiveRoleplayResearchEvidence(
	record queue.WebEvidenceRecord,
) ([]objectiveEvidence, error) {
	if record.ID < 1 || record.JobID < 1 {
		return nil, fmt.Errorf("web evidence requires its recorded acquisition and owning job")
	}
	evidence, projection, err := record.Acquired.Project()
	if err != nil {
		return nil, err
	}
	byID := make(map[webresearch.EvidenceID]webresearch.Evidence, len(evidence))
	for _, item := range evidence {
		byID[item.ID] = item
	}
	projected := make([]objectiveEvidence, len(projection))
	for index, bounded := range projection {
		item, exists := byID[bounded.EvidenceID]
		if !exists || item.CandidateID != bounded.CandidateID {
			return nil, fmt.Errorf("roleplay research bounded evidence lost exact acquisition authority")
		}
		text, capsuleTruncated, err := webresearch.CitationExcerpt(bounded)
		if err != nil {
			return nil, err
		}
		projected[index], err = newObjectiveEvidence(
			string(item.ID), text, "web_document", item.URL,
		)
		if err != nil {
			return nil, err
		}
		projected[index].WebEvidenceID = record.ID
		projected[index].WebEvidenceIndex = index
		projected[index].ObservedAt = item.ObservedAt
		projected[index].Truncated = item.Truncated || bounded.Truncated || capsuleTruncated
	}
	return projected, nil
}

func bindObjectiveRoleplayResearchCitations(
	projected []objectiveEvidence,
	artifact webresearch.Artifact,
) ([]objectiveEvidence, []string, error) {
	byID := make(map[string]int, len(projected))
	for index, item := range projected {
		byID[item.Capsule.ID] = index
	}
	selected := make([]objectiveEvidence, 0, len(artifact.Sources))
	ids := make([]string, 0, len(artifact.Sources))
	for _, source := range artifact.Sources {
		index, exists := byID[string(source.EvidenceID)]
		if !exists || projected[index].SourceRef != source.URL ||
			!projected[index].ObservedAt.Equal(source.ObservedAt) {
			return nil, nil, fmt.Errorf("roleplay research citation lost exact acquired evidence")
		}
		item := projected[index]
		for paragraphIndex, paragraph := range artifact.Paragraphs {
			if slices.Contains(paragraph.EvidenceIDs, source.EvidenceID) {
				item.ParagraphMask |= 1 << paragraphIndex
			}
		}
		if item.ParagraphMask == 0 {
			return nil, nil, fmt.Errorf("roleplay research citation has no paragraph binding")
		}
		if err := validateObjectiveEvidence(item); err != nil {
			return nil, nil, err
		}
		selected = append(selected, item)
		ids = append(ids, item.Capsule.ID)
	}
	return selected, ids, nil
}
