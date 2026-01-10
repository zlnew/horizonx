// Package response
package response

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"sort"

	"horizonx/internal/logger"
)

type ResponseWriter interface {
	Write(w http.ResponseWriter, status int, data *Response)
	WriteValidationError(w http.ResponseWriter, errors map[string]string)
}

type Response struct {
	Message string             `json:"message,omitempty"`
	Data    any                `json:"data,omitempty"`
	Meta    any                `json:"meta,omitempty"`
	Errors  *map[string]string `json:"errors,omitempty"`
}

type JSONWriter struct {
	log logger.Logger
}

func NewJSONWriter(log logger.Logger) ResponseWriter {
	return &JSONWriter{log: log}
}

func (j *JSONWriter) Write(w http.ResponseWriter, status int, data *Response) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)

	if data == nil {
		return
	}

	buf := &bytes.Buffer{}
	if err := json.NewEncoder(buf).Encode(&data); err != nil {
		http.Error(w, "failed to encode response", http.StatusInternalServerError)
		return
	}

	if _, err := buf.WriteTo(w); err != nil {
		j.log.Error("failed to write json response", "error", err.Error())
	}
}

func (j *JSONWriter) WriteValidationError(w http.ResponseWriter, errors map[string]string) {
	keys := make([]string, 0, len(errors))
	for k := range errors {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	firstField := keys[0]
	mainMessage := errors[firstField]
	remaining := len(errors) - 1

	var finalMessage string
	switch remaining {
	case 0:
		finalMessage = mainMessage
	case 1:
		finalMessage = fmt.Sprintf("%s (and 1 more error)", mainMessage)
	default:
		finalMessage = fmt.Sprintf("%s (and %d more errors)", mainMessage, remaining)
	}

	j.Write(w, http.StatusUnprocessableEntity, &Response{
		Message: finalMessage,
		Errors:  &errors,
	})
}
