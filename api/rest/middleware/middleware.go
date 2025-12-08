// Package middleware
package middleware

import "net/http"

type Middleware func(http.Handler) http.Handler

type Chain struct {
	middlewares []Middleware
}

func New() *Chain {
	return &Chain{middlewares: []Middleware{}}
}

func (c *Chain) Use(mw Middleware) {
	c.middlewares = append(c.middlewares, mw)
}

func (c *Chain) Then(h http.Handler) http.Handler {
	for i := len(c.middlewares) - 1; i >= 0; i-- {
		h = c.middlewares[i](h)
	}
	return h
}

func (c *Chain) ThenFunc(fn http.HandlerFunc) http.Handler {
	return c.Then(http.HandlerFunc(fn))
}

func (c *Chain) Apply(h http.Handler) http.Handler {
	for i := len(c.middlewares) - 1; i >= 0; i-- {
		h = c.middlewares[i](h)
	}
	return h
}
