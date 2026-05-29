package secrets

import (
	"context"
	"sync"
)

type MemoryStore struct {
	mu     sync.Mutex
	Values map[string]string
}

func (m *MemoryStore) GetAPISecrets(_ context.Context) (map[string]string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := map[string]string{}
	for key, value := range m.Values {
		out[key] = value
	}
	return out, nil
}

func (m *MemoryStore) SetAPISecrets(_ context.Context, updates map[string]string, clearKeys []string) (map[string]string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.Values = MergeStored(m.Values, updates, clearKeys)
	out := map[string]string{}
	for key, value := range m.Values {
		out[key] = value
	}
	return out, nil
}
