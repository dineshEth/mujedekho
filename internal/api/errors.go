// Package api implements the public REST surface under /api/v1.
package api

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"

	"github.com/google/uuid"

	"site24x7/internal/store"
)

// Error is the one error envelope the whole API emits. Clients match on code.
type Error struct {
	Code    string         `json:"code"`
	Message string         `json:"message"`
	Details map[string]any `json:"details,omitempty"`
}

type errorEnvelope struct {
	Error Error `json:"error"`
}

// WriteError maps internal failures onto the envelope. Store-level sentinels
// are translated here so no handler ever leaks driver errors.
func WriteError(w http.ResponseWriter, log *slog.Logger, status int, code, message string, details map[string]any) {
	body := errorEnvelope{Error: Error{Code: code, Message: message, Details: details}}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(body); err != nil {
		log.Error("api: encode error envelope", "err", err)
	}
}

// WriteInternal: logs the real cause, returns opacity to the client.
func WriteInternal(w http.ResponseWriter, log *slog.Logger, err error, what string) {
	log.Error("api: internal error", "what", what, "err", err)
	WriteError(w, log, http.StatusInternalServerError, "internal_error", "An internal error occurred.", nil)
}

// StatusFor maps domain/store errors to HTTP semantics.
func StatusFor(err error) (int, string, string) {
	switch {
	case errors.Is(err, store.ErrNotFound):
		return http.StatusNotFound, "not_found", "The requested resource does not exist or is outside your project scope."
	case errors.Is(err, store.ErrConflict):
		return http.StatusConflict, "conflict", "The resource already exists or violates a constraint."
	default:
		return 0, "", ""
	}
}

// WriteStoreError handles the common store error paths; returns false when
// err was not a recognized sentinel.
func WriteStoreError(w http.ResponseWriter, log *slog.Logger, err error) bool {
	status, code, msg := StatusFor(err)
	if status == 0 {
		return false
	}
	WriteError(w, log, status, code, msg, nil)
	return true
}

// ValidateJSON decodes a JSON body strictly (unknown fields rejected keeps API
// contracts honest) into dst. Returns ok=false when a response was written.
func ValidateJSON(r *http.Request, dst any) (ok bool) {
	dec := json.NewDecoder(http.MaxBytesReader(nil, r.Body, 1<<20))
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		return false
	}
	return true
}

func parseUUID(raw string) (uuid.UUID, bool) {
	id, err := uuid.Parse(raw)
	return id, err == nil
}
