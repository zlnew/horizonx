// Package event
package event

import (
	"reflect"

	"horizonx-server/internal/logger"
)

type Handler func(event any)

type Bus struct {
	handlers map[reflect.Type][]Handler
	log      logger.Logger
}

func New(log logger.Logger) *Bus {
	return &Bus{
		handlers: make(map[reflect.Type][]Handler),
		log:      log,
	}
}

func (b *Bus) Subscribe(event any, handler Handler) {
	t := reflect.TypeOf(event)
	b.handlers[t] = append(b.handlers[t], handler)
}

func (b *Bus) Publish(event any) {
	t := reflect.TypeOf(event)
	if handlers, ok := b.handlers[t]; ok {
		for _, h := range handlers {
			func() {
				defer func() {
					if r := recover(); r != nil {
						b.log.Warn(
							"event handler panic",
							"event", t.String(),
							"panic", r,
						)
					}
				}()
				h(event)
			}()
		}
	}
}
