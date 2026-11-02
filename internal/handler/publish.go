package handler

import (
	"net/http"
	"strconv"

	"github.com/google/uuid"
)

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
