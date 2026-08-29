package api

import (
	"encoding/json"
	"log"
	"net/http"
)

var jsonEncodingFailure = []byte("{\"error\":\"JSON response encoding failed\"}\n")

func writeError(w http.ResponseWriter, code int, message string) {
	writeJSON(w, code, map[string]any{"error": message})
}

func writeJSON(w http.ResponseWriter, code int, payload any) {
	body, err := json.Marshal(payload)
	if err != nil {
		log.Printf("JSON response encoding rejected status=%d payload_type=%T: %v", code, payload, err)
		code = http.StatusInternalServerError
		body = append([]byte(nil), jsonEncodingFailure...)
	} else {
		body = append(body, '\n')
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	if _, err := w.Write(body); err != nil {
		log.Printf("JSON response write failed status=%d payload_type=%T bytes=%d: %v", code, payload, len(body), err)
	}
}
