package queue

import (
	"context"
	"fmt"
	"reflect"

	"github.com/jackc/pgx/v5"
)

type legacyCutoverSequenceState struct {
	OID       uint32
	Schema    string
	Name      string
	LastValue int64
	IsCalled  bool
}

func loadLegacyCutoverSequenceState(
	ctx context.Context,
	tx pgx.Tx,
	runtimeSchema string,
) ([]legacyCutoverSequenceState, error) {
	rows, err := tx.Query(ctx, `
		SELECT sequences.oid,namespaces.nspname,sequences.relname
		FROM pg_class sequences
		JOIN pg_namespace namespaces ON namespaces.oid=sequences.relnamespace
		WHERE sequences.relkind='S' AND namespaces.nspname=$1
		ORDER BY sequences.oid
	`, runtimeSchema)
	if err != nil {
		return nil, fmt.Errorf("enumerate legacy cutover sequence state: %w", err)
	}
	states := make([]legacyCutoverSequenceState, 0, legacyExpectedSequenceCount)
	for rows.Next() {
		var state legacyCutoverSequenceState
		if err := rows.Scan(&state.OID, &state.Schema, &state.Name); err != nil {
			rows.Close()
			return nil, fmt.Errorf("scan legacy cutover sequence identity: %w", err)
		}
		states = append(states, state)
	}
	iterationErr := rows.Err()
	rows.Close()
	if iterationErr != nil {
		return nil, fmt.Errorf("iterate legacy cutover sequence identities: %w", iterationErr)
	}
	for index := range states {
		identifier := pgx.Identifier{states[index].Schema, states[index].Name}.Sanitize()
		if err := tx.QueryRow(ctx, `SELECT last_value,is_called FROM `+identifier).Scan(
			&states[index].LastValue, &states[index].IsCalled,
		); err != nil {
			return nil, fmt.Errorf("load legacy cutover sequence %s state: %w", identifier, err)
		}
	}
	return states, nil
}

func restoreLegacyCutoverSequenceState(
	ctx context.Context,
	tx pgx.Tx,
	runtimeSchema string,
	want []legacyCutoverSequenceState,
) error {
	got, err := loadLegacyCutoverSequenceState(ctx, tx, runtimeSchema)
	if err != nil {
		return err
	}
	if reflect.DeepEqual(got, want) {
		return nil
	}
	if len(got) != len(want) {
		return fmt.Errorf(
			"legacy cutover current-tail proof changed sequence identity count from %d to %d",
			len(want), len(got),
		)
	}
	for index := range want {
		if got[index].OID != want[index].OID || got[index].Schema != want[index].Schema ||
			got[index].Name != want[index].Name {
			return fmt.Errorf("legacy cutover current-tail proof changed sequence identity")
		}
		if got[index].LastValue == want[index].LastValue &&
			got[index].IsCalled == want[index].IsCalled {
			continue
		}
		if _, err := tx.Exec(ctx, `SELECT pg_catalog.setval(
			$1::oid::pg_catalog.regclass,$2,$3
		)`, want[index].OID, want[index].LastValue, want[index].IsCalled); err != nil {
			return fmt.Errorf(
				"restore legacy cutover sequence %q state: %w", want[index].Name, err,
			)
		}
	}
	restored, err := loadLegacyCutoverSequenceState(ctx, tx, runtimeSchema)
	if err != nil {
		return err
	}
	if !reflect.DeepEqual(restored, want) {
		return fmt.Errorf("legacy cutover current-tail proof did not restore exact sequence state")
	}
	return nil
}
