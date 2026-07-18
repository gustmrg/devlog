package collector

import (
	"context"

	"devlog/internal/domain"
)

// Collector is the stable extension point for local and remote event sources.
// A future GitLab implementation should only need to satisfy this interface.
type Collector interface {
	Type() string
	Validate() error
	Collect(context.Context, string) ([]domain.Event, string, error)
}

type Registry struct {
	collectors map[string]Collector
}

func NewRegistry(collectors ...Collector) *Registry {
	r := &Registry{collectors: make(map[string]Collector, len(collectors))}
	for _, c := range collectors {
		r.collectors[c.Type()] = c
	}
	return r
}

func (r *Registry) Get(sourceType string) (Collector, bool) {
	c, ok := r.collectors[sourceType]
	return c, ok
}
