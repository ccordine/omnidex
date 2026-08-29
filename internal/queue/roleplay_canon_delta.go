package queue

import (
	"context"
	"fmt"

	"github.com/gryph/omnidex/internal/roleplay"
)

// FilterNewRoleplayCanonFacts performs an exact byte comparison against
// world-global canon. It receives only model-returned candidates and never
// projects hidden facts back to a model.
func (r *Repository) FilterNewRoleplayCanonFacts(
	ctx context.Context,
	worldID string,
	candidates []string,
) ([]string, error) {
	if r == nil || r.pool == nil || ctx == nil {
		return nil, fmt.Errorf("roleplay canon delta requires repository authority")
	}
	if worldID == "" {
		return nil, fmt.Errorf("roleplay canon delta requires an exact world identity")
	}
	if len(candidates) > roleplay.MaxCanonFactsPerTurn {
		return nil, fmt.Errorf("roleplay canon delta exceeds its candidate bound")
	}
	seen := make(map[string]struct{}, len(candidates))
	for _, fact := range candidates {
		if err := roleplay.ValidateCanonFact(fact); err != nil {
			return nil, err
		}
		if _, duplicate := seen[fact]; duplicate {
			return nil, fmt.Errorf("roleplay canon delta candidate is duplicated")
		}
		seen[fact] = struct{}{}
	}
	if len(candidates) == 0 {
		return []string{}, nil
	}
	var worldExists bool
	if err := r.pool.QueryRow(ctx, `
		SELECT EXISTS (SELECT 1 FROM roleplay_worlds WHERE id=$1)
	`, worldID).Scan(&worldExists); err != nil {
		return nil, fmt.Errorf("resolve roleplay canon delta world: %w", err)
	}
	if !worldExists {
		return nil, fmt.Errorf("roleplay canon delta world authority is absent")
	}
	rows, err := r.pool.Query(ctx, `
		SELECT content
		FROM roleplay_canon_events
		WHERE world_id=$1 AND content=ANY($2::text[])
	`, worldID, candidates)
	if err != nil {
		return nil, fmt.Errorf("load exact roleplay canon delta: %w", err)
	}
	defer rows.Close()
	existing := make(map[string]struct{}, len(candidates))
	for rows.Next() {
		var fact string
		if err := rows.Scan(&fact); err != nil {
			return nil, err
		}
		existing[fact] = struct{}{}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	result := make([]string, 0, len(candidates)-len(existing))
	for _, fact := range candidates {
		if _, found := existing[fact]; !found {
			result = append(result, fact)
		}
	}
	return result, nil
}
