package provider

import (
	"fmt"
	"sync"

	"github.com/picklr-io/picklr/pkg/proto/provider"
	"github.com/picklr-io/picklr/providers/docker"
	"github.com/picklr-io/picklr/providers/null"
)

// Registry manages the lifecycle of providers.
type Registry struct {
	mu        sync.RWMutex
	providers map[string]provider.ProviderServer
}

func NewRegistry() *Registry {
	return &Registry{
		providers: make(map[string]provider.ProviderServer),
	}
}

// LoadProvider initializes and registers a provider.
// For MVP, we only support built-in providers like "null".
// In the future, this would load plugins via go-plugin.
func (r *Registry) LoadProvider(name string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.providers[name]; exists {
		return nil
	}

	var p provider.ProviderServer
	switch name {
	case "null":
		p = null.New()
	case "docker":
		p = docker.New()
	default:
		return fmt.Errorf("unknown provider: %s", name)
	}

	r.providers[name] = p
	return nil
}

// Get returns a registered provider.
func (r *Registry) Get(name string) (provider.ProviderServer, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	p, ok := r.providers[name]
	if !ok {
		return nil, fmt.Errorf("provider not loaded: %s", name)
	}
	return p, nil
}
