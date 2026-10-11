package task

import (
	"encoding/json"
	"net/http"
)

func writeJSON(w http.ResponseWriter, status int, data any) error {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(data); err != nil {
		return err
	}
	return nil
}

func writeError(w http.ResponseWriter, status int, message string) error {
	return writeJSON(w, status, errorResponse{
		Error: message,
	})
}
