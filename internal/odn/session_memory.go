package odn

import "strings"

func rememberCapabilityMemoriesFromObservations(session *Session, observations []StructuredCommandObservation) []SessionMemory {
	if session == nil {
		return nil
	}
	stored := []SessionMemory{}
	for _, obs := range observations {
		content := strings.TrimSpace(obs.CapabilityMemory)
		if content == "" {
			continue
		}
		if sessionHasMemoryContent(session, content) {
			continue
		}
		memory := SessionMemory{
			Kind:      "capability",
			Content:   content,
			Tags:      []string{"capability", "self-correction", "realtime-evidence"},
			CreatedAt: nowUTC(),
		}
		session.Memories = append(session.Memories, memory)
		stored = append(stored, memory)
	}
	return stored
}

func sessionHasMemoryContent(session *Session, content string) bool {
	if session == nil {
		return false
	}
	content = strings.TrimSpace(content)
	for _, memory := range session.Memories {
		if strings.TrimSpace(memory.Content) == content {
			return true
		}
	}
	return false
}
