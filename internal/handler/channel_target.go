package handler

import (
	"net/http"

	"github.com/himanshumehta/social-publisher/internal/service"
)

func (h *Handler) CreateChannelTargets(w http.ResponseWriter, r *http.Request) {
	userID, ok := h.requireUserID(w, r)
	if !ok {
		return
	}

	projectID, ok := h.requirePathUUID(w, r, "id", "project id")
	if !ok {
		return
	}

	var req struct {
		Targets []service.CreateChannelTargetRequest `json:"targets"`
	}
	if !h.requireJSON(w, r, &req) {
		return
	}

	user, err := h.services.User.GetByID(r.Context(), userID)
	isAdmin := err == nil && (user.Role == "admin" || user.Role == "super_admin")

	targets, err := h.services.ChannelTarget.CreateBulk(r.Context(), projectID, userID, isAdmin, req.Targets)
	if err != nil {
		h.logger.Error("create channel targets failed", "error", err)
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	writeJSON(w, http.StatusCreated, targets)
}

func (h *Handler) ListChannelTargets(w http.ResponseWriter, r *http.Request) {
	projectID, ok := h.requirePathUUID(w, r, "id", "project id")
	if !ok {
		return
	}

	targets, err := h.services.ChannelTarget.ListByProject(r.Context(), projectID)
	if err != nil {
		h.logger.Error("list channel targets failed", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to list channel targets")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"data": targets})
}

func (h *Handler) DeleteChannelTarget(w http.ResponseWriter, r *http.Request) {
	userID, ok := h.requireUserID(w, r)
	if !ok {
		return
	}

	targetID, ok := h.requirePathUUID(w, r, "id", "target id")
	if !ok {
		return
	}

	user, err := h.services.User.GetByID(r.Context(), userID)
	isAdmin := err == nil && (user.Role == "admin" || user.Role == "super_admin")

	if err := h.services.ChannelTarget.Delete(r.Context(), targetID, userID, isAdmin); err != nil {
		h.logger.Error("delete channel target failed", "error", err)
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}
