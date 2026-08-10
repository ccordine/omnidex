package queue

import (
	"context"
	"fmt"
	"strconv"

	"github.com/gryph/omnidex/internal/cognition"
	"github.com/gryph/omnidex/internal/taskstate"
	"github.com/jackc/pgx/v5"
)

type cognitionFactProjectionSources map[taskstate.Ref][]cognition.EvidenceRef

func loadCognitionFactProjectionSourcesTx(
	ctx context.Context,
	tx pgx.Tx,
	episode CognitionEpisode,
	graph cognition.ObligationGraphSnapshot,
) (cognitionFactProjectionSources, error) {
	scopes, err := activeCognitionAncestorScopes(graph)
	if err != nil {
		return nil, err
	}
	rows, err := tx.Query(ctx, `
		SELECT facts.entry_id,entries.scope_node_id,entries.updated_version,entries.content_sha256,
		       evidence.position,evidence.observation_id,evidence.revision,
		       evidence.revision_sha256,evidence.content_sha256
		FROM cognition_accepted_facts facts
		JOIN task_entries entries ON entries.ledger_id=facts.ledger_id AND entries.id=facts.entry_id
		JOIN cognition_accepted_fact_evidence evidence ON evidence.fact_id=facts.fact_id
		WHERE facts.episode_id=$1 AND entries.status='active'
		ORDER BY facts.entry_id,evidence.position
	`, episode.EpisodeID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make(cognitionFactProjectionSources)
	positions := make(map[taskstate.Ref]int)
	for rows.Next() {
		var entryID, scopeID, contentSHA, observationID, revisionSHA, evidenceSHA string
		var version, position, revision int64
		if err := rows.Scan(
			&entryID, &scopeID, &version, &contentSHA, &position, &observationID,
			&revision, &revisionSHA, &evidenceSHA,
		); err != nil {
			return nil, err
		}
		if _, relevant := scopes[cognition.ObligationID(scopeID)]; !relevant {
			continue
		}
		if version < 1 || position < 0 || revision < 1 || uint64(revision) > episode.CurrentRevision.Number {
			return nil, fmt.Errorf("%w: accepted fact projection has invalid chronology", ErrCognitionConflict)
		}
		selection := taskstate.Ref{
			URI:     "task:ledger/" + string(episode.LedgerID) + "/entry/" + entryID,
			Version: strconv.FormatInt(version, 10), Hash: contentSHA, Relation: taskstate.RefSource,
		}
		if err := taskstate.ValidateRef(selection); err != nil || positions[selection] != int(position) {
			return nil, fmt.Errorf("%w: accepted fact projection lineage is not contiguous", ErrCognitionConflict)
		}
		ref := cognition.EvidenceRef{
			ObservationID: cognition.ObservationID(observationID),
			Revision: cognition.WorldRevision{
				EpisodeID: episode.EpisodeID, Number: uint64(revision), SHA256: revisionSHA,
			},
			SHA256: evidenceSHA,
		}
		if err := ref.Validate(); err != nil {
			return nil, fmt.Errorf("%w: accepted fact projection evidence: %v", ErrCognitionConflict, err)
		}
		result[selection] = append(result[selection], ref)
		positions[selection]++
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return result, nil
}

func activeCognitionAncestorScopes(
	graph cognition.ObligationGraphSnapshot,
) (map[cognition.ObligationID]struct{}, error) {
	current, err := oneActiveCognitionObligation(graph)
	if err != nil {
		return nil, err
	}
	byID := make(map[cognition.ObligationID]cognition.Obligation, len(graph.Obligations))
	for _, obligation := range graph.Obligations {
		byID[obligation.ID] = obligation
	}
	scopes := make(map[cognition.ObligationID]struct{})
	for {
		if _, duplicate := scopes[current.ID]; duplicate {
			return nil, fmt.Errorf("%w: active obligation ancestry contains a cycle", ErrCognitionConflict)
		}
		scopes[current.ID] = struct{}{}
		if current.ParentID == "" {
			return scopes, nil
		}
		parent, exists := byID[current.ParentID]
		if !exists {
			return nil, fmt.Errorf("%w: active obligation parent is missing", ErrCognitionConflict)
		}
		current = parent
	}
}
