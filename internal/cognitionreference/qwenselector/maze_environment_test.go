package qwenselector_test

import (
	"context"
	"errors"
	"fmt"
	"sort"
)

type cellID string
type routeMarker string

const (
	markerPlain routeMarker = "plain"
	markerQuiet routeMarker = "quiet"
	markerVivid routeMarker = "vivid"
)

var (
	errReferenceMazeInvalid      = errors.New("invalid reference maze")
	errReferenceMazeIllegalMove  = errors.New("illegal reference maze move")
	errReferenceMazeContinuity   = errors.New("reference maze transition continuity failure")
	errReferenceMazeUnresolvable = errors.New("reference maze is unresolvable")
	errReferenceMazeBound        = errors.New("reference maze move bound exceeded")
)

type referenceMazeEdge struct {
	Left  cellID
	Right cellID
}

type referenceMazeSpec struct {
	Start   cellID
	Goal    cellID
	Markers map[cellID]routeMarker
	Edges   []referenceMazeEdge
}

type publicNeighbor struct {
	Cell   cellID
	Marker routeMarker
}

type mazeObservation struct {
	At        cellID
	Marker    routeMarker
	Neighbors []publicNeighbor
	Terminal  bool
	Revision  int
}

type referenceMazeWorld interface {
	Start(context.Context) (mazeObservation, error)
	Move(context.Context, cellID, cellID) (mazeObservation, error)
}

type referenceMazeEnvironment struct {
	spec      referenceMazeSpec
	adjacency map[cellID][]cellID
	current   cellID
	revision  int
}

func newReferenceMazeEnvironment(spec referenceMazeSpec) (*referenceMazeEnvironment, error) {
	if spec.Start == "" || spec.Goal == "" || spec.Markers[spec.Start] == "" || spec.Markers[spec.Goal] == "" {
		return nil, fmt.Errorf("%w: start and terminal cell must be registered", errReferenceMazeInvalid)
	}
	adjacency := make(map[cellID][]cellID, len(spec.Markers))
	for cell, marker := range spec.Markers {
		if cell == "" || !validRouteMarker(marker) {
			return nil, fmt.Errorf("%w: cell %q has invalid public marker", errReferenceMazeInvalid, cell)
		}
		adjacency[cell] = []cellID{}
	}
	seen := make(map[string]struct{}, len(spec.Edges))
	for _, edge := range spec.Edges {
		if edge.Left == edge.Right || spec.Markers[edge.Left] == "" || spec.Markers[edge.Right] == "" {
			return nil, fmt.Errorf("%w: edge %#v has invalid endpoints", errReferenceMazeInvalid, edge)
		}
		left, right := edge.Left, edge.Right
		if right < left {
			left, right = right, left
		}
		key := string(left) + "\x00" + string(right)
		if _, duplicate := seen[key]; duplicate {
			return nil, fmt.Errorf("%w: duplicate edge %#v", errReferenceMazeInvalid, edge)
		}
		seen[key] = struct{}{}
		adjacency[edge.Left] = append(adjacency[edge.Left], edge.Right)
		adjacency[edge.Right] = append(adjacency[edge.Right], edge.Left)
	}
	for cell := range adjacency {
		sort.Slice(adjacency[cell], func(i, j int) bool { return adjacency[cell][i] < adjacency[cell][j] })
	}
	frozen := referenceMazeSpec{
		Start: spec.Start, Goal: spec.Goal,
		Markers: make(map[cellID]routeMarker, len(spec.Markers)),
		Edges:   append([]referenceMazeEdge{}, spec.Edges...),
	}
	for cell, marker := range spec.Markers {
		frozen.Markers[cell] = marker
	}
	return &referenceMazeEnvironment{spec: frozen, adjacency: adjacency, current: frozen.Start}, nil
}

func (environment *referenceMazeEnvironment) Start(ctx context.Context) (mazeObservation, error) {
	if ctx == nil {
		return mazeObservation{}, fmt.Errorf("%w: context is nil", errReferenceMazeInvalid)
	}
	if err := ctx.Err(); err != nil {
		return mazeObservation{}, err
	}
	return environment.observe(), nil
}

func (environment *referenceMazeEnvironment) Move(ctx context.Context, from, to cellID) (mazeObservation, error) {
	if ctx == nil {
		return mazeObservation{}, fmt.Errorf("%w: context is nil", errReferenceMazeIllegalMove)
	}
	if err := ctx.Err(); err != nil {
		return mazeObservation{}, err
	}
	if from != environment.current || !containsCell(environment.adjacency[from], to) {
		return mazeObservation{}, fmt.Errorf("%w: %q to %q at revision %d", errReferenceMazeIllegalMove, from, to, environment.revision)
	}
	environment.current = to
	environment.revision++
	return environment.observe(), nil
}

func (environment *referenceMazeEnvironment) observe() mazeObservation {
	neighbors := make([]publicNeighbor, len(environment.adjacency[environment.current]))
	for index, cell := range environment.adjacency[environment.current] {
		neighbors[index] = publicNeighbor{Cell: cell, Marker: environment.spec.Markers[cell]}
	}
	return mazeObservation{
		At: environment.current, Marker: environment.spec.Markers[environment.current],
		Neighbors: neighbors, Terminal: environment.current == environment.spec.Goal,
		Revision: environment.revision,
	}
}

func generateReferenceMaze(seed uint64) (*referenceMazeEnvironment, error) {
	quietLength := 1 + int(seed%3)
	vividLength := 1 + int((seed/3)%3)
	markers := map[cellID]routeMarker{"s": markerPlain}
	edges := make([]referenceMazeEdge, 0, quietLength+vividLength)
	quietEnd := addReferenceMazeArm(&edges, markers, "s", "q", markerQuiet, quietLength)
	vividEnd := addReferenceMazeArm(&edges, markers, "s", "v", markerVivid, vividLength)
	goal := vividEnd
	if seed%2 == 0 {
		goal = quietEnd
	}
	return newReferenceMazeEnvironment(referenceMazeSpec{Start: "s", Goal: goal, Markers: markers, Edges: edges})
}

func addReferenceMazeArm(edges *[]referenceMazeEdge, markers map[cellID]routeMarker, start cellID, prefix string, marker routeMarker, length int) cellID {
	previous := start
	for index := 0; index < length; index++ {
		current := cellID(fmt.Sprintf("%s%d", prefix, index))
		markers[current] = marker
		*edges = append(*edges, referenceMazeEdge{Left: previous, Right: current})
		previous = current
	}
	return previous
}

func validRouteMarker(marker routeMarker) bool {
	return marker == markerPlain || marker == markerQuiet || marker == markerVivid
}

func containsCell(cells []cellID, wanted cellID) bool {
	for _, cell := range cells {
		if cell == wanted {
			return true
		}
	}
	return false
}
