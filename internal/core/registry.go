package core

import (
	"context"
	"sync"
)

type Collector interface {
	Collect(ctx context.Context) (any, error)
}

type Registry struct {
	mu         sync.RWMutex
	collectors map[string]Collector
	snapshot   Metrics
}

func NewRegistry() *Registry {
	return &Registry{
		collectors: make(map[string]Collector),
	}
}

func (r *Registry) Register(name string, c Collector) {
	r.mu.Lock()
	r.collectors[name] = c
	r.mu.Unlock()
}

func (r *Registry) Update(name string, value any) {
	r.mu.Lock()
	switch name {
	case "cpu":
		r.snapshot.CPU = value.(CPUMetric)
	case "gpu":
		r.snapshot.GPU = value.(GPUMetric)
	case "memory":
		r.snapshot.Memory = value.(MemoryMetric)
	case "disk":
		r.snapshot.Disk = value.(DiskMetric)
	}
	r.mu.Unlock()
}

func (r *Registry) Snapshot() Metrics {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.snapshot
}
