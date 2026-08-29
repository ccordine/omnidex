package roleplay

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
)

func TestSimulationDefinitionCapacityIsSerializedPerWorld(t *testing.T) {
	pool, _ := openRoleplayTestPool(t)
	installSimulationTestSchema(t, pool)
	world, viewpoint := bootstrapRoleplayChannel(t, pool, "simulation-capacity", "Capacity world", "Keeper")
	store, err := NewStore(pool)
	if err != nil {
		t.Fatal(err)
	}
	writeTestPersona(t, store, viewpoint.ID, "Keeps the gauges.")

	const attempts = MaxSimulationMeters + 8
	errorsByAttempt := make(chan error, attempts)
	var group sync.WaitGroup
	for index := 0; index < attempts; index++ {
		index := index
		group.Add(1)
		go func() {
			defer group.Done()
			errorsByAttempt <- store.RegisterMeter(context.Background(), MeterDefinition{
				WorldID: world.ID, Key: fmt.Sprintf("meter-%02d", index),
				Name: fmt.Sprintf("Meter %02d", index), Minimum: 0, Maximum: 10, InitialValue: 5,
			})
		}()
	}
	group.Wait()
	close(errorsByAttempt)
	succeeded := 0
	conflicted := 0
	for err := range errorsByAttempt {
		switch {
		case err == nil:
			succeeded++
		case errors.Is(err, ErrSimulationConflict):
			conflicted++
		default:
			t.Fatalf("unexpected registration error: %v", err)
		}
	}
	if succeeded != MaxSimulationMeters || conflicted != attempts-MaxSimulationMeters {
		t.Fatalf("succeeded=%d conflicted=%d", succeeded, conflicted)
	}
	var persisted int
	if err := pool.QueryRow(context.Background(), `
		SELECT COUNT(*) FROM roleplay_meter_definitions WHERE world_id=$1
	`, world.ID).Scan(&persisted); err != nil {
		t.Fatal(err)
	}
	if persisted != MaxSimulationMeters {
		t.Fatalf("persisted definitions=%d", persisted)
	}
}
