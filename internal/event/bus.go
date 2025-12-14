// Package event
package event

import "sync"

type Handler func(event any)

type Bus struct {
	mu       sync.RWMutex
	handlers map[string][]Handler
}

func New() *Bus {
	return &Bus{
		handlers: make(map[string][]Handler),
	}
}

func (b *Bus) Subscribe(eventName string, handler Handler) {
	b.mu.Lock()
	defer b.mu.Unlock()

	b.handlers[eventName] = append(b.handlers[eventName], handler)
}

func (b *Bus) Publish(eventName string, event any) {
	b.mu.RLock()
	handlers := b.handlers[eventName]
	b.mu.RUnlock()

	for _, h := range handlers {
		h(event)
	}
}
