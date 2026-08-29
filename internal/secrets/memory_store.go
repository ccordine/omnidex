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
	if err := ValidateStored(out); err != nil {
		return nil, err
	}
	return out, nil
}

func (m *MemoryStore) SetAPISecrets(_ context.Context, updates map[string]string, clearKeys []string) (map[string]string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	merged, err := MergeStored(m.Values, updates, clearKeys)
	if err != nil {
		return nil, err
	}
	m.Values = merged
	out := map[string]string{}
	for key, value := range m.Values {
		out[key] = value
	}
	return out, nil
}
