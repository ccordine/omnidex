package queue

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strconv"

	"github.com/gryph/omnidex/internal/cognition"
	"github.com/gryph/omnidex/internal/cognitionstate"
	"github.com/gryph/omnidex/internal/taskstate"
	"github.com/gryph/omnidex/internal/workingset"
	"github.com/jackc/pgx/v5"
)

func loadCognitionEvidencePacketTx(
	ctx context.Context,
	tx pgx.Tx,
	episode CognitionEpisode,
	graph cognition.ObligationGraphSnapshot,
	set workingset.Snapshot,
	maximum int,
) ([]cognitionstate.EvidenceMaterial, []cognition.EvidenceRef, cognitionFactProjectionSources, error) {
	if maximum < 0 {
		return nil, nil, nil, fmt.Errorf("cognition evidence packet limit is negative")
	}
	selected, err := selectActiveCognitionEvidence(graph)
	if err != nil {
		return nil, nil, nil, err
	}
	current, err := oneActiveCognitionObligation(graph)
	if err != nil {
		return nil, nil, nil, err
	}
	currentRefs, err := loadCurrentCognitionEvidenceRefsTx(ctx, tx, episode)
	if err != nil {
		return nil, nil, nil, err
	}
	for _, ref := range currentRefs {
		applies, applyErr := currentCognitionEvidenceApplies(
			set, current.ID, episode.CurrentRevision, ref,
		)
		if applyErr != nil {
			return nil, nil, nil, applyErr
		}
		if applies {
			selected[ref] = struct{}{}
		}
	}
	if len(selected) > maximum {
		return nil, nil, nil, fmt.Errorf("%w: causal evidence exceeds the episode evidence budget", ErrCognitionConflict)
	}
	factSources, err := loadCognitionFactProjectionSourcesTx(ctx, tx, episode, graph)
	if err != nil {
		return nil, nil, nil, err
	}
	for _, refs := range factSources {
		for _, ref := range refs {
			selected[ref] = struct{}{}
		}
	}
	for _, item := range set.Items {
		if item.State != workingset.ItemResident || item.Role != workingset.RoleEvidence {
			continue
		}
		if !cognitionEvidenceMembershipApplies(item, set.Scope, current.ID) {
			continue
		}
		ref, found, err := loadResidentCognitionEvidenceRefTx(ctx, tx, episode.EpisodeID, item.Ref)
		if err != nil {
			return nil, nil, nil, err
		}
		if found {
			selected[ref] = struct{}{}
		}
	}
	if len(selected) > maximum {
		return nil, nil, nil, fmt.Errorf("%w: resident or accepted-fact evidence exceeds the episode evidence budget", ErrCognitionConflict)
	}
	refs := make([]cognition.EvidenceRef, 0, len(selected))
	for ref := range selected {
		refs = append(refs, ref)
	}
	sort.Slice(refs, func(left, right int) bool {
		if refs[left].Revision.Number != refs[right].Revision.Number {
			return refs[left].Revision.Number < refs[right].Revision.Number
		}
		return refs[left].ObservationID < refs[right].ObservationID
	})
	materials := make([]cognitionstate.EvidenceMaterial, 0, len(refs))
	for _, ref := range refs {
		observation, err := loadCognitionObservationTx(ctx, tx, ref)
		if err != nil {
			return nil, nil, nil, err
		}
		materials = append(materials, cognitionstate.EvidenceMaterial{Ref: ref, Content: observation.Content})
	}
	return materials, refs, factSources, nil
}

func currentCognitionEvidenceApplies(
	set workingset.Snapshot,
	obligationID cognition.ObligationID,
	revision cognition.WorldRevision,
	ref cognition.EvidenceRef,
) (bool, error) {
	wantRef := cognitionEvidenceTaskRefs([]cognition.EvidenceRef{ref})[0]
	wantMembership, err := cognitionstate.AttentionMembership(
		cognition.AttentionScopeDecision, set.Scope, obligationID, revision.SHA256,
	)
	if err != nil {
		return false, err
	}
	for _, item := range set.Items {
		if item.State != workingset.ItemResident || item.Role != workingset.RoleEvidence || item.Ref != wantRef {
			continue
		}
		for _, membership := range item.Memberships {
			if membership == wantMembership {
				return true, nil
			}
		}
	}
	return false, nil
}

func loadCurrentCognitionEvidenceRefsTx(
	ctx context.Context,
	tx pgx.Tx,
	episode CognitionEpisode,
) ([]cognition.EvidenceRef, error) {
	rows, err := tx.Query(ctx, `
		SELECT observations.observation_json
		FROM cognition_transition_observations observations
		JOIN cognition_transitions transitions ON transitions.transition_id=observations.transition_id
		WHERE transitions.episode_id=$1 AND transitions.revision=$2
		ORDER BY observations.observation_id
	`, episode.EpisodeID, int64(episode.CurrentRevision.Number))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	refs := make([]cognition.EvidenceRef, 0)
	for rows.Next() {
		var raw []byte
		if err := rows.Scan(&raw); err != nil {
			return nil, err
		}
		var observation cognition.Observation
		if err := json.Unmarshal(raw, &observation); err != nil || observation.Validate() != nil ||
			observation.Revision != episode.CurrentRevision {
			return nil, fmt.Errorf("%w: current revision observation changed", ErrCognitionConflict)
		}
		refs = append(refs, observation.EvidenceRef())
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return refs, nil
}

func cognitionEvidenceMembershipApplies(
	item workingset.Item,
	root workingset.Scope,
	current cognition.ObligationID,
) bool {
	for _, membership := range item.Memberships {
		if cognitionstate.AttentionMembershipApplies(membership, root, current) {
			return true
		}
	}
	return false
}

func selectActiveCognitionEvidence(
	graph cognition.ObligationGraphSnapshot,
) (map[cognition.EvidenceRef]struct{}, error) {
	selected := make(map[cognition.EvidenceRef]struct{})
	active := 0
	for _, obligation := range graph.Obligations {
		if obligation.Status != cognition.ObligationActive {
			continue
		}
		active++
		for _, ref := range obligation.SupportingRefs {
			selected[ref] = struct{}{}
		}
	}
	if active != 1 {
		return nil, fmt.Errorf("%w: evidence selection requires exactly one active obligation", ErrCognitionConflict)
	}
	return selected, nil
}

func loadResidentCognitionEvidenceRefTx(
	ctx context.Context,
	tx pgx.Tx,
	episodeID cognition.EpisodeID,
	ref taskstate.Ref,
) (cognition.EvidenceRef, bool, error) {
	revision, err := strconv.ParseUint(ref.Version, 10, 64)
	if err != nil {
		return cognition.EvidenceRef{}, false, nil
	}
	rows, err := tx.Query(ctx, `
		SELECT observations.observation_json
		FROM cognition_transition_observations observations
		JOIN cognition_transitions transitions ON transitions.transition_id=observations.transition_id
		WHERE transitions.episode_id=$1 AND transitions.revision=$2 AND observations.content_sha256=$3
		ORDER BY observations.observation_id LIMIT 2
	`, episodeID, int64(revision), ref.Hash)
	if err != nil {
		return cognition.EvidenceRef{}, false, err
	}
	defer rows.Close()
	var matches []cognition.EvidenceRef
	for rows.Next() {
		var raw []byte
		if err := rows.Scan(&raw); err != nil {
			return cognition.EvidenceRef{}, false, err
		}
		var observation cognition.Observation
		if err := json.Unmarshal(raw, &observation); err != nil || observation.Validate() != nil {
			return cognition.EvidenceRef{}, false, fmt.Errorf("decode resident cognition observation")
		}
		candidate := observation.EvidenceRef()
		if cognitionEvidenceTaskRef(candidate) == ref.URI && ref.Relation == taskstate.RefEvidence {
			matches = append(matches, candidate)
		}
	}
	if err := rows.Err(); err != nil {
		return cognition.EvidenceRef{}, false, err
	}
	if len(matches) > 1 {
		return cognition.EvidenceRef{}, false, fmt.Errorf("%w: resident cognition evidence is ambiguous", ErrCognitionConflict)
	}
	if len(matches) == 0 {
		return cognition.EvidenceRef{}, false, nil
	}
	return matches[0], true, nil
}

func loadCognitionObservationTx(ctx context.Context, tx pgx.Tx, ref cognition.EvidenceRef) (cognition.Observation, error) {
	var raw []byte
	if err := tx.QueryRow(ctx, `
		SELECT observations.observation_json
		FROM cognition_transition_observations observations
		JOIN cognition_transitions transitions ON transitions.transition_id=observations.transition_id
		WHERE transitions.episode_id=$1 AND transitions.revision=$2
		  AND observations.observation_id=$3 AND observations.content_sha256=$4
	`, ref.Revision.EpisodeID, int64(ref.Revision.Number), ref.ObservationID, ref.SHA256).Scan(&raw); err != nil {
		return cognition.Observation{}, fmt.Errorf("load cognition observation %q: %w", ref.ObservationID, err)
	}
	var observation cognition.Observation
	if err := json.Unmarshal(raw, &observation); err != nil {
		return cognition.Observation{}, err
	}
	if err := observation.Validate(); err != nil || observation.EvidenceRef() != ref {
		return cognition.Observation{}, fmt.Errorf("%w: persisted cognition observation changed", ErrCognitionConflict)
	}
	return observation, nil
}
