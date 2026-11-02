package handler

import (
	"net/http"

	"github.com/himanshumehta/social-publisher/internal/middleware"
	"github.com/himanshumehta/social-publisher/internal/service"
)

func (h *Handler) TransitionProject(w http.ResponseWriter, r *http.Request) {
	userID, ok := h.requireUserID(w, r)
	if !ok {
		return
	}

	projectID, ok := h.requirePathUUID(w, r, "id", "project id")
	if !ok {
		return
	}

	var req service.TransitionRequest
	if !h.requireJSON(w, r, &req) {
		return
	}

	role := middleware.RoleFromContext(r.Context())
	project, err := h.services.Transition.Transition(r.Context(), userID, role, projectID, req)
	if err != nil {
		h.logger.Error("transition project failed", "error", err)
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, project)
}

func (h *Handler) GetAllowedTransitions(w http.ResponseWriter, r *http.Request) {
	userID, ok := h.requireUserID(w, r)
	if !ok {
		return
	}

	projectID, ok := h.requirePathUUID(w, r, "id", "project id")
	if !ok {
		return
	}

	role := middleware.RoleFromContext(r.Context())
	transitions, err := h.services.Transition.GetAllowedTransitions(r.Context(), projectID, userID, role)
	if err != nil {
		h.logger.Error("get allowed transitions failed", "error", err)
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string][]string{"transitions": transitions})
}
