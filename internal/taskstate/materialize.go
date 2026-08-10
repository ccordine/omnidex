package taskstate

import (
	"sort"
)

type MaterializedState struct {
	ID                LedgerID                     `json:"id"`
	Owner             LedgerOwner                  `json:"owner"`
	Version           uint64                       `json:"version"`
	Status            LedgerStatus                 `json:"status"`
	Nodes             []Node                       `json:"nodes"`
	Edges             []Edge                       `json:"edges"`
	Entries           []Entry                      `json:"entries"`
	NodeSupersessions []NodeGenerationSupersession `json:"node_supersessions"`
}

func (ledger *Ledger) ID() LedgerID {
	if ledger == nil {
		return ""
	}
	return ledger.id
}

func (ledger *Ledger) Owner() LedgerOwner {
	if ledger == nil {
		return LedgerOwner{}
	}
	return ledger.owner
}

func (ledger *Ledger) Version() uint64 {
	if ledger == nil {
		return 0
	}
	return ledger.version
}

func (ledger *Ledger) Status() LedgerStatus {
	if ledger == nil {
		return ""
	}
	return ledger.status
}

func (ledger *Ledger) Node(id NodeID) (Node, bool) {
	if ledger == nil {
		return Node{}, false
	}
	node, ok := ledger.nodes[id]
	return cloneNode(node), ok
}

func (ledger *Ledger) Entry(id EntryID) (Entry, bool) {
	if ledger == nil {
		return Entry{}, false
	}
	entry, ok := ledger.entries[id]
	return cloneEntry(entry), ok
}

func (ledger *Ledger) Nodes() []Node {
	if ledger == nil {
		return nil
	}
	nodes := make([]Node, 0, len(ledger.nodes))
	for _, node := range ledger.nodes {
		nodes = append(nodes, cloneNode(node))
	}
	sort.Slice(nodes, func(i, j int) bool { return nodes[i].ID < nodes[j].ID })
	return nodes
}

func (ledger *Ledger) Edges() []Edge {
	if ledger == nil {
		return nil
	}
	edges := make([]Edge, 0, len(ledger.edges))
	for _, edge := range ledger.edges {
		edges = append(edges, edge)
	}
	sort.Slice(edges, func(i, j int) bool { return edges[i].ID < edges[j].ID })
	return edges
}

func (ledger *Ledger) Entries() []Entry {
	if ledger == nil {
		return nil
	}
	entries := make([]Entry, 0, len(ledger.entries))
	for _, entry := range ledger.entries {
		entries = append(entries, cloneEntry(entry))
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].ID < entries[j].ID })
	return entries
}

func (ledger *Ledger) NodeSupersessions() []NodeGenerationSupersession {
	if ledger == nil {
		return nil
	}
	values := make([]NodeGenerationSupersession, 0, len(ledger.nodeSupersessions))
	for _, value := range ledger.nodeSupersessions {
		values = append(values, value)
	}
	sort.Slice(values, func(i, j int) bool { return values[i].NodeID < values[j].NodeID })
	return values
}

func (ledger *Ledger) NodeSupersession(id NodeID) (NodeGenerationSupersession, bool) {
	if ledger == nil {
		return NodeGenerationSupersession{}, false
	}
	value, ok := ledger.nodeSupersessions[id]
	return value, ok
}

func (ledger *Ledger) Events() []Event {
	if ledger == nil {
		return nil
	}
	events := make([]Event, len(ledger.events))
	for index, event := range ledger.events {
		events[index] = cloneEvent(event)
	}
	return events
}

func (ledger *Ledger) MaterializedState() MaterializedState {
	if ledger == nil {
		return MaterializedState{}
	}
	return MaterializedState{
		ID: ledger.id, Owner: ledger.owner, Version: ledger.version, Status: ledger.status,
		Nodes: ledger.Nodes(), Edges: ledger.Edges(), Entries: ledger.Entries(),
		NodeSupersessions: ledger.NodeSupersessions(),
	}
}

func (ledger *Ledger) NextRunnableNode() (Node, bool) {
	if ledger == nil || ledger.status != LedgerActive {
		return Node{}, false
	}
	ready := make([]Node, 0)
	for _, node := range ledger.nodes {
		_, superseded := ledger.nodeSupersessions[node.ID]
		if !superseded && node.Status == NodeReady && executableNode(node.Kind) {
			ready = append(ready, node)
		}
	}
	sort.Slice(ready, func(i, j int) bool {
		if ready[i].Priority != ready[j].Priority {
			return ready[i].Priority > ready[j].Priority
		}
		if ready[i].CreatedVersion != ready[j].CreatedVersion {
			return ready[i].CreatedVersion < ready[j].CreatedVersion
		}
		return ready[i].ID < ready[j].ID
	})
	if len(ready) == 0 {
		return Node{}, false
	}
	return cloneNode(ready[0]), true
}
