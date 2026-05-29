package secrets

import (
	"context"
	"strings"
	"sync"
)

type Store interface {
	GetAPISecrets(ctx context.Context) (map[string]string, error)
}

var (
	globalMu sync.RWMutex
	global   *Resolver
)

type Resolver struct {
	store Store
	mu    sync.RWMutex
	cache map[string]string
}

func NewResolver(store Store) *Resolver {
	return &Resolver{store: store}
}

func SetGlobal(resolver *Resolver) {
	globalMu.Lock()
	global = resolver
	globalMu.Unlock()
}

func Global() *Resolver {
	globalMu.RLock()
	defer globalMu.RUnlock()
	return global
}

func Lookup(key string) string {
	resolver := Global()
	if resolver == nil {
		return lookupFieldEnv(key)
	}
	return resolver.Get(context.Background(), key)
}

func (r *Resolver) Invalidate() {
	if r == nil {
		return
	}
	r.mu.Lock()
	r.cache = nil
	r.mu.Unlock()
}

func (r *Resolver) Get(ctx context.Context, key string) string {
	if r == nil {
		return lookupFieldEnv(key)
	}
	r.mu.RLock()
	if r.cache != nil {
		if value := strings.TrimSpace(r.cache[key]); value != "" {
			r.mu.RUnlock()
			return value
		}
	}
	r.mu.RUnlock()

	stored := map[string]string{}
	if r.store != nil {
		if values, err := r.store.GetAPISecrets(ctx); err == nil {
			stored = values
		}
	}
	r.mu.Lock()
	r.cache = stored
	r.mu.Unlock()

	if value := strings.TrimSpace(stored[key]); value != "" {
		return value
	}
	return lookupFieldEnv(key)
}

func (r *Resolver) RawStored(ctx context.Context) map[string]string {
	if r == nil || r.store == nil {
		return map[string]string{}
	}
	values, err := r.store.GetAPISecrets(ctx)
	if err != nil {
		return map[string]string{}
	}
	return values
}

func (r *Resolver) Snapshot(ctx context.Context) map[string]string {
	out := map[string]string{}
	for _, field := range Fields {
		if value := r.Get(ctx, field.Key); value != "" {
			out[field.Key] = value
		}
	}
	return out
}

func lookupFieldEnv(key string) string {
	for _, field := range Fields {
		if field.Key == key {
			return LookupEnv(field.EnvKeys)
		}
	}
	return ""
}
