package handler

import (
	"net/http"
	"strconv"

	"github.com/himanshumehta/social-publisher/internal/service"
)

func (h *Handler) CreateComment(w http.ResponseWriter, r *http.Request) {
	userID, ok := h.requireUserID(w, r)
	if !ok {
		return
	}

	var req service.CreateCommentRequest
	if !h.requireJSON(w, r, &req) {
		return
	}

	review, err := h.services.Review.CreateComment(r.Context(), userID, req)
	if err != nil {
		h.logger.Error("create comment failed", "error", err)
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	writeJSON(w, http.StatusCreated, review)
}

func (h *Handler) ListComments(w http.ResponseWriter, r *http.Request) {
	id, ok := h.requirePathUUID(w, r, "id", "project id")
	if !ok {
		return
	}

	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))

	reviews, total, err := h.services.Review.ListByProject(r.Context(), id, limit, offset)
	if err != nil {
		h.logger.Error("list comments failed", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to list comments")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"data":  reviews,
		"total": total,
	})
}

func (h *Handler) ListVideoComments(w http.ResponseWriter, r *http.Request) {
	id, ok := h.requirePathUUID(w, r, "videoId", "video id")
	if !ok {
		return
	}

	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))

	reviews, total, err := h.services.Review.ListByVideo(r.Context(), id, limit, offset)
	if err != nil {
		h.logger.Error("list video comments failed", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to list video comments")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"data":  reviews,
		"total": total,
	})
}

func (h *Handler) ListEditorProjects(w http.ResponseWriter, r *http.Request) {
	userID, ok := h.requireUserID(w, r)
	if !ok {
		return
	}

	status := r.URL.Query().Get("status")
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))

	projects, total, err := h.services.Project.ListByEditor(r.Context(), userID, status, limit, offset)
	if err != nil {
		h.logger.Error("list editor projects failed", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to list projects")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"data":  projects,
		"total": total,
	})
}

func (h *Handler) ListEditors(w http.ResponseWriter, r *http.Request) {
	users, err := h.services.User.ListByRole(r.Context(), "editor")
	if err != nil {
		h.logger.Error("list editors failed", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to list editors")
		return
	}

	writeJSON(w, http.StatusOK, users)
}
