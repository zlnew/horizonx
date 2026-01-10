// Package request
package request

import (
	"encoding/json"
	"errors"
	"net/http"
)

var ErrInvalidBody = errors.New("invalid request body")

type RequestDecoder interface {
	Decode(r *http.Request, req any) error
}

type JSONDecoder struct{}

func NewJSONDecoder() RequestDecoder {
	return &JSONDecoder{}
}

func (d *JSONDecoder) Decode(r *http.Request, req any) error {
	defer r.Body.Close()

	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()

	if err := dec.Decode(req); err != nil {
		return ErrInvalidBody
	}

	return nil
}
