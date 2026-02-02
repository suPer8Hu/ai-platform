package ai

import (
	"context"
	"fmt"
	"strings"
	"sync"
)

type ProviderFactory func(ctx context.Context, model string) (Provider, error)

type Registry struct {
	mu        sync.RWMutex
	factories map[string]ProviderFactory
}

func NewRegistry() *Registry {
	return &Registry{factories: make(map[string]ProviderFactory)}
}

func (r *Registry) Register(name string, f ProviderFactory) {
	name = strings.ToLower(strings.TrimSpace(name))
	r.mu.Lock()
	defer r.mu.Unlock()
	r.factories[name] = f
}

func (r *Registry) Get(ctx context.Context, name string, model string) (Provider, error) {
	name = strings.ToLower(strings.TrimSpace(name))
	r.mu.RLock()
	f, ok := r.factories[name]
	r.mu.RUnlock()
	if !ok {
		return nil, fmt.Errorf("unknown ai provider: %s", name)
	}
	return f(ctx, model)
}
