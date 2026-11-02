package handler

import (
	"net/http"
	"strconv"

	"github.com/google/uuid"
	"github.com/himanshumehta/social-publisher/internal/service"
)

// CreatePublishRecord — POST /api/v1/projects/{id}/publish-records
func (h *Handler) CreatePublishRecord(w http.ResponseWriter, r *http.Request) {
	idStr := r.PathValue("id")
	projectID, err := uuid.Parse(idStr)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid project id")
		return
	}

	var req service.CreatePublishRecordRequest
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	req.ProjectID = projectID

	record, err := h.services.Publish.CreateRecord(r.Context(), req)
	if err != nil {
		h.logger.Error("create publish record failed", "error", err)
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	writeJSON(w, http.StatusCreated, record)
}

// ListPublishRecords — GET /api/v1/projects/{id}/publish-records
func (h *Handler) ListPublishRecords(w http.ResponseWriter, r *http.Request) {
	idStr := r.PathValue("id")
	projectID, err := uuid.Parse(idStr)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid project id")
		return
	}

	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))

	records, total, err := h.services.Publish.ListByProject(r.Context(), projectID, limit, offset)
	if err != nil {
		h.logger.Error("list publish records failed", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to list publish records")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"data":  records,
		"total": total,
	})
}

// UpdatePublishRecord — PATCH /api/v1/publish-records/{id}
func (h *Handler) UpdatePublishRecord(w http.ResponseWriter, r *http.Request) {
	idStr := r.PathValue("id")
	id, err := uuid.Parse(idStr)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid publish record id")
		return
	}

	var req service.UpdatePublishRecordRequest
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	record, err := h.services.Publish.UpdateRecord(r.Context(), id, req)
	if err != nil {
		h.logger.Error("update publish record failed", "error", err)
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, record)
}

// DeletePublishRecord — DELETE /api/v1/publish-records/{id}
func (h *Handler) DeletePublishRecord(w http.ResponseWriter, r *http.Request) {
	idStr := r.PathValue("id")
	id, err := uuid.Parse(idStr)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid publish record id")
		return
	}

	if err := h.services.Publish.DeleteRecord(r.Context(), id); err != nil {
		h.logger.Error("delete publish record failed", "error", err)
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}

// ListProjectChannels — GET /api/v1/projects/{id}/channels
func (h *Handler) ListProjectChannels(w http.ResponseWriter, r *http.Request) {
	idStr := r.PathValue("id")
	projectID, err := uuid.Parse(idStr)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid project id")
		return
	}

	channels, err := h.services.Publish.GetCategoryChannels(r.Context(), projectID)
	if err != nil {
		h.logger.Error("list project channels failed", "error", err)
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"data": channels})
}

// GetSuggestedChannels — GET /api/v1/projects/{id}/suggested-channels
func (h *Handler) GetSuggestedChannels(w http.ResponseWriter, r *http.Request) {
	idStr := r.PathValue("id")
	projectID, err := uuid.Parse(idStr)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid project id")
		return
	}

	suggestions, err := h.services.Publish.GetSuggestedChannels(r.Context(), projectID)
	if err != nil {
		h.logger.Error("get suggested channels failed", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to get suggested channels")
		return
	}

	writeJSON(w, http.StatusOK, suggestions)
}
