package connectorsdk

import (
	"sort"
	"sync"
)

type Descriptor struct {
	ID           string       `json:"id"`
	DisplayName  string       `json:"display_name"`
	Capabilities Capabilities `json:"capabilities"`
}

type Registry struct {
	mu         sync.RWMutex
	connectors map[string]Connector
}

func NewRegistry() *Registry {
	return &Registry{connectors: make(map[string]Connector)}
}

func (r *Registry) Register(connector Connector) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.connectors[connector.ID()] = connector
}

func (r *Registry) Get(id string) (Connector, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	connector, ok := r.connectors[id]
	return connector, ok
}

func (r *Registry) List() []Descriptor {
	r.mu.RLock()
	defer r.mu.RUnlock()

	out := make([]Descriptor, 0, len(r.connectors))
	for _, connector := range r.connectors {
		out = append(out, Descriptor{
			ID:           connector.ID(),
			DisplayName:  connector.DisplayName(),
			Capabilities: connector.Capabilities(),
		})
	}

	sort.Slice(out, func(i, j int) bool {
		return out[i].ID < out[j].ID
	})

	return out
}
