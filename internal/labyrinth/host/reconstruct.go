package host

import (
	"bytes"
	"context"
	"fmt"
	"reflect"
	"strings"

	"github.com/gryph/omnidex/internal/cognition"
	"github.com/gryph/omnidex/internal/labyrinth"
)

func reconstructCandidate(
	ctx context.Context,
	environment *Environment,
	scenario labyrinth.Scenario,
	episode storedEpisode,
	history []storedAction,
) (kernelCandidate, error) {
	candidate, err := environment.newKernel(scenario, episode.Episode, historicalAuthorizer)
	if err != nil {
		return kernelCandidate{}, err
	}
	started, err := candidate.Start(ctx, episode.Scenario)
	if err != nil {
		_ = candidate.Close()
		return kernelCandidate{}, err
	}
	if err := requireExactTransition(started, episode.Start); err != nil {
		_ = candidate.Close()
		return kernelCandidate{}, fmt.Errorf("%w: sealed start diverged: %v", ErrReceiptCorrupt, err)
	}
	current := started.Current
	terminal := started.Terminal
	if episode.ReceiptCount < int64(len(history)) || episode.ReceiptCount > labyrinth.MaxEpisodeTransitions {
		_ = candidate.Close()
		return kernelCandidate{}, fmt.Errorf("%w: durable receipt count is inconsistent", ErrReceiptCorrupt)
	}
	for index, stored := range history {
		receipt := stored.Receipt
		if receipt.Expected != current || receipt.Transition == nil || receipt.Failure != nil {
			_ = candidate.Close()
			return kernelCandidate{}, fmt.Errorf("%w: history entry %d is not consecutive", ErrReceiptCorrupt, index)
		}
		actual, applyErr := candidate.Apply(ctx, episode.Episode, current, receipt.Action)
		if applyErr != nil {
			_ = candidate.Close()
			return kernelCandidate{}, fmt.Errorf("%w: history entry %d no longer applies: %v", ErrReceiptCorrupt, index, applyErr)
		}
		if err := requireExactTransition(actual, *receipt.Transition); err != nil {
			_ = candidate.Close()
			return kernelCandidate{}, fmt.Errorf("%w: history entry %d diverged: %v", ErrReceiptCorrupt, index, err)
		}
		current = actual.Current
		terminal = actual.Terminal
	}
	if current != episode.Current || terminal != episode.Terminal ||
		uint64(len(history))+1 != episode.Current.Number {
		_ = candidate.Close()
		return kernelCandidate{}, fmt.Errorf("%w: reconstructed head does not equal the durable episode head", ErrReceiptCorrupt)
	}
	return candidate, nil
}

func requireExactTransition(actual, expected cognition.Transition) error {
	actualRaw, actualDigest, err := encodeExact(actual)
	if err != nil {
		return err
	}
	expectedRaw, expectedDigest, err := encodeExact(expected)
	if err != nil {
		return err
	}
	if actualDigest != expectedDigest || !bytes.Equal(actualRaw, expectedRaw) {
		changed := make([]string, 0, 8)
		if actual.ActionID != expected.ActionID {
			changed = append(changed, "action_id")
		}
		if !reflect.DeepEqual(actual.Previous, expected.Previous) {
			changed = append(changed, "previous")
		}
		if actual.Current != expected.Current {
			changed = append(changed, "current")
		}
		if !reflect.DeepEqual(actual.Observations, expected.Observations) {
			changed = append(changed, "observations")
		}
		if !reflect.DeepEqual(actual.Effects, expected.Effects) {
			changed = append(changed, "effects")
		}
		if actual.Cost != expected.Cost {
			changed = append(changed, "cost")
		}
		if actual.Terminal != expected.Terminal {
			changed = append(changed, "terminal")
		}
		if actual.PublicOutcome != expected.PublicOutcome {
			changed = append(changed, "public_outcome")
		}
		return fmt.Errorf("transition content differs in %s", strings.Join(changed, ","))
	}
	return nil
}
