package qwenselector_test

import (
	"context"
	"fmt"
	"sort"
)

type mazeMoveKind string

const (
	mazeMoveForward   mazeMoveKind = "forward"
	mazeMoveBacktrack mazeMoveKind = "backtrack"
)

type mazeMove struct {
	From cellID
	To   cellID
	Kind mazeMoveKind
}

type routePreferenceFact struct {
	GapID       GapID
	CandidateID CandidateID
	Marker      routeMarker
}

type referenceMazeResult struct {
	Complete        bool
	Moves           []mazeMove
	DeadEnds        map[cellID]struct{}
	PreferenceFacts []routePreferenceFact
	SelectorCalls   int
}

type referenceMazeMemory struct {
	visited    map[cellID]struct{}
	parent     map[cellID]cellID
	known      map[cellID]mazeObservation
	deadEnds   map[cellID]struct{}
	preference *routePreferenceFact
}

func runReferenceMaze(ctx context.Context, world referenceMazeWorld, selector Selector, maxMoves int) (referenceMazeResult, error) {
	result := referenceMazeResult{Moves: []mazeMove{}, DeadEnds: make(map[cellID]struct{}), PreferenceFacts: []routePreferenceFact{}}
	if ctx == nil || world == nil || maxMoves < 1 {
		return result, fmt.Errorf("%w: runner inputs are invalid", errReferenceMazeInvalid)
	}
	if err := ctx.Err(); err != nil {
		return result, err
	}
	observation, err := world.Start(ctx)
	if err != nil {
		return result, err
	}
	if err := ctx.Err(); err != nil {
		return result, err
	}
	if observation.Revision != 0 {
		return result, fmt.Errorf("%w: initial revision is %d, want 0", errReferenceMazeContinuity, observation.Revision)
	}
	if err := validateMazeObservation(observation); err != nil {
		return result, err
	}
	memory := referenceMazeMemory{
		visited: make(map[cellID]struct{}), parent: make(map[cellID]cellID),
		known: make(map[cellID]mazeObservation), deadEnds: make(map[cellID]struct{}),
	}
	memory.known[observation.At] = cloneMazeObservation(observation)
	for {
		memory.visited[observation.At] = struct{}{}
		if observation.Terminal {
			result.Complete = true
			result.DeadEnds = cloneCellSet(memory.deadEnds)
			return result, nil
		}
		if len(result.Moves) >= maxMoves {
			return result, fmt.Errorf("%w: exceeded %d moves", errReferenceMazeBound, maxMoves)
		}
		unvisited := mazeUnvisitedNeighbors(observation, memory.visited)
		var next cellID
		kind := mazeMoveForward
		switch {
		case len(unvisited) == 0:
			memory.deadEnds[observation.At] = struct{}{}
			parent, exists := memory.parent[observation.At]
			if !exists {
				return result, fmt.Errorf("%w: exhausted public topology", errReferenceMazeUnresolvable)
			}
			if !observationHasNeighbor(observation, parent) {
				return result, fmt.Errorf(
					"%w: parent %q is absent from current public neighbors",
					errReferenceMazeContinuity, parent,
				)
			}
			next, kind = parent, mazeMoveBacktrack
		case len(unvisited) == 1:
			next = unvisited[0].Cell
		default:
			if memory.preference == nil && isGenuineSemanticFork(unvisited) {
				gap := referenceMazeGap()
				selected, selectionErr := SelectCandidate(ctx, selector, gap)
				if selectionErr != nil {
					return result, selectionErr
				}
				preference, preferenceErr := materializeRoutePreference(gap.ID, selected)
				if preferenceErr != nil {
					return result, preferenceErr
				}
				memory.preference = &preference
				result.PreferenceFacts = append(result.PreferenceFacts, preference)
				result.SelectorCalls++
			}
			next = chooseMazeNeighbor(unvisited, memory.preference)
		}
		if kind == mazeMoveForward {
			memory.parent[next] = observation.At
		}
		prior := observation.At
		previousRevision := observation.Revision
		expectedMarker, exists := observationNeighborMarker(observation, next)
		if !exists {
			return result, fmt.Errorf(
				"%w: requested destination %q is absent from public neighbors",
				errReferenceMazeContinuity, next,
			)
		}
		if err := ctx.Err(); err != nil {
			return result, err
		}
		observation, err = world.Move(ctx, prior, next)
		if err != nil {
			return result, err
		}
		if err := ctx.Err(); err != nil {
			return result, err
		}
		if observation.At != next || observation.Revision != previousRevision+1 {
			return result, fmt.Errorf(
				"%w: requested %q at revision %d, received %q at revision %d",
				errReferenceMazeContinuity, next, previousRevision+1,
				observation.At, observation.Revision,
			)
		}
		if err := validateMazeObservation(observation); err != nil {
			return result, err
		}
		reverseMarker, hasReverse := observationNeighborMarker(observation, prior)
		if observation.Marker != expectedMarker || !hasReverse || reverseMarker != memory.known[prior].Marker {
			return result, fmt.Errorf(
				"%w: destination %q does not preserve its public marker or reverse edge",
				errReferenceMazeContinuity, next,
			)
		}
		if known, seen := memory.known[observation.At]; seen {
			if !sameStaticMazeSurface(known, observation) {
				return result, fmt.Errorf(
					"%w: cell %q public surface changed on revisit",
					errReferenceMazeContinuity, observation.At,
				)
			}
		} else {
			memory.known[observation.At] = cloneMazeObservation(observation)
		}
		result.Moves = append(result.Moves, mazeMove{From: prior, To: next, Kind: kind})
	}
}

func referenceMazeGap() SemanticGap {
	return SemanticGap{
		ID: "gap.route-style", Kind: GapCandidateSelection,
		ObjectiveID: "objective.reference-navigation",
		Question:    "Which persistent route style best fits this unresolved public choice?",
		Evidence: []SemanticEvidence{{
			ID: "E10", Content: "At least one unexplored route is quiet-marked and at least one is vivid-marked; both are legal and neither has code-owned priority.",
		}},
		Candidates: []SemanticCandidate{
			{ID: "C17", Summary: "Prefer quiet public markers when choices are otherwise unresolved.", EvidenceIDs: []EvidenceID{"E10"}},
			{ID: "C23", Summary: "Prefer vivid public markers when choices are otherwise unresolved.", EvidenceIDs: []EvidenceID{"E10"}},
		},
	}
}

func materializeRoutePreference(gapID GapID, selected CandidateID) (routePreferenceFact, error) {
	switch selected {
	case "C17":
		return routePreferenceFact{GapID: gapID, CandidateID: selected, Marker: markerQuiet}, nil
	case "C23":
		return routePreferenceFact{GapID: gapID, CandidateID: selected, Marker: markerVivid}, nil
	default:
		return routePreferenceFact{}, fmt.Errorf("%w: route preference %q", ErrInvalidSelection, selected)
	}
}

func isGenuineSemanticFork(neighbors []publicNeighbor) bool {
	hasQuiet, hasVivid := false, false
	for _, neighbor := range neighbors {
		hasQuiet = hasQuiet || neighbor.Marker == markerQuiet
		hasVivid = hasVivid || neighbor.Marker == markerVivid
	}
	return hasQuiet && hasVivid
}

func chooseMazeNeighbor(neighbors []publicNeighbor, preference *routePreferenceFact) cellID {
	candidates := append([]publicNeighbor{}, neighbors...)
	sort.Slice(candidates, func(left, right int) bool {
		leftPreferred := preference != nil && candidates[left].Marker == preference.Marker
		rightPreferred := preference != nil && candidates[right].Marker == preference.Marker
		if leftPreferred != rightPreferred {
			return leftPreferred
		}
		return candidates[left].Cell < candidates[right].Cell
	})
	return candidates[0].Cell
}

func mazeUnvisitedNeighbors(observation mazeObservation, visited map[cellID]struct{}) []publicNeighbor {
	result := make([]publicNeighbor, 0, len(observation.Neighbors))
	for _, neighbor := range observation.Neighbors {
		if _, seen := visited[neighbor.Cell]; !seen {
			result = append(result, neighbor)
		}
	}
	return result
}

func validateMazeObservation(observation mazeObservation) error {
	if observation.At == "" || !validRouteMarker(observation.Marker) || observation.Revision < 0 {
		return fmt.Errorf("%w: invalid public observation", errReferenceMazeInvalid)
	}
	seen := make(map[cellID]struct{}, len(observation.Neighbors))
	for _, neighbor := range observation.Neighbors {
		if neighbor.Cell == "" || neighbor.Cell == observation.At || !validRouteMarker(neighbor.Marker) {
			return fmt.Errorf("%w: invalid public neighbor", errReferenceMazeInvalid)
		}
		if _, duplicate := seen[neighbor.Cell]; duplicate {
			return fmt.Errorf("%w: duplicate public neighbor %q", errReferenceMazeInvalid, neighbor.Cell)
		}
		seen[neighbor.Cell] = struct{}{}
	}
	return nil
}

func observationHasNeighbor(observation mazeObservation, wanted cellID) bool {
	_, exists := observationNeighborMarker(observation, wanted)
	return exists
}

func observationNeighborMarker(observation mazeObservation, wanted cellID) (routeMarker, bool) {
	for _, neighbor := range observation.Neighbors {
		if neighbor.Cell == wanted {
			return neighbor.Marker, true
		}
	}
	return "", false
}

func sameStaticMazeSurface(left, right mazeObservation) bool {
	if left.At != right.At || left.Marker != right.Marker || left.Terminal != right.Terminal ||
		len(left.Neighbors) != len(right.Neighbors) {
		return false
	}
	leftNeighbors := append([]publicNeighbor{}, left.Neighbors...)
	rightNeighbors := append([]publicNeighbor{}, right.Neighbors...)
	sort.Slice(leftNeighbors, func(i, j int) bool { return leftNeighbors[i].Cell < leftNeighbors[j].Cell })
	sort.Slice(rightNeighbors, func(i, j int) bool { return rightNeighbors[i].Cell < rightNeighbors[j].Cell })
	for index := range leftNeighbors {
		if leftNeighbors[index] != rightNeighbors[index] {
			return false
		}
	}
	return true
}

func cloneMazeObservation(observation mazeObservation) mazeObservation {
	observation.Neighbors = append([]publicNeighbor{}, observation.Neighbors...)
	return observation
}

func cloneCellSet(source map[cellID]struct{}) map[cellID]struct{} {
	cloned := make(map[cellID]struct{}, len(source))
	for cell := range source {
		cloned[cell] = struct{}{}
	}
	return cloned
}
