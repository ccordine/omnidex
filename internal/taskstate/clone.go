package taskstate

func cloneInt64(value *int64) *int64 {
	if value == nil {
		return nil
	}
	copy := *value
	return &copy
}

func int64Pointer(value int64) *int64 { return cloneInt64(&value) }

func cloneFloat64(value *float64) *float64 {
	if value == nil {
		return nil
	}
	copy := *value
	return &copy
}

func cloneJSONObject(value JSONObject) JSONObject {
	return JSONObject{raw: value.Bytes()}
}

func cloneRefs(refs []Ref) []Ref {
	if refs == nil {
		return nil
	}
	result := make([]Ref, len(refs))
	copy(result, refs)
	return result
}

func normalizedRefs(refs []Ref) []Ref {
	if refs == nil {
		return make([]Ref, 0)
	}
	return cloneRefs(refs)
}

func cloneStrings(values []string) []string {
	if values == nil {
		return nil
	}
	result := make([]string, len(values))
	copy(result, values)
	return result
}

func normalizedStrings(values []string) []string {
	if values == nil {
		return make([]string, 0)
	}
	return cloneStrings(values)
}

func cloneNodeIDs(values []NodeID) []NodeID {
	result := make([]NodeID, len(values))
	copy(result, values)
	return result
}

func cloneNode(node Node) Node {
	node.AssignedStepID = cloneInt64(node.AssignedStepID)
	node.CreatedStepID = cloneInt64(node.CreatedStepID)
	node.CompletedStepID = cloneInt64(node.CompletedStepID)
	node.VerificationRefs = cloneRefs(node.VerificationRefs)
	node.AcceptanceCriteria = cloneStrings(node.AcceptanceCriteria)
	node.Metadata = cloneJSONObject(node.Metadata)
	return node
}

func cloneEntry(entry Entry) Entry {
	entry.Confidence = cloneFloat64(entry.Confidence)
	entry.CreatedStepID = cloneInt64(entry.CreatedStepID)
	entry.Metadata = cloneJSONObject(entry.Metadata)
	entry.Refs = cloneRefs(entry.Refs)
	return entry
}

func cloneEvent(event Event) Event {
	event.StepID = cloneInt64(event.StepID)
	if event.Node != nil {
		node := cloneNode(*event.Node)
		event.Node = &node
	}
	if event.Edge != nil {
		edge := *event.Edge
		event.Edge = &edge
	}
	if event.Entry != nil {
		entry := cloneEntry(*event.Entry)
		event.Entry = &entry
	}
	event.NodeIDs = cloneNodeIDs(event.NodeIDs)
	event.VerificationRefs = cloneRefs(event.VerificationRefs)
	return event
}

func equalInt64Pointers(left, right *int64) bool {
	if left == nil || right == nil {
		return left == nil && right == nil
	}
	return *left == *right
}

func equalFloat64Pointers(left, right *float64) bool {
	if left == nil || right == nil {
		return left == nil && right == nil
	}
	return *left == *right
}
