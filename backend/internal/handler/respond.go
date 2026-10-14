package handler

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"github.com/farrellm/bbb/internal/store"
)

// maxBody caps JSON request bodies; nothing the app sends comes close.
const maxBody = 64 << 10

// writeJSON sends v as a JSON body with the given status code.
func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

// writeError sends {"error": msg} with the given status code.
func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, errorBody{Error: msg})
}

type errorBody struct {
	Error string `json:"error"`
}

// internalError logs err and sends a 500 with msg, so no 500 goes unlogged.
func internalError(w http.ResponseWriter, r *http.Request, msg string, err error) {
	slog.Error(msg, "err", err, "method", r.Method, "path", r.URL.Path)
	writeError(w, http.StatusInternalServerError, msg)
}

// notFoundOr500 maps store.ErrNotFound to a 404 and anything else to a
// logged 500.
func notFoundOr500(w http.ResponseWriter, r *http.Request, err error, notFoundMsg, failMsg string) {
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, notFoundMsg)
		return
	}
	internalError(w, r, failMsg, err)
}

// decodeJSON decodes a size-limited request body into dst, rejecting
// unknown fields.
func decodeJSON(w http.ResponseWriter, r *http.Request, dst any) error {
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxBody))
	dec.DisallowUnknownFields()
	return dec.Decode(dst)
}

// pathID parses the named URL parameter as a positive id.
func pathID(r *http.Request, param string) (int64, bool) {
	id, err := strconv.ParseInt(chi.URLParam(r, param), 10, 64)
	return id, err == nil && id > 0
}

// queryInt parses an optional integer query parameter, returning def when
// it is absent and false when it is malformed or outside [lo, hi].
func queryInt(r *http.Request, name string, def, lo, hi int64) (int64, bool) {
	v := r.URL.Query().Get(name)
	if v == "" {
		return def, true
	}
	n, err := strconv.ParseInt(v, 10, 64)
	return n, err == nil && n >= lo && n <= hi
}

// queryBool parses an optional boolean query parameter: "1" or "true".
func queryBool(r *http.Request, name string) bool {
	v := r.URL.Query().Get(name)
	return v == "1" || v == "true"
}
