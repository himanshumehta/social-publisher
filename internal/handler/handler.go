package handler

import (
	"encoding/json"
	"log/slog"
	"net/http"

	"github.com/google/uuid"
	"github.com/himanshumehta/social-publisher/internal/config"
	"github.com/himanshumehta/social-publisher/internal/service"
)

type Handler struct {
	cfg      *config.Config
	services *service.Services
	logger   *slog.Logger
}

func New(cfg *config.Config, svc *service.Services, logger *slog.Logger) *Handler {
	return &Handler{cfg: cfg, services: svc, logger: logger}
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

func readJSON(r *http.Request, v any) error {
	defer r.Body.Close()
	return json.NewDecoder(r.Body).Decode(v)
}

// requireUserID extracts the authenticated user ID from the request context.
// Returns false (and writes a 401 response) if the user is not authenticated.
func (h *Handler) requireUserID(w http.ResponseWriter, r *http.Request) (uuid.UUID, bool) {
	userID, ok := userIDFromRequest(r)
	if !ok {
		writeError(w, http.StatusUnauthorized, "not authenticated")
	}
	return userID, ok
}

// requirePathUUID parses a UUID path parameter from the request.
// Returns false (and writes a 400 response) if the value is not a valid UUID.
func (h *Handler) requirePathUUID(w http.ResponseWriter, r *http.Request, param, label string) (uuid.UUID, bool) {
	id, err := uuid.Parse(r.PathValue(param))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid "+label)
		return uuid.UUID{}, false
	}
	return id, true
}

// requireJSON decodes the JSON request body into v.
// Returns false (and writes a 400 response) if decoding fails.
func (h *Handler) requireJSON(w http.ResponseWriter, r *http.Request, v any) bool {
	if err := readJSON(r, v); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return false
	}
	return true
}

// safeSlice returns an empty slice instead of nil to prevent JSON null in API responses.
func safeSlice[T any](s []T) []T {
	if s == nil {
		return []T{}
	}
	return s
}
