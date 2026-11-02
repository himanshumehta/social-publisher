package handler

import (
	"net/http"
	"strconv"

	"github.com/himanshumehta/social-publisher/internal/middleware"
)

func (h *Handler) ListAllUsers(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	search := q.Get("search")
	role := q.Get("role")
	limit, _ := strconv.Atoi(q.Get("limit"))
	offset, _ := strconv.Atoi(q.Get("offset"))
	if limit <= 0 {
		limit = 50
	}

	users, total, err := h.services.User.ListAll(r.Context(), search, role, limit, offset)
	if err != nil {
		h.logger.Error("list all users failed", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to list users")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"data":  users,
		"total": total,
	})
}

type changeRoleRequest struct {
	Role string `json:"role"`
}

func (h *Handler) ChangeUserRole(w http.ResponseWriter, r *http.Request) {
	actorID, ok := h.requireUserID(w, r)
	if !ok {
		return
	}

	targetID, ok := h.requirePathUUID(w, r, "id", "user id")
	if !ok {
		return
	}

	var req changeRoleRequest
	if !h.requireJSON(w, r, &req) {
		return
	}

	validRoles := map[string]bool{"creator": true, "editor": true, "admin": true}
	if !validRoles[req.Role] {
		writeError(w, http.StatusBadRequest, "invalid role: must be creator, editor, or admin")
		return
	}

	// Prevent changing own role
	if actorID == targetID {
		writeError(w, http.StatusBadRequest, "cannot change your own role")
		return
	}

	actorRole := middleware.RoleFromContext(r.Context())

	// Only super_admin can promote to admin
	if req.Role == "admin" && actorRole != "super_admin" {
		writeError(w, http.StatusForbidden, "only super_admin can promote users to admin")
		return
	}

	if err := h.services.User.ChangeRole(r.Context(), targetID, req.Role, actorID); err != nil {
		h.logger.Error("change user role failed", "error", err, "target", targetID, "role", req.Role)
		writeError(w, http.StatusInternalServerError, "failed to change user role")
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"message": "role updated"})
}

func (h *Handler) GetAdminAnalytics(w http.ResponseWriter, r *http.Request) {
	period := r.URL.Query().Get("period")
	if period == "" {
		period = "30d"
	}

	analytics, err := h.services.Dashboard.GetAdminAnalytics(r.Context(), period)
	if err != nil {
		h.logger.Error("get admin analytics failed", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to get analytics")
		return
	}

	writeJSON(w, http.StatusOK, analytics)
}

func (h *Handler) RetryThumbnail(w http.ResponseWriter, r *http.Request) {
	id, ok := h.requirePathUUID(w, r, "id", "publish record id")
	if !ok {
		return
	}

	if err := h.services.Orchestrator.RetryYouTubeThumbnail(r.Context(), id); err != nil {
		h.logger.Error("retry thumbnail failed", "publish_record_id", id, "error", err)
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"message": "thumbnail set"})
}

func (h *Handler) GetCreatorAnalytics(w http.ResponseWriter, r *http.Request) {
	userID, ok := h.requireUserID(w, r)
	if !ok {
		return
	}

	period := r.URL.Query().Get("period")
	if period == "" {
		period = "30d"
	}

	analytics, err := h.services.Dashboard.GetCreatorAnalytics(r.Context(), userID, period)
	if err != nil {
		h.logger.Error("get creator analytics failed", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to get analytics")
		return
	}

	writeJSON(w, http.StatusOK, analytics)
}
