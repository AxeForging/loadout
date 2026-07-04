package adapter

import "github.com/AxeForging/loadout/internal/domain"

// Registry holds the registered adapters in registration order.
type Registry struct {
	order []domain.AgentID
	byID  map[domain.AgentID]Adapter
}

func NewRegistry() *Registry {
	return &Registry{byID: map[domain.AgentID]Adapter{}}
}

// Register adds an adapter; registering the same ID twice panics because it
// is always a programming error.
func (r *Registry) Register(a Adapter) {
	if _, dup := r.byID[a.ID()]; dup {
		panic("adapter registered twice: " + string(a.ID()))
	}
	r.byID[a.ID()] = a
	r.order = append(r.order, a.ID())
}

func (r *Registry) Get(id domain.AgentID) (Adapter, bool) {
	a, ok := r.byID[id]
	return a, ok
}

// All returns adapters in registration order.
func (r *Registry) All() []Adapter {
	out := make([]Adapter, 0, len(r.order))
	for _, id := range r.order {
		out = append(out, r.byID[id])
	}
	return out
}
