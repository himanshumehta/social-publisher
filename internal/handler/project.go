package handler

import (
	"net/http"
	"strconv"

	"github.com/google/uuid"
	"github.com/himanshumehta/social-publisher/internal/model"
	"github.com/himanshumehta/social-publisher/internal/service"
)

func (h *Handler) CreateProject(w http.ResponseWriter, r *http.Request) {
	userID, ok := h.requireUserID(w, r)
	if !ok {
		return
	}

	var req service.CreateProjectRequest
	if !h.requireJSON(w, r, &req) {
		return
	}

	project, err := h.services.Project.Create(r.Context(), userID, req)
	if err != nil {
		h.logger.Error("create project failed", "error", err)
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	writeJSON(w, http.StatusCreated, project)
}

func (h *Handler) ListAllProjects(w http.ResponseWriter, r *http.Request) {
	status := r.URL.Query().Get("status")
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))

	projects, total, err := h.services.Project.ListAll(r.Context(), status, limit, offset)
	if err != nil {
		h.logger.Error("list all projects failed", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to list projects")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"data":  projects,
		"total": total,
	})
}

func (h *Handler) GetProject(w http.ResponseWriter, r *http.Request) {
	idStr := r.PathValue("id")

	// Try UUID first, fall back to slug
	var project *model.Project
	var err error
	if id, parseErr := uuid.Parse(idStr); parseErr == nil {
		project, err = h.services.Project.GetByID(r.Context(), id)
	} else {
		project, err = h.services.Project.GetBySlug(r.Context(), idStr)
	}
	if err != nil {
		writeError(w, http.StatusNotFound, "project not found")
		return
	}

	writeJSON(w, http.StatusOK, project)
}

func (h *Handler) CreateSelfProject(w http.ResponseWriter, r *http.Request) {
	userID, ok := h.requireUserID(w, r)
	if !ok {
		return
	}

	var req service.CreateSelfProjectRequest
	if !h.requireJSON(w, r, &req) {
		return
	}

	project, err := h.services.Project.CreateSelfProject(r.Context(), userID, req)
	if err != nil {
		h.logger.Error("create self project failed", "error", err)
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	writeJSON(w, http.StatusCreated, project)
}

func (h *Handler) UpdateProject(w http.ResponseWriter, r *http.Request) {
	userID, ok := h.requireUserID(w, r)
	if !ok {
		return
	}

	id, ok := h.requirePathUUID(w, r, "id", "project id")
	if !ok {
		return
	}

	var req service.UpdateProjectRequest
	if !h.requireJSON(w, r, &req) {
		return
	}

	// Check if user is admin
	user, err := h.services.User.GetByID(r.Context(), userID)
	isAdmin := err == nil && (user.Role == "admin" || user.Role == "super_admin")

	project, err := h.services.Project.Update(r.Context(), id, userID, isAdmin, req)
	if err != nil {
		h.logger.Error("update project failed", "error", err)
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, project)
}

func (h *Handler) DeleteProject(w http.ResponseWriter, r *http.Request) {
	id, ok := h.requirePathUUID(w, r, "id", "project id")
	if !ok {
		return
	}

	err := h.services.Project.Delete(r.Context(), id)
	if err != nil {
		h.logger.Error("delete project failed", "error", err)
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"message": "project deleted"})
}

func (h *Handler) ListMyProjects(w http.ResponseWriter, r *http.Request) {
	userID, ok := h.requireUserID(w, r)
	if !ok {
		return
	}

	status := r.URL.Query().Get("status")
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))

	projects, total, err := h.services.Project.ListByCreator(r.Context(), userID, status, limit, offset)
	if err != nil {
		h.logger.Error("list my projects failed", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to list projects")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"data":  projects,
		"total": total,
	})
}

func (h *Handler) RequestEditing(w http.ResponseWriter, r *http.Request) {
	userID, ok := h.requireUserID(w, r)
	if !ok {
		return
	}

	id, ok := h.requirePathUUID(w, r, "id", "project id")
	if !ok {
		return
	}

	var req service.RequestEditingRequest
	if !h.requireJSON(w, r, &req) {
		return
	}

	project, err := h.services.Project.RequestEditing(r.Context(), id, userID, req.EditorID, req.Notes)
	if err != nil {
		h.logger.Error("request editing failed", "error", err)
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, project)
}

func (h *Handler) MarkEditingDone(w http.ResponseWriter, r *http.Request) {
	userID, ok := h.requireUserID(w, r)
	if !ok {
		return
	}

	id, ok := h.requirePathUUID(w, r, "id", "project id")
	if !ok {
		return
	}

	project, err := h.services.Project.MarkEditingDone(r.Context(), id, userID)
	if err != nil {
		h.logger.Error("mark editing done failed", "error", err)
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, project)
}

func (h *Handler) ListOpenAssignments(w http.ResponseWriter, r *http.Request) {
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))

	projects, total, err := h.services.Project.ListOpenAssignments(r.Context(), limit, offset)
	if err != nil {
		h.logger.Error("list open assignments failed", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to list assignments")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"data":  projects,
		"total": total,
	})
}

func (h *Handler) ClaimAssignment(w http.ResponseWriter, r *http.Request) {
	userID, ok := h.requireUserID(w, r)
	if !ok {
		return
	}

	id, ok := h.requirePathUUID(w, r, "id", "project id")
	if !ok {
		return
	}

	project, err := h.services.Project.ClaimAssignment(r.Context(), id, userID)
	if err != nil {
		h.logger.Error("claim assignment failed", "error", err)
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, project)
}

func (h *Handler) ListReviewQueue(w http.ResponseWriter, r *http.Request) {
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))

	projects, total, err := h.services.Project.ListReviewQueue(r.Context(), limit, offset)
	if err != nil {
		h.logger.Error("list review queue failed", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to list review queue")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"data":  projects,
		"total": total,
	})
}
