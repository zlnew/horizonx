package rest

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
)

type APIResponse struct {
	Message string `json:"message,omitempty"`
	Data    any    `json:"data,omitempty"`
	Meta    any    `json:"meta,omitempty"`
	Errors  any    `json:"errors,omitempty"`
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	buf := &bytes.Buffer{}
	if err := json.NewEncoder(buf).Encode(payload); err != nil {
		http.Error(w, "internal server error: failed to encode response", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)

	if _, err := buf.WriteTo(w); err != nil {
		fmt.Printf("failed to write response: %v\n", err)
	}
}

func JSONSuccess(w http.ResponseWriter, status int, resp APIResponse) {
	writeJSON(w, status, resp)
}

func JSONError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, APIResponse{
		Message: message,
		Data:    nil,
	})
}

func JSONValidationError(w http.ResponseWriter, errors map[string]string) {
	if len(errors) == 0 {
		writeJSON(w, http.StatusUnprocessableEntity, APIResponse{
			Message: "The given data was invalid.",
			Errors:  nil,
		})
		return
	}

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

	writeJSON(w, http.StatusUnprocessableEntity, APIResponse{
		Message: finalMessage,
		Errors:  errors,
	})
}
