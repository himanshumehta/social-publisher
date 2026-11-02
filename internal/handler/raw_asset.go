package handler

import (
	"net/http"
	"strconv"

	"github.com/google/uuid"
	"github.com/himanshumehta/social-publisher/internal/model"
)

// ListRawAssets — GET /api/v1/projects/{id}/raw-assets
func (h *Handler) ListRawAssets(w http.ResponseWriter, r *http.Request) {
	idStr := r.PathValue("id")
	projectID, err := uuid.Parse(idStr)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid project id")
		return
	}

	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))

	rawAssets, total, err := h.services.RawAsset.ListByProject(r.Context(), projectID, limit, offset)
	if err != nil {
		h.logger.Error("list raw assets failed", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to list raw assets")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"data":  rawAssets,
		"total": total,
	})
}

// UpdateRawAssetStatus — PATCH /api/v1/raw-assets/{id}/status
func (h *Handler) UpdateRawAssetStatus(w http.ResponseWriter, r *http.Request) {
	idStr := r.PathValue("id")
	rawAssetID, err := uuid.Parse(idStr)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid raw asset id")
		return
	}

	var req struct {
		Status string `json:"status"`
	}
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.Status == "" {
		writeError(w, http.StatusBadRequest, "status is required")
		return
	}

	if err := h.services.RawAsset.UpdateStatus(r.Context(), rawAssetID, model.RawAssetStatus(req.Status)); err != nil {
		h.logger.Error("update raw asset status failed", "error", err)
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"message": "status updated"})
}
